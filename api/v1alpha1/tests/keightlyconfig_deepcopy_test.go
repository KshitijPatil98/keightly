//go:build !envtest
// +build !envtest

package v1alpha1_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	keightlyv1alpha1 "github.com/KshitijPatil98/keightly/api/v1alpha1"
)

func TestKeightlyConfigDeepCopy(t *testing.T) {
	original := &keightlyv1alpha1.KeightlyConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "keightly",
			Labels: map[string]string{
				"team": "platform",
			},
		},
		Spec: keightlyv1alpha1.KeightlyConfigSpec{
			AI: keightlyv1alpha1.AIConfig{
				Provider: "anthropic",
				Model:    "claude-opus-4-6",
				APIKeySecretRef: keightlyv1alpha1.SecretKeyRef{
					Name: "keightly-secrets",
					Key:  "anthropic-api-key",
				},
			},
			DiagnosisRetention:     "72h",
			MaxConcurrentDiagnoses: 5,
		},
		Status: keightlyv1alpha1.KeightlyConfigStatus{
			Active:            true,
			ConnectedMonitors: 2,
			LastHealthCheck:   "2026-01-01T00:00:00Z",
		},
	}

	cloned := original.DeepCopy()
	if cloned == nil {
		t.Fatalf("DeepCopy returned nil")
	}

	cloned.Labels["team"] = "changed"
	cloned.Spec.AI.Model = "changed-model"
	cloned.Status.Active = false
	cloned.Status.ConnectedMonitors = 99

	if original.Labels["team"] != "platform" {
		t.Fatalf("expected original labels unchanged, got %q", original.Labels["team"])
	}
	if original.Spec.AI.Model != "claude-opus-4-6" {
		t.Fatalf("expected original spec.ai.model unchanged, got %q", original.Spec.AI.Model)
	}
	if !original.Status.Active {
		t.Fatalf("expected original status.active unchanged")
	}
	if original.Status.ConnectedMonitors != 2 {
		t.Fatalf("expected original connectedMonitors unchanged, got %d", original.Status.ConnectedMonitors)
	}
}
