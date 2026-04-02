//go:build envtest
// +build envtest

package v1alpha1_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	keightlyv1alpha1 "github.com/KshitijPatil98/keightly/api/v1alpha1"
)

func TestKeightlyDiagnosisCRDValidation(t *testing.T) {
	if envtestStartErr != nil {
		t.Skipf("skipping envtest CRD validation test: %v", envtestStartErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("diagnosis-validation-%d", time.Now().UnixNano()),
		},
	}
	if err := k8sClient.Create(ctx, namespace); err != nil {
		t.Fatalf("failed creating test namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), namespace)
	})

	t.Run("accepts valid diagnosis with all fields populated", func(t *testing.T) {
		obj := &keightlyv1alpha1.KeightlyDiagnosis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uniqueName("full"),
				Namespace: namespace.Name,
			},
			Spec: keightlyv1alpha1.KeightlyDiagnosisSpec{
				FailureType:   "OOMKill",
				PodName:       "payments-api-6d9f7d6fb4-xkt2f",
				ContainerName: "app",
				Namespace:     namespace.Name,
				NodeName:      "ip-10-0-0-15",
				OwnerRef: keightlyv1alpha1.OwnerRef{
					Kind: "Deployment",
					Name: "payments-api",
				},
				RevisionHash: "6d9f7d6fb4",
				MonitorRef: keightlyv1alpha1.MonitorRef{
					Name:      "payments-monitor",
					Namespace: namespace.Name,
				},
				DetectedAt: time.Now().UTC().Format(time.RFC3339),
				Context: keightlyv1alpha1.DiagnosisContext{
					RestartCount: 4,
					ExitCode:     137,
					Resources: &keightlyv1alpha1.ResourceValues{
						Requests: map[string]string{
							"cpu":    "250m",
							"memory": "256Mi",
						},
						Limits: map[string]string{
							"cpu":    "500m",
							"memory": "512Mi",
						},
					},
					Sources: []keightlyv1alpha1.ContextSource{
						{
							Name: "logs",
							Data: "out of memory",
						},
						{
							Name: "events",
							Data: "Killing container app due to OOM",
						},
					},
				},
			},
		}

		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("expected valid KeightlyDiagnosis, got error: %v", err)
		}
		t.Cleanup(func() {
			_ = k8sClient.Delete(context.Background(), obj)
		})
	})

	t.Run("accepts minimal diagnosis with only required fields", func(t *testing.T) {
		obj := newMinimalDiagnosis(namespace.Name, uniqueName("minimal"))
		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("expected valid minimal KeightlyDiagnosis, got error: %v", err)
		}
		t.Cleanup(func() {
			_ = k8sClient.Delete(context.Background(), obj)
		})
	})

	t.Run("context.sources accepts empty list single source and multiple sources", func(t *testing.T) {
		testCases := []struct {
			name    string
			sources []any
		}{
			{
				name:    "empty-list",
				sources: []any{},
			},
			{
				name: "single-source",
				sources: []any{
					map[string]any{
						"name": "logs",
						"data": "panic: nil pointer dereference",
					},
				},
			},
			{
				name: "multiple-sources",
				sources: []any{
					map[string]any{
						"name": "logs",
						"data": "container OOMKilled",
					},
					map[string]any{
						"name": "events",
						"data": "Back-off restarting failed container",
					},
					map[string]any{
						"name": "topology",
						"data": "node under memory pressure",
					},
				},
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				obj := newMinimalDiagnosisUnstructured(namespace.Name, uniqueName("sources"))
				if err := unstructured.SetNestedSlice(obj.Object, tc.sources, "spec", "context", "sources"); err != nil {
					t.Fatalf("failed setting sources: %v", err)
				}

				if err := k8sClient.Create(ctx, obj); err != nil {
					t.Fatalf("expected valid sources=%s, got error: %v", tc.name, err)
				}
				t.Cleanup(func() {
					_ = k8sClient.Delete(context.Background(), obj)
				})
			})
		}
	})

	t.Run("accepts empty revisionHash", func(t *testing.T) {
		obj := newMinimalDiagnosisUnstructured(namespace.Name, uniqueName("revision"))
		if err := unstructured.SetNestedField(obj.Object, "", "spec", "revisionHash"); err != nil {
			t.Fatalf("failed setting revisionHash: %v", err)
		}

		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("expected empty revisionHash to be valid, got error: %v", err)
		}
		t.Cleanup(func() {
			_ = k8sClient.Delete(context.Background(), obj)
		})
	})

	t.Run("accepts resources with empty request and limit maps", func(t *testing.T) {
		obj := newMinimalDiagnosisUnstructured(namespace.Name, uniqueName("resources"))
		if err := unstructured.SetNestedMap(obj.Object, map[string]any{}, "spec", "context", "resources", "requests"); err != nil {
			t.Fatalf("failed setting empty requests map: %v", err)
		}
		if err := unstructured.SetNestedMap(obj.Object, map[string]any{}, "spec", "context", "resources", "limits"); err != nil {
			t.Fatalf("failed setting empty limits map: %v", err)
		}

		if err := k8sClient.Create(ctx, obj); err != nil {
			t.Fatalf("expected empty resources maps to be valid, got error: %v", err)
		}
		t.Cleanup(func() {
			_ = k8sClient.Delete(context.Background(), obj)
		})
	})
}

func TestKeightlyDiagnosisStatusValidation(t *testing.T) {
	if envtestStartErr != nil {
		t.Skipf("skipping envtest CRD validation test: %v", envtestStartErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("diagnosis-status-validation-%d", time.Now().UnixNano()),
		},
	}
	if err := k8sClient.Create(ctx, namespace); err != nil {
		t.Fatalf("failed creating test namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), namespace)
	})

	t.Run("category enum validation", func(t *testing.T) {
		testCases := []struct {
			name     string
			category string
			valid    bool
		}{
			{name: "application", category: "application", valid: true},
			{name: "infrastructure", category: "infrastructure", valid: true},
			{name: "configuration", category: "configuration", valid: true},
			{name: "dependency", category: "dependency", valid: true},
			{name: "invalid", category: "network", valid: false},
		}

		for i, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				obj := createDiagnosisForStatusUpdate(t, ctx, namespace.Name, fmt.Sprintf("category-%d-%d", i, time.Now().UnixNano()))
				obj.Status = keightlyv1alpha1.KeightlyDiagnosisStatus{
					Phase: "Diagnosed",
					Diagnosis: &keightlyv1alpha1.DiagnosisResult{
						Category:   tc.category,
						Confidence: 0.8,
					},
				}

				err := k8sClient.Status().Update(ctx, obj)
				if tc.valid && err != nil {
					t.Fatalf("expected valid category %q, got error: %v", tc.category, err)
				}
				if !tc.valid {
					expectInvalid(t, err, "category")
				}
			})
		}
	})

	t.Run("recommendation type enum validation", func(t *testing.T) {
		testCases := []struct {
			name    string
			recType string
			valid   bool
		}{
			{name: "resource", recType: "resource", valid: true},
			{name: "code", recType: "code", valid: true},
			{name: "infrastructure", recType: "infrastructure", valid: true},
			{name: "configuration", recType: "configuration", valid: true},
			{name: "invalid", recType: "runbook", valid: false},
		}

		for i, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				obj := createDiagnosisForStatusUpdate(t, ctx, namespace.Name, fmt.Sprintf("rectype-%d-%d", i, time.Now().UnixNano()))
				obj.Status = keightlyv1alpha1.KeightlyDiagnosisStatus{
					Phase: "Diagnosed",
					Diagnosis: &keightlyv1alpha1.DiagnosisResult{
						Category:   "application",
						Confidence: 0.8,
						Recommendations: []keightlyv1alpha1.Recommendation{
							{
								Action:   "adjust settings",
								Type:     tc.recType,
								Priority: "immediate",
							},
						},
					},
				}

				err := k8sClient.Status().Update(ctx, obj)
				if tc.valid && err != nil {
					t.Fatalf("expected valid recommendation type %q, got error: %v", tc.recType, err)
				}
				if !tc.valid {
					expectInvalid(t, err, "recommendation type")
				}
			})
		}
	})

	t.Run("recommendation priority enum validation", func(t *testing.T) {
		testCases := []struct {
			name     string
			priority string
			valid    bool
		}{
			{name: "immediate", priority: "immediate", valid: true},
			{name: "short-term", priority: "short-term", valid: true},
			{name: "long-term", priority: "long-term", valid: true},
			{name: "invalid", priority: "later", valid: false},
		}

		for i, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				obj := createDiagnosisForStatusUpdate(t, ctx, namespace.Name, fmt.Sprintf("priority-%d-%d", i, time.Now().UnixNano()))
				obj.Status = keightlyv1alpha1.KeightlyDiagnosisStatus{
					Phase: "Diagnosed",
					Diagnosis: &keightlyv1alpha1.DiagnosisResult{
						Category:   "application",
						Confidence: 0.8,
						Recommendations: []keightlyv1alpha1.Recommendation{
							{
								Action:   "adjust settings",
								Type:     "resource",
								Priority: tc.priority,
							},
						},
					},
				}

				err := k8sClient.Status().Update(ctx, obj)
				if tc.valid && err != nil {
					t.Fatalf("expected valid recommendation priority %q, got error: %v", tc.priority, err)
				}
				if !tc.valid {
					expectInvalid(t, err, "recommendation priority")
				}
			})
		}
	})

	t.Run("phase enum validation", func(t *testing.T) {
		testCases := []struct {
			name  string
			phase string
			valid bool
		}{
			{name: "Pending", phase: "Pending", valid: true},
			{name: "Gathering", phase: "Gathering", valid: true},
			{name: "Diagnosing", phase: "Diagnosing", valid: true},
			{name: "Diagnosed", phase: "Diagnosed", valid: true},
			{name: "Error", phase: "Error", valid: true},
			{name: "invalid", phase: "Running", valid: false},
		}

		for i, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				obj := createDiagnosisForStatusUpdate(t, ctx, namespace.Name, fmt.Sprintf("phase-%d-%d", i, time.Now().UnixNano()))
				obj.Status = keightlyv1alpha1.KeightlyDiagnosisStatus{
					Phase: tc.phase,
				}

				err := k8sClient.Status().Update(ctx, obj)
				if tc.valid && err != nil {
					t.Fatalf("expected valid phase %q, got error: %v", tc.phase, err)
				}
				if !tc.valid {
					expectInvalid(t, err, "phase")
				}
			})
		}
	})

	t.Run("confidence validation", func(t *testing.T) {
		testCases := []struct {
			name       string
			confidence float64
			valid      bool
		}{
			{name: "zero", confidence: 0.0, valid: true},
			{name: "one", confidence: 1.0, valid: true},
			{name: "negative", confidence: -0.1, valid: false},
			{name: "above-one", confidence: 1.1, valid: false},
		}

		for i, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				obj := createDiagnosisForStatusUpdate(t, ctx, namespace.Name, fmt.Sprintf("confidence-%d-%d", i, time.Now().UnixNano()))
				obj.Status = keightlyv1alpha1.KeightlyDiagnosisStatus{
					Phase: "Diagnosed",
					Diagnosis: &keightlyv1alpha1.DiagnosisResult{
						Category:   "application",
						Confidence: tc.confidence,
					},
				}

				err := k8sClient.Status().Update(ctx, obj)
				if tc.valid && err != nil {
					t.Fatalf("expected valid confidence %v, got error: %v", tc.confidence, err)
				}
				if !tc.valid {
					expectInvalid(t, err, "confidence")
				}
			})
		}
	})
}
