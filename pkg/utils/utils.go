package utils

import (
	"time"
)

const (
	VolumeName      = "repo-volume"
	VolumeMountPath = "/repo" // Mount point for the repo
	PvcStorage      = "1Gi"   // PVC storage size
	ServicePortName = "ssh"
	// image with git installed
	InitContainerImg = "alpine/git:latest"
	// Timeout for K8s API operations
	OperationTimeout = 2 * time.Minute
)
