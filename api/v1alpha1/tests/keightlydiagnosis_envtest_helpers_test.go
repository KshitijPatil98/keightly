//go:build envtest
// +build envtest

package v1alpha1_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	keightlyv1alpha1 "github.com/KshitijPatil98/keightly/api/v1alpha1"
)

func newMinimalDiagnosis(namespace, name string) *keightlyv1alpha1.KeightlyDiagnosis {
	return &keightlyv1alpha1.KeightlyDiagnosis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: keightlyv1alpha1.KeightlyDiagnosisSpec{
			FailureType:   "CrashLoopBackOff",
			PodName:       "app-7f6c5c8dd6-9nkt5",
			ContainerName: "app",
			Namespace:     namespace,
			NodeName:      "ip-10-0-0-12",
			OwnerRef: keightlyv1alpha1.OwnerRef{
				Kind: "Deployment",
				Name: "app",
			},
			MonitorRef: keightlyv1alpha1.MonitorRef{
				Name:      "team-monitor",
				Namespace: namespace,
			},
			DetectedAt: time.Now().UTC().Format(time.RFC3339),
			Context: keightlyv1alpha1.DiagnosisContext{
				RestartCount: 1,
				ExitCode:     1,
			},
		},
	}
}

func newMinimalDiagnosisUnstructured(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "keightly.io/v1alpha1",
			"kind":       "KeightlyDiagnosis",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"failureType":   "CrashLoopBackOff",
				"podName":       "app-7f6c5c8dd6-9nkt5",
				"containerName": "app",
				"namespace":     namespace,
				"nodeName":      "ip-10-0-0-12",
				"ownerRef": map[string]any{
					"kind": "Deployment",
					"name": "app",
				},
				"monitorRef": map[string]any{
					"name":      "team-monitor",
					"namespace": namespace,
				},
				"detectedAt": time.Now().UTC().Format(time.RFC3339),
				"context": map[string]any{
					"restartCount": 1,
					"exitCode":     1,
				},
			},
		},
	}
}

func createDiagnosisForStatusUpdate(t *testing.T, ctx context.Context, namespace, name string) *keightlyv1alpha1.KeightlyDiagnosis {
	t.Helper()

	obj := newMinimalDiagnosis(namespace, name)
	if err := k8sClient.Create(ctx, obj); err != nil {
		t.Fatalf("failed creating KeightlyDiagnosis for status validation: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), obj)
	})

	return obj
}

func expectInvalid(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected invalid error for %s, got nil", field)
	}
	if !apierrors.IsInvalid(err) {
		t.Fatalf("expected invalid error for %s, got: %v", field, err)
	}
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
