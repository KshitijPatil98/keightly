//go:build envtest
// +build envtest

package v1alpha1_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	keightlyv1alpha1 "github.com/KshitijPatil98/keightly/api/v1alpha1"
)

func TestKeightlyMonitorCRDValidation(t *testing.T) {
	if envtestStartErr != nil {
		t.Skipf("skipping envtest CRD validation test: %v", envtestStartErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("monitor-validation-%d", time.Now().UnixNano()),
		},
	}
	if err := k8sClient.Create(ctx, namespace); err != nil {
		t.Fatalf("failed creating test namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), namespace)
	})

	t.Run("rejects missing spec", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "keightly.io/v1alpha1",
				"kind":       "KeightlyMonitor",
				"metadata": map[string]interface{}{
					"name":      "missing-spec",
					"namespace": namespace.Name,
				},
			},
		}

		err := k8sClient.Create(ctx, obj)
		if err == nil {
			_ = k8sClient.Delete(context.Background(), obj)
			t.Fatalf("expected validation error for missing spec, got nil")
		}
		if !apierrors.IsInvalid(err) {
			t.Fatalf("expected invalid error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "spec: Required value") {
			t.Fatalf("expected missing spec error, got: %v", err)
		}
	})

	t.Run("rejects empty failure type item", func(t *testing.T) {
		obj := &keightlyv1alpha1.KeightlyMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "empty-failure-type",
				Namespace: namespace.Name,
			},
			Spec: keightlyv1alpha1.KeightlyMonitorSpec{
				FailureTypes: []string{""},
			},
		}

		err := k8sClient.Create(ctx, obj)
		if err == nil {
			_ = k8sClient.Delete(context.Background(), obj)
			t.Fatalf("expected validation error for empty failure type, got nil")
		}
		if !apierrors.IsInvalid(err) {
			t.Fatalf("expected invalid error, got: %v", err)
		}
	})

	t.Run("rejects unsupported failure type item", func(t *testing.T) {
		obj := &keightlyv1alpha1.KeightlyMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unsupported-failure-type",
				Namespace: namespace.Name,
			},
			Spec: keightlyv1alpha1.KeightlyMonitorSpec{
				TargetNamespaces: []string{namespace.Name},
				FailureTypes:     []string{"ImagePullBackOff"},
			},
		}

		err := k8sClient.Create(ctx, obj)
		if err == nil {
			_ = k8sClient.Delete(context.Background(), obj)
			t.Fatalf("expected validation error for unsupported failure type, got nil")
		}
		if !apierrors.IsInvalid(err) {
			t.Fatalf("expected invalid error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "failureTypes") {
			t.Fatalf("expected failureTypes validation error, got: %v", err)
		}
	})

	t.Run("accepts valid monitor", func(t *testing.T) {
		obj := &keightlyv1alpha1.KeightlyMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "valid-monitor",
				Namespace: namespace.Name,
			},
			Spec: keightlyv1alpha1.KeightlyMonitorSpec{
				TargetNamespaces: []string{namespace.Name},
				FailureTypes:     []string{"OOMKill"},
			},
		}

		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("expected valid KeightlyMonitor, got error: %v", err)
		}
		t.Cleanup(func() {
			_ = k8sClient.Delete(context.Background(), obj)
		})
	})
}
