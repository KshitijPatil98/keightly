package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	klarityiov1alpha1 "github.com/KshitijPatil98/klarity/api/v1alpha1"
	"github.com/KshitijPatil98/klarity/internal/controller"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))   // registers all core K8s types (Secret, Pod, etc.)
	utilruntime.Must(klarityiov1alpha1.AddToScheme(scheme)) // registers our CRD types
}

func main() {
	flag.Parse()

	// Structured JSON logs at Info level. Same format everywhere — no dev/prod split.
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))

	// Bridge slog into controller-runtime's logr so internal controller-runtime
	// log output (watches, queue, leader election, etc.) flows through the same
	// pipeline as our own slog calls.
	ctrl.SetLogger(logr.FromSlogHandler(handler))

	log := slog.Default().With("component", "main")

	cfg := ctrl.GetConfigOrDie()
	log.Info("connected to cluster", "apiServer", cfg.Host)

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		log.Error("unable to create manager", "error", err)
		os.Exit(1)
	}

	if err := (&controller.KlarityConfigReconciler{
		Client:     mgr.GetClient(),
		HTTPClient: &http.Client{},
	}).SetupWithManager(mgr); err != nil {
		log.Error("unable to set up KlarityConfig controller", "error", err)
		os.Exit(1)
	}

	if err := (&controller.KlarityMonitorReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		log.Error("unable to set up KlarityMonitor controller", "error", err)
		os.Exit(1)
	}

	log.Info("all controllers registered, starting manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error("manager exited with error", "error", err)
		os.Exit(1)
	}
}
