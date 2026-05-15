// git-api-server — lightweight HTTP API for Git operations inside cap-git containers.
// Build: GOOS=linux GOARCH=arm64 go build -o git-api-server .
package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var workspace = envOrDefault("GIT_WORKSPACE", "/workspace")

func main() {
	log.Println("[cap-git] Starting API server on :9090, workspace=", workspace)
	runGit("config", "--global", "user.email", "agent@cloud-agent-platform.dev")
	runGit("config", "--global", "user.name", "CAP Git Container")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/init", handleInit)
	mux.HandleFunc("/files", handleFiles)
	mux.HandleFunc("/commit", handleCommit)
	mux.HandleFunc("/diff", handleDiff)
	mux.HandleFunc("/push", handlePush)
	mux.HandleFunc("/log", handleLog)
	mux.HandleFunc("/branch", handleBranch)

	srv := &http.Server{Addr: ":9090", Handler: mux, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, 200, map[string]interface{}{"ok": true, "service": "cap-git"})
}

func handleInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonOK(w, 405, map[string]interface{}{"ok": false, "error": "POST required"})
		return
	}
	var body struct {
		RepoURL string `json:"repo_url"`
		Branch  string `json:"branch"`
	}
	readJSON(r, &body)
	if body.Branch == "" {
		body.Branch = "main"
	}

	if body.RepoURL != "" {
		log.Printf("[cap-git] Cloning %s (branch: %s)", body.RepoURL, body.Branch)
		// Set proxy if available
		if proxy := os.Getenv("HTTP_PROXY"); proxy == "" {
			if proxy = os.Getenv("http_proxy"); proxy != "" {
				runGit("config", "--global", "http.proxy", proxy)
			}
		} else {
			runGit("config", "--global", "http.proxy", proxy)
		}
		out, err := runGit("clone", "--depth", "1", "--single-branch", "-b", body.Branch, body.RepoURL, "/tmp/clone")
		if err != nil {
			log.Printf("[cap-git] Clone failed: %v, initing empty", err)
			runGit("init", workspace)
			jsonOK(w, 200, map[string]interface{}{"ok": true, "action": "init_empty", "warning": out})
			return
		}
		// Move cloned content
		exec.Command("sh", "-c", "cp -a /tmp/clone/. /workspace/ && rm -rf /tmp/clone").Run()
		jsonOK(w, 200, map[string]interface{}{"ok": true, "action": "clone", "branch": body.Branch})
	} else {
		runGit("init", workspace)
		jsonOK(w, 200, map[string]interface{}{"ok": true, "action": "init_empty"})
	}
}

func handleFiles(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath != "" {
		full := filepath.Join(workspace, filePath)
		data, err := os.ReadFile(full)
		if err != nil {
			jsonOK(w, 404, map[string]interface{}{"ok": false, "error": "file not found"})
			return
		}
		jsonOK(w, 200, map[string]interface{}{
			"ok":       true,
			"path":     filePath,
			"content":  base64.StdEncoding.EncodeToString(data),
			"encoding": "base64",
		})
		return
	}
	// List files
	out, err := runGit("ls-files")
	var files []string
	if err == nil && out != "" {
		for _, f := range strings.Split(out, "\n") {
			if f != "" {
				files = append(files, f)
			}
		}
	}
	if len(files) == 0 {
		filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.Contains(path, "/.git/") {
				return nil
			}
			rel, _ := filepath.Rel(workspace, path)
			files = append(files, rel)
			return nil
		})
	}
	jsonOK(w, 200, map[string]interface{}{"ok": true, "files": files})
}

func handleCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonOK(w, 405, map[string]interface{}{"ok": false, "error": "POST required"})
		return
	}
	var body struct {
		Message string `json:"message"`
		Branch  string `json:"branch"`
	}
	readJSON(r, &body)
	if body.Message == "" {
		body.Message = "agent commit"
	}

	if body.Branch != "" {
		runGit("checkout", "-b", body.Branch) // may fail if branch exists, that's ok
		runGit("checkout", body.Branch)
	}

	runGit("add", "-A")
	out, _ := runGit("diff", "--cached", "--stat")
	if strings.TrimSpace(out) == "" {
		jsonOK(w, 200, map[string]interface{}{"ok": true, "action": "commit", "message": "no changes"})
		return
	}

	runGit("commit", "-m", body.Message)
	sha, _ := runGit("rev-parse", "--short", "HEAD")
	jsonOK(w, 200, map[string]interface{}{"ok": true, "action": "commit", "sha": strings.TrimSpace(sha), "message": body.Message})
}

func handleDiff(w http.ResponseWriter, r *http.Request) {
	diff, _ := runGit("diff", "HEAD~1")
	if diff == "" {
		diff, _ = runGit("diff", "--cached")
	}
	if diff == "" {
		diff, _ = runGit("diff")
	}
	jsonOK(w, 200, map[string]interface{}{
		"ok":        true,
		"diff":      base64.StdEncoding.EncodeToString([]byte(diff)),
		"diff_size": len(diff),
	})
}

func handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonOK(w, 405, map[string]interface{}{"ok": false, "error": "POST required"})
		return
	}
	var body struct {
		Remote string `json:"remote"`
		Branch string `json:"branch"`
		Token  string `json:"token"`
	}
	readJSON(r, &body)
	if body.Remote == "" {
		body.Remote = "origin"
	}

	if body.Token != "" {
		urlOut, err := runGit("remote", "get-url", body.Remote)
		if err == nil && strings.HasPrefix(urlOut, "https://") {
			authURL := strings.Replace(urlOut, "https://", "https://"+body.Token+"@", 1)
			runGit("remote", "set-url", body.Remote, authURL)
		}
	}

	var out string
	var err error
	if body.Branch != "" {
		out, err = runGit("push", body.Remote, body.Branch)
	} else {
		out, err = runGit("push", body.Remote)
	}
	if err != nil {
		jsonOK(w, 500, map[string]interface{}{"ok": false, "error": out})
		return
	}
	jsonOK(w, 200, map[string]interface{}{"ok": true, "output": out})
}

func handleLog(w http.ResponseWriter, r *http.Request) {
	out, err := runGit("log", "--oneline", "-20")
	if err != nil {
		out = "no commits yet"
	}
	jsonOK(w, 200, map[string]interface{}{
		"ok":  true,
		"log": base64.StdEncoding.EncodeToString([]byte(out)),
	})
}

func handleBranch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonOK(w, 405, map[string]interface{}{"ok": false, "error": "POST required"})
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	readJSON(r, &body)
	if body.Name == "" {
		jsonOK(w, 400, map[string]interface{}{"ok": false, "error": "branch name required"})
		return
	}
	runGit("checkout", "-b", body.Name)
	jsonOK(w, 200, map[string]interface{}{"ok": true, "branch": body.Name})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", workspace}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func readJSON(r *http.Request, v interface{}) {
	data, _ := io.ReadAll(r.Body)
	json.Unmarshal(data, v)
}

func jsonOK(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// GetFreePort returns an available TCP port.
func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
