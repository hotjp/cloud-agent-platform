// Package pool provides Worker pool management with prewarming, scaling, and health checks.
package pool

import "time"

// Config contains configuration for the Worker pool.
type Config struct {
	// MinWorkers is the minimum number of workers to keep warm.
	MinWorkers int `koanf:"min_workers"`
	// MaxWorkers is the maximum number of workers in the pool.
	MaxWorkers int `koanf:"max_workers"`
	// ScaleUpStep is the number of workers to add when scaling up.
	ScaleUpStep int `koanf:"scale_up_step"`
	// ScaleDownStep is the number of workers to remove when scaling down.
	ScaleDownStep int `koanf:"scale_down_step"`
	// ScaleUpThreshold is the queue length threshold that triggers scale up.
	ScaleUpThreshold int `koanf:"scale_up_threshold"`
	// ScaleDownThreshold is the queue length threshold that triggers scale down.
	ScaleDownThreshold int `koanf:"scale_down_threshold"`
	// ScaleInterval is the interval between scaling checks.
	ScaleInterval time.Duration `koanf:"scale_interval"`
	// HealthCheckInterval is the interval between health checks.
	HealthCheckInterval time.Duration `koanf:"health_check_interval"`
	// HealthCheckTimeout is the timeout for each health check.
	HealthCheckTimeout time.Duration `koanf:"health_check_timeout"`
	// MaxTaskDuration is the maximum time a task can run before being considered stuck.
	MaxTaskDuration time.Duration `koanf:"max_task_duration"`
	// CleanupInterval is the interval between cleanup of idle workers above MinWorkers.
	CleanupInterval time.Duration `koanf:"cleanup_interval"`
	// IdleWorkerTTL is how long an idle worker above MinWorkers is kept before destruction.
	IdleWorkerTTL time.Duration `koanf:"idle_worker_ttl"`
	// ShutdownTimeout is the timeout for graceful shutdown.
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`
	// SandboxOpts contains default sandbox options for creating workers.
	SandboxOpts SandboxOpts `koanf:"sandbox_opts"`
}

// SandboxOpts contains default sandbox options.
type SandboxOpts struct {
	Image            string            `koanf:"image"`
	WorkingDir       string            `koanf:"working_dir"`
	Envvars          map[string]string `koanf:"envvars"`
	CPUPeriod        int64             `koanf:"cpu_period"`
	CPUQuota         int64             `koanf:"cpu_quota"`
	MemoryLimit      int64             `koanf:"memory_limit"`
	NetworkDisabled  bool              `koanf:"network_disabled"`
	ReadonlyRootfs   bool              `koanf:"readonly_rootfs"`
	Timeout          time.Duration     `koanf:"timeout"`
}

// DefaultConfig returns the default pool configuration.
func DefaultConfig() Config {
	return Config{
		MinWorkers:          2,
		MaxWorkers:          10,
		ScaleUpStep:         2,
		ScaleDownStep:       1,
		ScaleUpThreshold:    5,
		ScaleDownThreshold:  2,
		ScaleInterval:       10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		HealthCheckTimeout:  5 * time.Second,
		MaxTaskDuration:    5 * time.Minute,
		CleanupInterval:    60 * time.Second,
		IdleWorkerTTL:       5 * time.Minute,
		ShutdownTimeout:     30 * time.Second,
		SandboxOpts: SandboxOpts{
			Image:   "alpine:latest",
			Timeout: 30 * time.Second,
		},
	}
}

// Validate validates the pool configuration.
func (c Config) Validate() error {
	if c.MinWorkers < 0 {
		return ErrInvalidMinWorkers
	}
	if c.MaxWorkers < c.MinWorkers {
		return ErrMaxWorkersLessThanMin
	}
	if c.ScaleUpStep <= 0 {
		return ErrInvalidScaleUpStep
	}
	if c.ScaleDownStep <= 0 {
		return ErrInvalidScaleDownStep
	}
	if c.ScaleUpThreshold <= 0 {
		return ErrInvalidScaleUpThreshold
	}
	if c.ScaleDownThreshold >= c.ScaleUpThreshold {
		return ErrInvalidScaleThresholds
	}
	return nil
}

// ErrInvalidMinWorkers indicates MinWorkers is negative.
var ErrInvalidMinWorkers = &ConfigError{Field: "MinWorkers", Message: "must be non-negative"}

// ErrMaxWorkersLessThanMin indicates MaxWorkers is less than MinWorkers.
var ErrMaxWorkersLessThanMin = &ConfigError{Field: "MaxWorkers", Message: "must be >= MinWorkers"}

// ErrInvalidScaleUpStep indicates ScaleUpStep is not positive.
var ErrInvalidScaleUpStep = &ConfigError{Field: "ScaleUpStep", Message: "must be positive"}

// ErrInvalidScaleDownStep indicates ScaleDownStep is not positive.
var ErrInvalidScaleDownStep = &ConfigError{Field: "ScaleDownStep", Message: "must be positive"}

// ErrInvalidScaleUpThreshold indicates ScaleUpThreshold is not positive.
var ErrInvalidScaleUpThreshold = &ConfigError{Field: "ScaleUpThreshold", Message: "must be positive"}

// ErrInvalidScaleThresholds indicates scale thresholds are misconfigured.
var ErrInvalidScaleThresholds = &ConfigError{Field: "ScaleDownThreshold", Message: "must be < ScaleUpThreshold"}

// ConfigError represents a configuration validation error.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "invalid pool config: " + e.Field + " " + e.Message
}
