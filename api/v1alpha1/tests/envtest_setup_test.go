//go:build envtest
// +build envtest

package v1alpha1_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	keightlyv1alpha1 "github.com/KshitijPatil98/keightly/api/v1alpha1"
)

var (
	testEnv         *envtest.Environment
	k8sClient       ctrlclient.Client
	envtestStartErr error
)

func TestMain(m *testing.M) {
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		envtestStartErr = err
		if os.Getenv("CI") != "" {
			panic(fmt.Sprintf("failed to start envtest in CI: %v", err))
		}
		code := m.Run()
		os.Exit(code)
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(keightlyv1alpha1.AddToScheme(scheme))

	k8sClient, err = ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		panic(fmt.Sprintf("failed to create client: %v", err))
	}

	code := m.Run()

	if err := testEnv.Stop(); err != nil {
		panic(fmt.Sprintf("failed to stop envtest: %v", err))
	}

	os.Exit(code)
}
