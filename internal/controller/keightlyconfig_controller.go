package controller

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/KshitijPatil98/keightly/api/v1alpha1"
)

const (
	// operatorNamespace is the namespace where the operator runs and where all
	// credential Secrets are expected to live. Never configurable — keeps credential
	// isolation simple and auditable.
	operatorNamespace = "keightly-system"

	// configName is the required name of the singleton KeightlyConfig CR,
	// enforced at the CRD level via a CEL admission rule.
	configName = "keightly"

	// anthropicHealthURL is the lightweight endpoint used to verify API key
	// validity and network reachability without triggering any billing.
	anthropicHealthURL = "https://api.anthropic.com/v1/models"

	// anthropicAPIVersion is the Anthropic API version header value required
	// on every request to the Anthropic API.
	anthropicAPIVersion = "2023-06-01"

	// healthCheckTimeout is the per-request deadline for the AI API health probe.
	healthCheckTimeout = 10 * time.Second

	// requeueSecretMissing is the requeue delay when the API key Secret does not
	// exist, the expected key is absent, or the key value is empty. The user must
	// create or fix the Secret — no point retrying fast.
	requeueSecretMissing = 30 * time.Second

	// requeueAPIKeyInvalid is the requeue delay when the AI API returns 401.
	// The user must rotate the API key — no point hammering the endpoint.
	requeueAPIKeyInvalid = 5 * time.Minute

	// requeueAPIError is the requeue delay when the AI API is unreachable or
	// returns an unexpected status — likely a transient network or service issue.
	requeueAPIError = 1 * time.Minute
)

// KeightlyConfigReconciler reconciles KeightlyConfig CRs. It is responsible for:
//   - Validating that the AI API key Secret exists and is well-formed.
//   - Verifying live connectivity to the AI API.
//   - Counting enabled KeightlyMonitor CRs and reporting the total.
//   - Keeping status.active, status.connectedMonitors, and status.lastHealthCheck
//     up to date via the status subresource.
//
// The controller deliberately does no diagnosis work — it owns only the
// configuration and health reporting lifecycle.
type KeightlyConfigReconciler struct {
	client.Client

	// HTTPClient is used for the AI API health probe. It must be set before
	// the controller is started. Injected so tests can substitute a fake server.
	HTTPClient *http.Client
}

// Reconcile is the main reconciliation loop for KeightlyConfig.
//
// It runs whenever the singleton KeightlyConfig CR, a Secret in keightly-system,
// or any KeightlyMonitor CR changes. It validates the AI secret, probes the API,
// counts enabled monitors, and writes all observations back to status.
func (r *KeightlyConfigReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := slog.Default().With("controller", "KeightlyConfig", "name", req.Name)
	log.Info("reconcile triggered")

	// 1. Fetch the singleton KeightlyConfig CR.
	var config v1alpha1.KeightlyConfig
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		if apierrors.IsNotFound(err) {
			// CR deleted — nothing to reconcile.
			log.Info("KeightlyConfig not found, skipping")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("fetching KeightlyConfig %q: %w", req.Name, err)
	}

	// Accumulate status changes on a local copy and flush at the end of each path.
	status := config.Status

	// 2. Resolve and validate the AI API key from the referenced Secret.
	log.Info("resolving AI API key",
		"secret", config.Spec.AI.APIKeySecretRef.Name,
		"key", config.Spec.AI.APIKeySecretRef.Key)
	apiKey, err := r.resolveAPIKey(ctx, config.Spec.AI.APIKeySecretRef)
	if err != nil {
		log.Error("AI secret validation failed",
			"error", err,
			"secret", config.Spec.AI.APIKeySecretRef.Name,
			"key", config.Spec.AI.APIKeySecretRef.Key,
			"requeueAfter", requeueSecretMissing)
		status.Active = false
		status.Message = err.Error()
		if updateErr := r.updateStatus(ctx, &config, status); updateErr != nil {
			log.Error("failed to update status after secret error", "error", updateErr)
			return reconcile.Result{}, fmt.Errorf("updating status after secret error: %w", updateErr)
		}
		return reconcile.Result{RequeueAfter: requeueSecretMissing}, nil
	}
	log.Info("AI API key resolved successfully")

	// 3. Verify live connectivity to the AI API.
	log.Info("verifying AI API connectivity", "url", anthropicHealthURL)
	requeue, err := r.verifyAPIConnectivity(ctx, apiKey)
	if err != nil {
		log.Error("AI API connectivity check failed",
			"error", err,
			"url", anthropicHealthURL,
			"requeueAfter", requeue)
		status.Active = false
		status.Message = err.Error()
		if updateErr := r.updateStatus(ctx, &config, status); updateErr != nil {
			log.Error("failed to update status after API connectivity error", "error", updateErr)
			return reconcile.Result{}, fmt.Errorf("updating status after API connectivity error: %w", updateErr)
		}
		return reconcile.Result{RequeueAfter: requeue}, nil
	}
	log.Info("AI API connectivity verified")

	status.Active = true
	status.Message = ""
	status.LastHealthCheck = time.Now().UTC().Format(time.RFC3339)

	// 4. Count enabled KeightlyMonitor CRs across all namespaces.
	count, err := r.countEnabledMonitors(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("counting enabled monitors: %w", err)
	}
	status.ConnectedMonitors = count
	log.Info("counted enabled monitors", "count", count)

	// 5. Persist status.
	if err := r.updateStatus(ctx, &config, status); err != nil {
		return reconcile.Result{}, fmt.Errorf("updating KeightlyConfig status: %w", err)
	}

	log.Info("KeightlyConfig reconciled successfully",
		"active", status.Active,
		"connectedMonitors", status.ConnectedMonitors,
		"lastHealthCheck", status.LastHealthCheck)

	// Healthy — no explicit requeue. Controller-runtime re-triggers on any
	// watched resource change.
	return reconcile.Result{}, nil
}

// resolveAPIKey fetches the API key value from the Secret referenced in the spec.
// Returns an error if the Secret is missing, the key is absent, or the value is empty.
func (r *KeightlyConfigReconciler) resolveAPIKey(ctx context.Context, ref v1alpha1.SecretKeyRef) (string, error) {
	var secret corev1.Secret
	secretKey := types.NamespacedName{Namespace: operatorNamespace, Name: ref.Name}
	if err := r.Get(ctx, secretKey, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("secret %q not found in namespace %q", ref.Name, operatorNamespace)
		}
		return "", fmt.Errorf("fetching secret %q: %w", ref.Name, err)
	}

	val, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %q", ref.Key, ref.Name)
	}
	if len(val) == 0 {
		return "", fmt.Errorf("key %q in secret %q is empty", ref.Key, ref.Name)
	}

	return string(val), nil
}

// verifyAPIConnectivity makes a lightweight GET to the Anthropic models endpoint
// to confirm that the API key is valid and the API is reachable from this cluster.
//
// Returns the requeue duration and an error on failure:
//   - 401 Unauthorized → (requeueAPIKeyInvalid, error) — user must fix the key
//   - timeout or other error → (requeueAPIError, error) — likely transient
//   - 200 OK → (0, nil)
func (r *KeightlyConfigReconciler) verifyAPIConnectivity(ctx context.Context, apiKey string) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, anthropicHealthURL, nil)
	if err != nil {
		return requeueAPIError, fmt.Errorf("building API health check request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return requeueAPIError, fmt.Errorf("Anthropic API unreachable: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return 0, nil
	case http.StatusUnauthorized:
		return requeueAPIKeyInvalid, fmt.Errorf("Anthropic API returned 401: API key is invalid")
	default:
		return requeueAPIError, fmt.Errorf("Anthropic API returned unexpected status %d", resp.StatusCode)
	}
}

// countEnabledMonitors lists all KeightlyMonitor CRs across all namespaces
// and returns the count of those with spec.enabled = true.
func (r *KeightlyConfigReconciler) countEnabledMonitors(ctx context.Context) (int, error) {
	var monitors v1alpha1.KeightlyMonitorList
	if err := r.List(ctx, &monitors); err != nil {
		return 0, fmt.Errorf("listing KeightlyMonitors: %w", err)
	}

	count := 0
	for _, m := range monitors.Items {
		if m.Spec.Enabled {
			count++
		}
	}
	return count, nil
}

// updateStatus writes the given status back via the status subresource client.
func (r *KeightlyConfigReconciler) updateStatus(ctx context.Context, config *v1alpha1.KeightlyConfig, status v1alpha1.KeightlyConfigStatus) error {
	config.Status = status
	if err := r.Status().Update(ctx, config); err != nil {
		return fmt.Errorf("updating KeightlyConfig status: %w", err)
	}
	return nil
}

// mapSecretToConfig maps any Secret event in keightly-system to a reconcile
// request for the singleton KeightlyConfig CR.
func (r *KeightlyConfigReconciler) mapSecretToConfig(_ context.Context, _ client.Object) []reconcile.Request {
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: configName}},
	}
}

// mapMonitorToConfig maps any KeightlyMonitor event to a reconcile request for
// the singleton KeightlyConfig CR, so connectedMonitors stays current.
func (r *KeightlyConfigReconciler) mapMonitorToConfig(_ context.Context, _ client.Object) []reconcile.Request {
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: configName}},
	}
}

// SetupWithManager registers the KeightlyConfig controller with the manager and
// configures its watches:
//   - KeightlyConfig CRs (primary resource, spec changes only — status updates are
//     filtered out via GenerationChangedPredicate to prevent reconcile loops)
//   - Secrets in keightly-system (re-validate if the API key secret changes)
//   - KeightlyMonitor CRs in any namespace (recount connectedMonitors on spec changes only)
func (r *KeightlyConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// GenerationChangedPredicate ensures status-only updates (e.g. our own
		// status writes) do not re-trigger reconciliation. Only spec changes,
		// creation, and deletion fire a reconcile via this watch. Secondary
		// watches (Secrets, Monitors) are unaffected by this predicate.
		For(&v1alpha1.KeightlyConfig{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.mapSecretToConfig),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return obj.GetNamespace() == operatorNamespace
			})),
		).
		Watches(
			&v1alpha1.KeightlyMonitor{},
			handler.EnqueueRequestsFromMapFunc(r.mapMonitorToConfig),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Complete(r)
}
