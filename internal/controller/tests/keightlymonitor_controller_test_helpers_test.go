package controller_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/KshitijPatil98/keightly/api/v1alpha1"
	"github.com/KshitijPatil98/keightly/internal/controller"
)

const (
	testMonitorName = "team-monitor"
)

func newTestMonitorReconciler(t *testing.T, objects ...client.Object) *controller.KeightlyMonitorReconciler {
	t.Helper()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))

	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.KeightlyMonitor{})
	if len(objects) > 0 {
		builder = builder.WithObjects(objects...)
	}

	return &controller.KeightlyMonitorReconciler{
		Client: builder.Build(),
	}
}

func newCountingMonitorReconciler(t *testing.T, objects ...client.Object) (*controller.KeightlyMonitorReconciler, *countingStatusWriter) {
	t.Helper()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))

	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.KeightlyMonitor{})
	if len(objects) > 0 {
		builder = builder.WithObjects(objects...)
	}

	baseClient := builder.Build()
	statusWriter := &countingStatusWriter{delegate: baseClient.Status()}
	c := &countingClient{
		Client:       baseClient,
		statusWriter: statusWriter,
	}

	return &controller.KeightlyMonitorReconciler{Client: c}, statusWriter
}

func monitorRequest(namespace, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func newKeightlyMonitor(
	namespace, name string,
	enabled bool,
	targetNamespaces []string,
	selector *metav1.LabelSelector,
) *v1alpha1.KeightlyMonitor {
	return &v1alpha1.KeightlyMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.KeightlyMonitorSpec{
			TargetNamespaces: targetNamespaces,
			FailureTypes:     []string{"OOMKill", "CrashLoopBackOff"},
			Selector:         selector,
			Enabled:          enabled,
		},
	}
}

func newPod(namespace, name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
	}
}

func labelSelector(matchLabels map[string]string) *metav1.LabelSelector {
	if matchLabels == nil {
		return nil
	}
	return &metav1.LabelSelector{MatchLabels: matchLabels}
}

func invalidLabelSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "app",
				Operator: "invalid-operator",
				Values:   []string{"api"},
			},
		},
	}
}

func getMonitor(t *testing.T, c client.Client, namespace, name string) v1alpha1.KeightlyMonitor {
	t.Helper()
	var monitor v1alpha1.KeightlyMonitor
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, &monitor); err != nil {
		t.Fatalf("failed to get KeightlyMonitor %s/%s: %v", namespace, name, err)
	}
	return monitor
}

type countingClient struct {
	client.Client
	statusWriter *countingStatusWriter
}

func (c *countingClient) Status() client.SubResourceWriter {
	return c.statusWriter
}

type countingStatusWriter struct {
	delegate client.SubResourceWriter
	updates  int
}

func (w *countingStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return w.delegate.Create(ctx, obj, subResource, opts...)
}

func (w *countingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	w.updates++
	return w.delegate.Update(ctx, obj, opts...)
}

func (w *countingStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return w.delegate.Patch(ctx, obj, patch, opts...)
}

func (w *countingStatusWriter) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
	return w.delegate.Apply(ctx, obj, opts...)
}
