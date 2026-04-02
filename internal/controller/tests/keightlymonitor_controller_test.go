package controller_test

import (
	"context"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcile_MonitorDoesNotExist(t *testing.T) {
	r := newTestMonitorReconciler(t)

	result, err := r.Reconcile(context.Background(), monitorRequest("team-a", testMonitorName))
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("expected no requeue, got %+v", result)
	}
}

func TestReconcile_DisabledMonitorSetsPausedPhase(t *testing.T) {
	monitor := newKeightlyMonitor("team-a", testMonitorName, false, []string{"team-a"}, nil)
	r := newTestMonitorReconciler(t, monitor)

	result, err := r.Reconcile(context.Background(), monitorRequest("team-a", testMonitorName))
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("expected no requeue, got %+v", result)
	}

	got := getMonitor(t, r.Client, "team-a", testMonitorName)
	if got.Status.Phase != "Paused" {
		t.Fatalf("expected status.phase=Paused, got %q", got.Status.Phase)
	}
}

func TestReconcile_UpdatesWatchedPodsAndPhase(t *testing.T) {
	testCases := []struct {
		name             string
		monitorNamespace string
		targetNamespaces []string
		selector         map[string]string
		pods             []podFixture
		wantWatchedPods  int
	}{
		{
			name:             "counts matching pods across target namespaces",
			monitorNamespace: "ops",
			targetNamespaces: []string{"team-a", "team-b"},
			selector:         map[string]string{"app": "api"},
			pods: []podFixture{
				{namespace: "team-a", name: "api-a", labels: map[string]string{"app": "api"}},
				{namespace: "team-b", name: "api-b", labels: map[string]string{"app": "api"}},
				{namespace: "team-b", name: "worker-b", labels: map[string]string{"app": "worker"}},
				{namespace: "team-c", name: "api-c", labels: map[string]string{"app": "api"}},
			},
			wantWatchedPods: 2,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			monitor := newKeightlyMonitor(tc.monitorNamespace, testMonitorName, true, tc.targetNamespaces, labelSelector(tc.selector))
			objects := []client.Object{monitor}
			for _, p := range tc.pods {
				objects = append(objects, newPod(p.namespace, p.name, p.labels))
			}

			r := newTestMonitorReconciler(t, objects...)
			result, err := r.Reconcile(context.Background(), monitorRequest(tc.monitorNamespace, testMonitorName))
			if err != nil {
				t.Fatalf("reconcile returned unexpected error: %v", err)
			}
			if result.Requeue || result.RequeueAfter != 0 {
				t.Fatalf("expected no requeue, got %+v", result)
			}

			got := getMonitor(t, r.Client, tc.monitorNamespace, testMonitorName)
			if got.Status.Phase != "Active" {
				t.Fatalf("expected status.phase=Active, got %q", got.Status.Phase)
			}
			if got.Status.WatchedPods != tc.wantWatchedPods {
				t.Fatalf("expected status.watchedPods=%d, got %d", tc.wantWatchedPods, got.Status.WatchedPods)
			}
		})
	}
}

func TestReconcile_InvalidSelectorReturnsError(t *testing.T) {
	monitor := newKeightlyMonitor("team-a", testMonitorName, true, []string{"team-a"}, invalidLabelSelector())
	r := newTestMonitorReconciler(t, monitor)

	_, err := r.Reconcile(context.Background(), monitorRequest("team-a", testMonitorName))
	if err == nil {
		t.Fatalf("expected reconcile error for invalid selector, got nil")
	}
	if !strings.Contains(err.Error(), "parsing pod selector") {
		t.Fatalf("expected selector parse error, got: %v", err)
	}
}

func TestReconcile_SkipsStatusWriteWhenNoStatusChange(t *testing.T) {
	monitor := newKeightlyMonitor("team-a", testMonitorName, true, []string{"team-a"}, labelSelector(map[string]string{"app": "api"}))
	monitor.Status.Phase = "Active"
	monitor.Status.WatchedPods = 1
	monitor.Status.DiagnosesCreated = 3
	monitor.Status.LastFailureDetected = "2026-03-31T12:00:00Z"

	pod := newPod("team-a", "api-1", map[string]string{"app": "api"})
	r, statusWriter := newCountingMonitorReconciler(t, monitor, pod)

	result, err := r.Reconcile(context.Background(), monitorRequest("team-a", testMonitorName))
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("expected no requeue, got %+v", result)
	}
	if statusWriter.updates != 0 {
		t.Fatalf("expected no status update when status is unchanged, got %d updates", statusWriter.updates)
	}
}

func TestReconcile_WritesStatusWhenWatchedPodsChanges(t *testing.T) {
	monitor := newKeightlyMonitor("team-a", testMonitorName, true, []string{"team-a"}, labelSelector(map[string]string{"app": "api"}))
	monitor.Status.Phase = "Active"
	monitor.Status.WatchedPods = 0

	pod := newPod("team-a", "api-1", map[string]string{"app": "api"})
	r, statusWriter := newCountingMonitorReconciler(t, monitor, pod)

	result, err := r.Reconcile(context.Background(), monitorRequest("team-a", testMonitorName))
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("expected no requeue, got %+v", result)
	}
	if statusWriter.updates != 1 {
		t.Fatalf("expected exactly one status update, got %d", statusWriter.updates)
	}

	got := getMonitor(t, r.Client, "team-a", testMonitorName)
	if got.Status.WatchedPods != 1 {
		t.Fatalf("expected status.watchedPods=1, got %d", got.Status.WatchedPods)
	}
}

type podFixture struct {
	namespace string
	name      string
	labels    map[string]string
}
