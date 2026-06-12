package clients

import (
	"fmt"
	"os"
	"path/filepath"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var kubeLog = l.Logger.WithField("logger", "KubeClient")

// NewKubeClientSet creates a new Kubernetes clientset
func NewKubeClientSet() (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	// Try to get in-cluster config (when running inside a Kubernetes cluster)
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fallback to local kubeconfig file (for local development/testing)
		kubeConfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		if _, err := os.Stat(kubeConfigPath); os.IsNotExist(err) { //nolint:gosec // kubeConfigPath is derived from HOME env var, not user input
			return nil, fmt.Errorf("kubeconfig file not found at %s and not in-cluster: %s", kubeConfigPath, err.Error())
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build kubeconfig: %s", err.Error())
		}
		kubeLog.Info("Using local kubeconfig")
	} else {
		kubeLog.Info("Using in-cluster config")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %s", err.Error())
	}

	return clientset, nil
}
