package controller

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/KshitijPatil98/klarity/api/v1alpha1"
)

// KlarityMonitorReconciler reconciles KlarityMonitor CRs. It is responsible for:
//   - Watching pods in the configured target namespaces.
//   - Detecting OOMKill and CrashLoopBackOff failures.
//   - Running existence-based deduplication before creating a KlarityDiagnosis CR.
//   - Creating KlarityDiagnosis CRs in Pending phase for the Diagnosis controller
//     to pick up and process.
type KlarityMonitorReconciler struct {
	client.Client
}

// Reconcile is the main reconciliation loop for KlarityMonitor.
//
// It runs whenever a KlarityMonitor spec changes or a pod in a watched namespace
// has its container status updated. It lists matching pods, detects failures,
// deduplicates against existing KlarityDiagnosis CRs, and creates new ones in
// Pending phase for the Diagnosis controller to process.
func (r *KlarityMonitorReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := slog.Default().With("controller", "KlarityMonitor", "name", req.Name, "namespace", req.Namespace)
	log.Info("reconcile triggered")

	// 1. Fetch the KlarityMonitor CR.
	var monitor v1alpha1.KlarityMonitor
	if err := r.Get(ctx, req.NamespacedName, &monitor); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("KlarityMonitor not found, skipping")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("fetching KlarityMonitor %q: %w", req.NamespacedName, err)
	}

	// Accumulate status changes on a local copy and flush at the end of each path.
	status := monitor.Status

	// 2. Check spec.enabled. If false, pause and stop processing.
	if !monitor.Spec.Enabled {
		log.Info("monitor is disabled, pausing")
		status.Phase = "Paused"
		if err := r.updateStatus(ctx, &monitor, status); err != nil {
			return reconcile.Result{}, fmt.Errorf("updating status to Paused: %w", err)
		}
		return reconcile.Result{}, nil
	}

	// 3. Resolve target namespaces from spec. The CRD enforces MinItems=1 so this
	// slice is always non-empty, but we guard defensively.
	targetNamespaces := monitor.Spec.TargetNamespaces
	if len(targetNamespaces) == 0 {
		targetNamespaces = []string{monitor.Namespace}
	}
	log.Info("resolved target namespaces", "namespaces", targetNamespaces)

	// 4. Build the label selector once and reuse it across all namespace list calls.
	var labelSelector labels.Selector
	if monitor.Spec.Selector != nil {
		var err error
		labelSelector, err = metav1LabelSelectorToSelector(monitor.Spec.Selector)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("parsing pod selector: %w", err)
		}
	} else {
		labelSelector = labels.Everything()
	}

	// List pods across all target namespaces and count the total matched.
	var matchedPods []corev1.Pod
	for _, ns := range targetNamespaces {
		var podList corev1.PodList
		if err := r.List(ctx, &podList,
			client.InNamespace(ns),
			client.MatchingLabelsSelector{Selector: labelSelector},
		); err != nil {
			return reconcile.Result{}, fmt.Errorf("listing pods in namespace %q: %w", ns, err)
		}
		matchedPods = append(matchedPods, podList.Items...)
	}
	log.Info("listed matched pods", "count", len(matchedPods))

	// 5. Update watchedPods count.
	status.WatchedPods = len(matchedPods)

	// 6. Set phase to Active — failure detection phases will be added in later phases.
	status.Phase = "Active"

	// 7. Persist status only if something actually changed to avoid unnecessary API
	// writes when pods churn frequently via the pod watch added in Phase 5.
	if err := r.updateStatus(ctx, &monitor, status); err != nil {
		return reconcile.Result{}, fmt.Errorf("updating KlarityMonitor status: %w", err)
	}

	log.Info("KlarityMonitor reconciled successfully",
		"phase", status.Phase,
		"watchedPods", status.WatchedPods)

	return reconcile.Result{}, nil
}

// updateStatus writes the given status back via the status subresource only if
// values have changed, avoiding spurious writes when the reconcile is triggered
// frequently by the pod watch.
func (r *KlarityMonitorReconciler) updateStatus(ctx context.Context, monitor *v1alpha1.KlarityMonitor, status v1alpha1.KlarityMonitorStatus) error {
	if monitor.Status.Phase == status.Phase &&
		monitor.Status.WatchedPods == status.WatchedPods &&
		monitor.Status.DiagnosesCreated == status.DiagnosesCreated &&
		monitor.Status.LastFailureDetected == status.LastFailureDetected {
		return nil
	}
	monitor.Status = status
	if err := r.Status().Update(ctx, monitor); err != nil {
		return fmt.Errorf("updating KlarityMonitor status: %w", err)
	}
	return nil
}

// metav1LabelSelectorToSelector converts a *metav1.LabelSelector to a labels.Selector
// suitable for use in client.MatchingLabelsSelector.
func metav1LabelSelectorToSelector(s *metav1.LabelSelector) (labels.Selector, error) {
	selector, err := metav1.LabelSelectorAsSelector(s)
	if err != nil {
		return nil, fmt.Errorf("converting label selector: %w", err)
	}
	return selector, nil
}

// SetupWithManager registers the KlarityMonitor controller with the manager.
func (r *KlarityMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KlarityMonitor{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
