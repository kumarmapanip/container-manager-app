// main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/kumarmapanip/containerctl/pkg/config"
	"github.com/kumarmapanip/containerctl/pkg/k8s"
	"github.com/kumarmapanip/containerctl/pkg/utils"
	"github.com/joho/godotenv"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"go.uber.org/zap"
)

var (
	kubeConfig string
)

// getClientset initializes the Kubernetes clientset using kubeconfig.
func getClientset(logger *zap.SugaredLogger) (*kubernetes.Clientset, error) {
	if kubeConfig == "" {
		fmt.Println("invalid kubeconfig")
		return nil, nil
	}
	fmt.Println(kubeConfig)
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("building kubeconfig: %w", err)
	}

	return kubernetes.NewForConfig(config)
}

func init() {
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("Warning: .env file not found or unable to load, using system environment variables.")
		return
	}

	kubeConfig = os.Getenv("KUBECONFIG")
}


func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	sugar := logger.Sugar()

	if err := godotenv.Load(); err != nil {
		fmt.Println("Warning: .env file not found or unable to load, using system environment variables.")
	}

	startCmd := flag.NewFlagSet("start", flag.ExitOnError)
	stopCmd := flag.NewFlagSet("stop", flag.ExitOnError)
	relaunchCmd := flag.NewFlagSet("relaunch", flag.ExitOnError)

	startConfigFile := startCmd.String("config", "", "Path to JSON/YAML config file")
	stopConfigFile := stopCmd.String("config", "", "Path to JSON/YAML config file")
	relaunchConfigFile := relaunchCmd.String("config", "", "Path to JSON/YAML config file")

	if len(os.Args) < 2 {
		sugar.Error("Expected 'start', 'stop' or 'relaunch' subcommand")
		os.Exit(1)
	}

	// Initialize Kubernetes clientset
	clientset, err := getClientset(sugar)
	if err != nil {
		sugar.Fatalf("Error creating Kubernetes client: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), utils.OperationTimeout)
	defer cancel()

	switch os.Args[1] {
	case "start":
		startCmd.Parse(os.Args[2:])
		if *startConfigFile == "" {
			sugar.Fatal("Please provide a config file via the -config flag.")
		}
		cfg, err := config.LoadConfig(*startConfigFile, sugar)
		if err != nil {
			sugar.Fatalf("Error loading config: %v", err)
		}
		// Create or reuse the PVC.
		if err := k8s.CreatePVC(ctx, clientset, cfg, sugar); err != nil {
			sugar.Fatalf("PVC error: %v", err)
		}
		// Create the Pod (with init container) and Service.
		if err := k8s.CreatePod(ctx, clientset, cfg, sugar); err != nil {
			sugar.Fatalf("Pod creation error: %v", err)
		}
		if err := k8s.CreateService(ctx, clientset, cfg, sugar); err != nil {
			sugar.Fatalf("Service creation error: %v", err)
		}
		if err := k8s.WaitForPodReady(ctx, clientset, cfg, sugar); err != nil {
			sugar.Fatalf("Pod readiness error: %v", err)
		}
		sugar.Info("Container started successfully. You can now SSH into it using the node IP and the assigned NodePort.")

	case "stop":
		stopCmd.Parse(os.Args[2:])
		if *stopConfigFile == "" {
			sugar.Fatal("Please provide a config file via the -config flag.")
		}
		cfg, err := config.LoadConfig(*stopConfigFile, sugar)
		if err != nil {
			sugar.Fatalf("Error loading config: %v", err)
		}
		if err := k8s.DeletePod(ctx, clientset, cfg, sugar); err != nil {
			sugar.Fatalf("Error deleting pod: %v", err)
		}
		if err := k8s.DeleteService(ctx, clientset, cfg, sugar); err != nil {
			sugar.Fatalf("Error deleting service: %v", err)
		}
		sugar.Info("Container stopped successfully. Repository data is persisted in the PVC.")

	case "relaunch":
		relaunchCmd.Parse(os.Args[2:])
		if *relaunchConfigFile == "" {
			sugar.Fatal("Please provide a config file via the -config flag.")
		}
		cfg, err := config.LoadConfig(*relaunchConfigFile, sugar)
		if err != nil {
			sugar.Fatalf("Error loading config: %v", err)
		}
		if err := k8s.CreatePod(ctx, clientset, cfg, sugar); err != nil {
			sugar.Fatalf("Pod creation error: %v", err)
		}
		if err := k8s.CreateService(ctx, clientset, cfg, sugar); err != nil {
			sugar.Fatalf("Service creation error: %v", err)
		}
		if err := k8s.WaitForPodReady(ctx, clientset, cfg, sugar); err != nil {
			sugar.Fatalf("Pod readiness error: %v", err)
		}
		sugar.Info("Container relaunched successfully. Previous repository data is available.")

	default:
		sugar.Error("Expected subcommand 'start', 'stop' or 'relaunch'")
		os.Exit(1)
	}
}
