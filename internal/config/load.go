// Package config implements configuration management using koanf.
package config

// Load loads configuration from YAML file and environment variables.
func Load(path string) (*Config, error) {
	// TODO: Implement proper koanf loading
	_ = path
	return &Config{}, nil
}

// MustLoad loads configuration or panics.
func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		panic("failed to load config: " + err.Error())
	}
	return cfg
}
