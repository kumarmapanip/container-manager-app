package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/kumarmapanip/containerctl/pkg/config"
	"github.com/kumarmapanip/containerctl/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/client-go/kubernetes"

	"go.uber.org/zap"
)

// CreatePVC creates (or reuses) a PersistentVolumeClaim.
func CreatePVC(ctx context.Context, clientset *kubernetes.Clientset, cfg *config.ContainerConfig, logger *zap.SugaredLogger) error {
	pvcName := cfg.Name + "-pvc"
	_, err := clientset.CoreV1().PersistentVolumeClaims(cfg.Namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err == nil {
		logger.Infof("PVC %q already exists, skipping creation.", pvcName)
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("getting PVC: %w", err)
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvcName,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(utils.PvcStorage),
				},
			},
		},
	}
	_, err = clientset.CoreV1().PersistentVolumeClaims(cfg.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating PVC: %w", err)
	}
	logger.Infof("PVC %q created successfully.", pvcName)
	return nil
}

// CreatePod creates a pod with an init container (to clone the repo) and the main SSH container.
func CreatePod(ctx context.Context, clientset *kubernetes.Clientset, cfg *config.ContainerConfig, logger *zap.SugaredLogger) error {
	podName := cfg.Name
	pvcName := cfg.Name + "-pvc"

	initContainer := corev1.Container{
		Name:  "repo-cloner",
		Image: utils.InitContainerImg,
		Command: []string{
			"sh",
			"-c",
			fmt.Sprintf(`
	set -ex
	
	if [ -d %s/.git ]; then
		echo "Repo already exists, pulling latest changes..."
		cd %s && git pull
	elif [ -z "$(ls -A %s 2>/dev/null)" ]; then
		echo "Directory %s is empty. Cloning..."
		git clone %s %s
	else
		echo "Directory %s is not empty and not a git repo. Removing it..."
		rm -rf %s/*
		git clone %s %s
	fi
	`,
				utils.VolumeMountPath,
				utils.VolumeMountPath,
				utils.VolumeMountPath,
				utils.VolumeMountPath,
				cfg.RepoURL,
				utils.VolumeMountPath,
				utils.VolumeMountPath,
				utils.VolumeMountPath,
				cfg.RepoURL,
				utils.VolumeMountPath,
			),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      utils.VolumeName,
				MountPath: utils.VolumeMountPath,
			},
		},
	}

	mainContainer := corev1.Container{
		Name:    "ssh-container",
		Image:   cfg.BaseImage,
		Command: []string{"sh", "-c", "echo Starting SSH server...; /usr/sbin/sshd -D"},
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: int32(cfg.SSHPort),
				Name:          utils.ServicePortName,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				"cpu":    resource.MustParse(cfg.CPU),
				"memory": resource.MustParse(cfg.Memory),
			},
			Limits: corev1.ResourceList{
				"cpu":    resource.MustParse(cfg.CPU),
				"memory": resource.MustParse(cfg.Memory),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      utils.VolumeName,
				MountPath: utils.VolumeMountPath,
			},
		},

		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(cfg.SSHPort),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       5,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(cfg.SSHPort),
				},
			},
			InitialDelaySeconds: 20,
			PeriodSeconds:       10,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   podName,
			Labels: map[string]string{"app": cfg.Name},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{initContainer},
			Containers:     []corev1.Container{mainContainer},
			Volumes: []corev1.Volume{
				{
					Name: utils.VolumeName,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}
	_, err := clientset.CoreV1().Pods(cfg.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating Pod: %w", err)
	}
	logger.Infof("Pod %q created successfully.", podName)
	return nil
}

// CreateService exposes the pod’s SSH port externally via a NodePort Service.
func CreateService(ctx context.Context, clientset *kubernetes.Clientset, cfg *config.ContainerConfig, logger *zap.SugaredLogger) error {
	serviceName := cfg.Name + "-ssh"
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceName,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": cfg.Name},
			Ports: []corev1.ServicePort{
				{
					Name:       utils.ServicePortName,
					Port:       int32(cfg.SSHPort),
					TargetPort: intstr.FromInt(cfg.SSHPort),
				},
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}
	_, err := clientset.CoreV1().Services(cfg.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating Service: %w", err)
	}
	logger.Infof("Service %q created successfully.", serviceName)
	return nil
}

// DeletePod removes the pod.
func DeletePod(ctx context.Context, clientset *kubernetes.Clientset, cfg *config.ContainerConfig, logger *zap.SugaredLogger) error {
	if err := clientset.CoreV1().Pods(cfg.Namespace).Delete(ctx, cfg.Name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting Pod: %w", err)
	}
	logger.Infof("Pod %q deleted.", cfg.Name)
	return nil
}

// DeleteService removes the Service.
func DeleteService(ctx context.Context, clientset *kubernetes.Clientset, cfg *config.ContainerConfig, logger *zap.SugaredLogger) error {
	serviceName := cfg.Name + "-ssh"
	if err := clientset.CoreV1().Services(cfg.Namespace).Delete(ctx, serviceName, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("deleting Service: %w", err)
	}
	logger.Infof("Service %q deleted.", serviceName)
	return nil
}

// WaitForPodReady polls (with timeout) until the pod is in Running state.
func WaitForPodReady(ctx context.Context, clientset *kubernetes.Clientset, cfg *config.ContainerConfig, logger *zap.SugaredLogger) error {
	podName := cfg.Name
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for pod %q to be ready", podName)
		case <-ticker.C:
			pod, err := clientset.CoreV1().Pods(cfg.Namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				logger.Warnf("Error getting pod %q: %v", podName, err)
				continue
			}
			if pod.Status.Phase == corev1.PodRunning {
				logger.Infof("Pod %q is running.", podName)
				return nil
			}
			logger.Infof("Waiting for pod %q (current phase: %s)...", podName, pod.Status.Phase)
		}
	}
}
