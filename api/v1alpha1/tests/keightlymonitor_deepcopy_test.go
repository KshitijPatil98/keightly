//go:build !envtest
// +build !envtest

package v1alpha1_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	keightlyv1alpha1 "github.com/KshitijPatil98/keightly/api/v1alpha1"
)

func TestKeightlyMonitorDeepCopy(t *testing.T) {
	original := &keightlyv1alpha1.KeightlyMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-monitor",
			Namespace: "payments",
			Labels: map[string]string{
				"team": "payments",
			},
		},
		Spec: keightlyv1alpha1.KeightlyMonitorSpec{
			TargetNamespaces: []string{"payments", "billing"},
			FailureTypes:     []string{"OOMKill", "CrashLoopBackOff"},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "payments-api",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "tier",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"backend"},
					},
				},
			},
			Severity: "warning",
			Enabled:  true,
		},
		Status: keightlyv1alpha1.KeightlyMonitorStatus{
			Phase:               "Active",
			WatchedPods:         10,
			DiagnosesCreated:    4,
			LastFailureDetected: "2026-01-01T00:00:00Z",
		},
	}

	cloned := original.DeepCopy()
	if cloned == nil {
		t.Fatalf("DeepCopy returned nil")
	}

	cloned.Labels["team"] = "changed"
	cloned.Spec.TargetNamespaces[0] = "changed-ns"
	cloned.Spec.FailureTypes[0] = "ChangedFailure"
	cloned.Spec.Selector.MatchLabels["app"] = "changed-app"
	cloned.Spec.Selector.MatchExpressions[0].Values[0] = "changed-tier"
	cloned.Status.Phase = "Paused"
	cloned.Status.WatchedPods = 0

	if original.Labels["team"] != "payments" {
		t.Fatalf("expected original labels unchanged, got %q", original.Labels["team"])
	}
	if original.Spec.TargetNamespaces[0] != "payments" {
		t.Fatalf("expected original targetNamespaces unchanged, got %q", original.Spec.TargetNamespaces[0])
	}
	if original.Spec.FailureTypes[0] != "OOMKill" {
		t.Fatalf("expected original failureTypes unchanged, got %q", original.Spec.FailureTypes[0])
	}
	if original.Spec.Selector.MatchLabels["app"] != "payments-api" {
		t.Fatalf("expected original selector.matchLabels unchanged, got %q", original.Spec.Selector.MatchLabels["app"])
	}
	if original.Spec.Selector.MatchExpressions[0].Values[0] != "backend" {
		t.Fatalf("expected original selector.matchExpressions unchanged, got %q", original.Spec.Selector.MatchExpressions[0].Values[0])
	}
	if original.Status.Phase != "Active" {
		t.Fatalf("expected original status.phase unchanged, got %q", original.Status.Phase)
	}
	if original.Status.WatchedPods != 10 {
		t.Fatalf("expected original status.watchedPods unchanged, got %d", original.Status.WatchedPods)
	}
}
