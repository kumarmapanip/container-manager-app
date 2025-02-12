package config

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"sigs.k8s.io/yaml"
)

// ContainerConfig represents the user input for our container.
type ContainerConfig struct {
	// Pod name (DNS‑1123 compliant)
	Name string `json:"name" yaml:"name"`
	// Base image to use (with SSH server pre‑installed)
	BaseImage string `json:"baseImage" yaml:"baseImage"`
	// CPU resource request/limit (e.g. "500m")
	CPU string `json:"cpu" yaml:"cpu"`
	// Memory resource request/limit (e.g. "256Mi")
	Memory string `json:"memory" yaml:"memory"`
	// Public Git repository URL to clone
	RepoURL string `json:"repoUrl" yaml:"repoUrl"`
	// SSH port inside the container (default 22)
	SSHPort int `json:"sshPort" yaml:"sshPort"`
	// Kubernetes namespace (default "default")
	Namespace string `json:"namespace" yaml:"namespace"`
}

// LoadConfig reads and parses JSON/YAML config.
func LoadConfig(path string, logger *zap.SugaredLogger) (*ContainerConfig, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	var config ContainerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Set default values
	if config.SSHPort == 0 {
		config.SSHPort = 22
	}
	if config.Namespace == "" {
		config.Namespace = "default"
	}
	logger.Infof("Configuration loaded: %+v", config)
	return &config, nil
}
