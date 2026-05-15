#!/usr/bin/env python3
"""cap-git HTTP API — Lightweight Git operations server for project containers."""
import http.server
import json
import os
import subprocess
import base64
import urllib.parse

PORT = int(os.environ.get("GIT_API_PORT", "9090"))
WORKSPACE = os.environ.get("GIT_WORKSPACE", "/workspace")


def run_git(*args, check=False, capture=True):
    """Run a git command in the workspace."""
    cmd = ["git", "-C", WORKSPACE] + list(args)
    try:
        result = subprocess.run(
            cmd, capture_output=capture, text=True, timeout=30, check=check,
            env={**os.environ, "GIT_TERMINAL_PROMPT": "0"}
        )
        return result
    except subprocess.TimeoutExpired:
        return None
    except subprocess.CalledProcessError as e:
        return e


def json_response(code, data):
    """Build HTTP response as JSON bytes."""
    body = json.dumps(data, ensure_ascii=False).encode()
    return code, body


class GitAPIHandler(http.server.BaseHTTPRequestHandler):
    def _read_body(self):
        length = int(self.headers.get("Content-Length", 0))
        if length > 0:
            return json.loads(self.rfile.read(length))
        return {}

    def _send(self, code, body_bytes):
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body_bytes)))
        self.end_headers()
        self.wfile.write(body_bytes)

    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        path = parsed.path.rstrip("/") or "/"
        query = urllib.parse.parse_qs(parsed.query)

        if path == "/health":
            self._send(*json_response(200, {"ok": True, "service": "cap-git"}))

        elif path == "/files":
            filepath = query.get("path", [None])[0]
            if filepath:
                full = os.path.join(WORKSPACE, filepath)
                if os.path.isfile(full):
                    with open(full, "rb") as f:
                        content = base64.b64encode(f.read()).decode()
                    self._send(*json_response(200, {"ok": True, "path": filepath, "content": content, "encoding": "base64"}))
                else:
                    self._send(*json_response(404, {"ok": False, "error": "file not found"}))
            else:
                r = run_git("ls-files")
                if r and r.returncode == 0:
                    files = [f for f in r.stdout.strip().split("\n") if f]
                else:
                    files = []
                    for root, dirs, fnames in os.walk(WORKSPACE):
                        if ".git" in root.split(os.sep):
                            continue
                        for fn in fnames:
                            rel = os.path.relpath(os.path.join(root, fn), WORKSPACE)
                            files.append(rel)
                self._send(*json_response(200, {"ok": True, "files": files}))

        elif path == "/diff":
            # Try HEAD~1 first (last commit), then staged, then unstaged
            diff = ""
            for args in [["HEAD~1", "HEAD"], ["--cached"], []]:
                r = run_git("diff", *args)
                if r and r.stdout.strip():
                    diff = r.stdout
                    break
            diff_b64 = base64.b64encode(diff.encode()).decode()
            self._send(*json_response(200, {"ok": True, "diff": diff_b64, "diff_size": len(diff)}))

        elif path == "/log":
            r = run_git("log", "--oneline", "-20")
            log = r.stdout.strip() if r and r.returncode == 0 else "no commits yet"
            log_b64 = base64.b64encode(log.encode()).decode()
            self._send(*json_response(200, {"ok": True, "log": log_b64}))

        else:
            self._send(*json_response(404, {"ok": False, "error": f"not found: {path}"}))

    def do_POST(self):
        parsed = urllib.parse.urlparse(self.path)
        path = parsed.path.rstrip("/") or "/"
        body = self._read_body()

        if path == "/init":
            repo_url = body.get("repo_url", "")
            branch = body.get("branch", "main")

            if repo_url:
                self._log(f"Cloning {repo_url} (branch: {branch})")
                proxy = os.environ.get("HTTP_PROXY", os.environ.get("http_proxy", ""))
                if proxy:
                    run_git("config", "--global", "http.proxy", proxy)
                r = run_git("clone", "--depth", "1", "--single-branch", "-b", branch, repo_url, "/tmp/clone")
                if r and r.returncode == 0:
                    # Move cloned content into workspace
                    subprocess.run(["cp", "-a", "/tmp/clone/.", WORKSPACE], check=False)
                    subprocess.run(["rm", "-rf", "/tmp/clone"], check=False)
                    self._send(*json_response(200, {"ok": True, "action": "clone", "branch": branch}))
                else:
                    err = r.stderr.strip() if r else "unknown error"
                    # Clone failed, init empty
                    run_git("init")
                    self._send(*json_response(200, {"ok": True, "action": "init_empty", "warning": err}))
            else:
                run_git("init")
                self._send(*json_response(200, {"ok": True, "action": "init_empty"}))

        elif path == "/commit":
            message = body.get("message", "agent commit")
            branch_name = body.get("branch", "")

            if branch_name:
                r = run_git("checkout", "-b", branch_name)
                if r and r.returncode != 0:
                    run_git("checkout", branch_name)

            run_git("add", "-A")
            r = run_git("diff", "--cached", "--stat")
            if r and not r.stdout.strip():
                self._send(*json_response(200, {"ok": True, "action": "commit", "message": "no changes"}))
            else:
                r = run_git("commit", "-m", message)
                sha_r = run_git("rev-parse", "--short", "HEAD")
                sha = sha_r.stdout.strip() if sha_r and sha_r.returncode == 0 else "unknown"
                self._send(*json_response(200, {"ok": True, "action": "commit", "sha": sha, "message": message}))

        elif path == "/push":
            remote = body.get("remote", "origin")
            push_branch = body.get("branch", "")
            token = body.get("token", "")

            if token:
                url_r = run_git("remote", "get-url", remote)
                if url_r and url_r.returncode == 0:
                    current_url = url_r.stdout.strip()
                    if current_url.startswith("https://"):
                        auth_url = current_url.replace("https://", f"https://{token}@")
                        run_git("remote", "set-url", remote, auth_url)

            if push_branch:
                r = run_git("push", remote, push_branch)
            else:
                r = run_git("push", remote)

            output = r.stdout.strip() if r else ""
            err = r.stderr.strip() if r else "push failed"
            ok = r and r.returncode == 0
            self._send(*json_response(200 if ok else 500, {"ok": ok, "output": output, "error": "" if ok else err}))

        elif path == "/branch":
            name = body.get("name", "")
            if not name:
                self._send(*json_response(400, {"ok": False, "error": "branch name required"}))
            else:
                run_git("checkout", "-b", name)
                self._send(*json_response(200, {"ok": True, "branch": name}))

        else:
            self._send(*json_response(404, {"ok": False, "error": f"not found: {path}"}))

    def _log(self, msg):
        print(f"[cap-git] {msg}", flush=True)

    def log_message(self, format, *args):
        self._log(format % args)


if __name__ == "__main__":
    # Configure git globally
    run_git("config", "--global", "user.email", "agent@cloud-agent-platform.dev")
    run_git("config", "--global", "user.name", "CAP Git Container")

    print(f"[cap-git] Starting API server on :{PORT}, workspace={WORKSPACE}", flush=True)
    server = http.server.HTTPServer(("0.0.0.0", PORT), GitAPIHandler)
    server.serve_forever()
