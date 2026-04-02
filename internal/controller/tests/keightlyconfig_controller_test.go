package controller_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcile_ConfigDoesNotExist(t *testing.T) {
	r := newTestReconciler(t, unexpectedHTTPClient(t))

	result, err := r.Reconcile(context.Background(), configRequest())
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("expected no requeue, got %+v", result)
	}
}

func TestReconcile_SecretValidationFailures(t *testing.T) {
	t.Run("secret does not exist", func(t *testing.T) {
		cfg := newKeightlyConfig("keightly-secrets", "anthropic-api-key")
		r := newTestReconciler(t, unexpectedHTTPClient(t), cfg)

		result, err := r.Reconcile(context.Background(), configRequest())
		if err != nil {
			t.Fatalf("reconcile returned unexpected error: %v", err)
		}
		if result.RequeueAfter != 30*time.Second {
			t.Fatalf("expected requeueAfter=30s, got %s", result.RequeueAfter)
		}

		got := getConfig(t, r.Client)
		if got.Status.Active {
			t.Fatalf("expected status.active=false when secret is missing")
		}
	})

	t.Run("secret key does not exist", func(t *testing.T) {
		cfg := newKeightlyConfig("keightly-secrets", "anthropic-api-key")
		secret := newSecret("keightly-secrets", map[string][]byte{
			"some-other-key": []byte("present"),
		})
		r := newTestReconciler(t, unexpectedHTTPClient(t), cfg, secret)

		result, err := r.Reconcile(context.Background(), configRequest())
		if err != nil {
			t.Fatalf("reconcile returned unexpected error: %v", err)
		}
		if result.RequeueAfter != 30*time.Second {
			t.Fatalf("expected requeueAfter=30s, got %s", result.RequeueAfter)
		}

		got := getConfig(t, r.Client)
		if got.Status.Active {
			t.Fatalf("expected status.active=false when secret key is missing")
		}
	})

	t.Run("secret key value is empty", func(t *testing.T) {
		cfg := newKeightlyConfig("keightly-secrets", "anthropic-api-key")
		secret := newSecret("keightly-secrets", map[string][]byte{
			"anthropic-api-key": {},
		})
		r := newTestReconciler(t, unexpectedHTTPClient(t), cfg, secret)

		result, err := r.Reconcile(context.Background(), configRequest())
		if err != nil {
			t.Fatalf("reconcile returned unexpected error: %v", err)
		}
		if result.RequeueAfter != 30*time.Second {
			t.Fatalf("expected requeueAfter=30s, got %s", result.RequeueAfter)
		}

		got := getConfig(t, r.Client)
		if got.Status.Active {
			t.Fatalf("expected status.active=false when secret key is empty")
		}
	})
}

func TestReconcile_APIInvalidReturnsRequeueFiveMinutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := newKeightlyConfig("keightly-secrets", "anthropic-api-key")
	secret := newSecret("keightly-secrets", map[string][]byte{
		"anthropic-api-key": []byte("bad-key"),
	})

	r := newTestReconciler(t, anthropicHTTPClientForServer(t, server, 0, nil), cfg, secret)
	result, err := r.Reconcile(context.Background(), configRequest())
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}
	if result.RequeueAfter != 5*time.Minute {
		t.Fatalf("expected requeueAfter=5m, got %s", result.RequeueAfter)
	}

	got := getConfig(t, r.Client)
	if got.Status.Active {
		t.Fatalf("expected status.active=false on 401")
	}
}

func TestReconcile_APITimeoutReturnsRequeueOneMinute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := newKeightlyConfig("keightly-secrets", "anthropic-api-key")
	secret := newSecret("keightly-secrets", map[string][]byte{
		"anthropic-api-key": []byte("slow-key"),
	})

	r := newTestReconciler(t, anthropicHTTPClientForServer(t, server, 50*time.Millisecond, nil), cfg, secret)
	result, err := r.Reconcile(context.Background(), configRequest())
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}
	if result.RequeueAfter != time.Minute {
		t.Fatalf("expected requeueAfter=1m, got %s", result.RequeueAfter)
	}

	got := getConfig(t, r.Client)
	if got.Status.Active {
		t.Fatalf("expected status.active=false on timeout")
	}
}

func TestReconcile_HealthySetsStatusAndNoRequeue(t *testing.T) {
	type observedRequest struct {
		method     string
		path       string
		apiKey     string
		apiVersion string
	}

	deadlineObserved := make(chan time.Duration, 1)
	observed := make(chan observedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		observed <- observedRequest{
			method:     req.Method,
			path:       req.URL.Path,
			apiKey:     req.Header.Get("x-api-key"),
			apiVersion: req.Header.Get("anthropic-version"),
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := newKeightlyConfig("keightly-secrets", "anthropic-api-key")
	secret := newSecret("keightly-secrets", map[string][]byte{
		"anthropic-api-key": []byte("valid-key"),
	})

	start := time.Now().UTC()
	r := newTestReconciler(t, anthropicHTTPClientForServer(t, server, 0, func(req *http.Request) {
		deadline, ok := req.Context().Deadline()
		if !ok {
			deadlineObserved <- -1
			return
		}
		deadlineObserved <- time.Until(deadline)
	}), cfg, secret)
	result, err := r.Reconcile(context.Background(), configRequest())
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("expected no requeue, got %+v", result)
	}

	select {
	case req := <-observed:
		if req.method != http.MethodGet {
			t.Fatalf("expected method GET, got %s", req.method)
		}
		if req.path != "/v1/models" {
			t.Fatalf("expected path /v1/models, got %s", req.path)
		}
		if req.apiKey != "valid-key" {
			t.Fatalf("expected x-api-key header to be set")
		}
		if req.apiVersion != testAnthropicHeader {
			t.Fatalf("expected anthropic-version=%s, got %s", testAnthropicHeader, req.apiVersion)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not observe health-check request")
	}

	select {
	case observedDeadline := <-deadlineObserved:
		if observedDeadline < 0 {
			t.Fatalf("expected request context deadline to be set")
		}
		if observedDeadline < 8*time.Second || observedDeadline > 11*time.Second {
			t.Fatalf("expected request deadline close to 10s, got %s", observedDeadline)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not observe request deadline")
	}

	got := getConfig(t, r.Client)
	if !got.Status.Active {
		t.Fatalf("expected status.active=true when API is healthy")
	}
	if got.Status.ConnectedMonitors != 0 {
		t.Fatalf("expected connectedMonitors=0, got %d", got.Status.ConnectedMonitors)
	}
	if got.Status.LastHealthCheck == "" {
		t.Fatalf("expected lastHealthCheck to be populated")
	}
	lastHealthCheck, err := time.Parse(time.RFC3339, got.Status.LastHealthCheck)
	if err != nil {
		t.Fatalf("expected RFC3339 lastHealthCheck, got %q: %v", got.Status.LastHealthCheck, err)
	}
	if lastHealthCheck.Before(start.Add(-1 * time.Second)) {
		t.Fatalf("expected lastHealthCheck close to reconcile time, got %s", lastHealthCheck)
	}
}

func TestConnectedMonitorsCountsOnlyEnabledViaReconcile(t *testing.T) {
	testCases := []struct {
		name     string
		monitors []client.Object
		want     int
	}{
		{
			name:     "zero monitors returns zero",
			monitors: nil,
			want:     0,
		},
		{
			name: "counts only enabled monitors",
			monitors: []client.Object{
				newMonitor("team-a", "enabled-1", true),
				newMonitor("team-a", "disabled-1", false),
			},
			want: 1,
		},
		{
			name: "mixed enabled and disabled returns correct count",
			monitors: []client.Object{
				newMonitor("team-a", "enabled-1", true),
				newMonitor("team-b", "enabled-2", true),
				newMonitor("team-c", "disabled-1", false),
				newMonitor("team-c", "disabled-2", false),
			},
			want: 2,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cfg := newKeightlyConfig("keightly-secrets", "anthropic-api-key")
			secret := newSecret("keightly-secrets", map[string][]byte{
				"anthropic-api-key": []byte("valid-key"),
			})

			objects := append([]client.Object{cfg, secret}, tc.monitors...)
			r := newTestReconciler(t, anthropicHTTPClientForServer(t, server, 0, nil), objects...)

			result, err := r.Reconcile(context.Background(), configRequest())
			if err != nil {
				t.Fatalf("reconcile returned unexpected error: %v", err)
			}
			if result.Requeue || result.RequeueAfter != 0 {
				t.Fatalf("expected no requeue, got %+v", result)
			}

			got := getConfig(t, r.Client)
			if got.Status.ConnectedMonitors != tc.want {
				t.Fatalf("expected connectedMonitors=%d, got %d", tc.want, got.Status.ConnectedMonitors)
			}
		})
	}
}
