# Keightly — CLAUDE.md

This file gives Claude Code (and human contributors) full context about the Keightly project. Read this before making any changes.

---

## What is Keightly?

Keightly (K8 + ly) is a **Kubernetes-native Operator** that automatically detects, diagnoses, and explains Kubernetes failures using AI. It specialises deeply in two failure modes:

1. **OOMKill** — pods killed by the Linux kernel OOM killer due to exceeding memory limits
2. **CrashLoopBackOff** — pods stuck in a restart loop due to application failures

When a failure is detected, Keightly autonomously gathers context (pod logs, events, node state, resource metrics, topology), sends it to an AI model, and produces a structured, actionable diagnosis stored as a native Kubernetes object queryable via `kubectl`.

The longer term vision is to become the open source AI SRE for Kubernetes — autonomously diagnosing every class of cluster failure (Pending pods, node pressure, ImagePullBackOff, PVC binding failures, RBAC errors, network/DNS failures, and more). OOMKill and CrashLoopBackOff are the wedge.

---

## What Makes Keightly Different

- **Kubernetes-native**: results are stored as CRDs (`KeightlyDiagnosis`), queryable via `kubectl get keightlydiagnoses`
- **Open source and self-hosted**: no data leaves the cluster, unlike SaaS alternatives
- **Single binary**: one Go binary, one Docker image, one Helm chart — `helm install keightly` just works
- **Expert-level diagnosis**: prompts encode real SRE operational knowledge, not generic AI output
- **Raw Anthropic Go SDK**: no Python, no LangChain, no unnecessary abstractions

---

## Architecture

Keightly is a Kubernetes Operator — a controller that encodes human SRE operational knowledge into software.

### Three-CRD model:

```
KeightlyConfig   (cluster-scoped singleton)
  └── sets AI provider, model, API key secret ref, retention, concurrency limits

KeightlyMonitor  (namespace-scoped, one per team/scope)
  └── defines what to watch: target namespaces, failure types, pod selector, severity

KeightlyDiagnosis  (namespace-scoped, operator-created only)
  └── one per detected failure: immutable spec snapshot + mutable status with AI output
```

### Data flow:

```
User applies KeightlyConfig CR (once, cluster-wide)
    ↓
User applies KeightlyMonitor CR (per team/namespace)
    ↓
Monitor controller starts watching pods in configured namespaces
    ↓
Pod OOMKills or enters CrashLoopBackOff
    ↓
Monitor controller detects failure via pod status:
  - OOMKill: lastState.terminated.reason == "OOMKilled" or exitCode 137
  - CrashLoopBackOff: state.waiting.reason == "CrashLoopBackOff"
    ↓
Operator checks existence-based dedup:
  - Is there already a KeightlyDiagnosis for this workload + container + failureType + revisionHash?
  - If yes → skip (same version, same failure already diagnosed)
  - If no → create KeightlyDiagnosis CR in Pending phase
    ↓
KeightlyDiagnosis controller reconciles → phase transitions to Gathering
    ↓
Collectors run, populating spec.context.sources:
  - "logs"     → previous container logs
  - "events"   → namespace events filtered by pod/node
  - "topology" → ownerRef chain, replica counts, related resources
  - "metrics"  → resource usage from metrics-server
    ↓
Phase transitions to Diagnosing → context sent to AI model
    ↓
AI returns structured diagnosis → written to status.diagnosis
    ↓
Phase transitions to Diagnosed
    ↓
Engineer runs: kubectl get keightlydiagnoses
```

No Redis, no Python worker, no separate processes. Controller-runtime provides the internal work queue. CRDs in etcd provide durability.

---

## Repository Structure

Modelled after prometheus-operator and cert-manager. Standard Go project layout.

```
keightly/
├── CLAUDE.md
├── AGENTS.md                              ← instructions for Codex (review & tests)
├── README.md
├── LICENSE                                ← Apache 2.0
├── CHANGELOG.md
├── CONTRIBUTING.md
├── SECURITY.md
├── Dockerfile
├── go.mod
├── go.sum
├── .gitignore
├── .github/
│   └── workflows/                         ← CI pipelines
├── cmd/
│   └── operator/
│       └── main.go                        ← entrypoint, manager setup
├── api/
│   └── v1alpha1/
│       ├── keightlyconfig_types.go        ← KeightlyConfig CRD spec (cluster-scoped singleton)
│       ├── keightlymonitor_types.go       ← KeightlyMonitor CRD spec (namespace-scoped)
│       ├── keightlydiagnosis_types.go     ← KeightlyDiagnosis CRD spec (operator-created)
│       ├── groupversion_info.go           ← registers API group keightly.io/v1alpha1
│       └── zz_generated.deepcopy.go      ← auto-generated, do not edit
├── internal/
│   ├── controller/
│   │   ├── keightlyconfig_controller.go
│   │   ├── keightlymonitor_controller.go
│   │   └── keightlydiagnosis_controller.go
│   ├── collector/
│   │   ├── logs.go
│   │   ├── events.go
│   │   ├── topology.go
│   │   └── metrics.go
│   └── diagnosis/
│       ├── engine.go                      ← Anthropic Go SDK agent loop
│       ├── tools.go                       ← kubectl tool implementations
│       └── prompts.go                     ← expert SRE system prompts
├── config/
│   ├── crd/                               ← generated, do not hand-edit
│   └── rbac/
├── helm/
│   └── keightly/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── hack/
│   └── update-codegen.sh                  ← runs controller-gen
├── test/
│   └── e2e/
└── examples/
    ├── keightlyconfig.yaml
    ├── keightlymonitor.yaml
    └── oomkill-test.yaml
```

Key conventions:
- `internal/` packages cannot be imported externally — all business logic lives here
- `api/` contains only type definitions — no logic
- `config/` is generated, never hand-edited
- `hack/` contains codegen scripts, mirrors kubernetes/kubernetes convention
- `examples/` is what users copy-paste to get started

---

## Technology Decisions

| Decision | Choice | Reason |
|---|---|---|
| Language | Go only | Single binary, K8s ecosystem native |
| Operator framework | controller-runtime | Industry standard (ArgoCD, cert-manager, Prometheus Operator) |
| AI SDK | Raw Anthropic Go SDK | No abstraction, full control, debuggable |
| AI framework | None | 5 tools + ~150 line loop = no framework needed |
| Queue | None | CRDs in etcd + controller-runtime work queue |
| Deduplication | Existence-based (revisionHash) | No time windows to tune; naturally handles rollouts |
| License | Apache 2.0 | K8s ecosystem standard, CNCF compatible |
| Deployment | Helm | Standard for K8s operators |

---

## CRD Specifications

### KeightlyConfig — cluster-wide singleton, must be named `"keightly"`

```yaml
apiVersion: keightly.io/v1alpha1
kind: KeightlyConfig
metadata:
  name: keightly
spec:
  ai:
    provider: anthropic
    model: claude-sonnet-4-20250514
    apiKeySecretRef:
      name: keightly-ai-key
      key: ANTHROPIC_API_KEY
  diagnosisRetention: "72h"
  maxConcurrentDiagnoses: 5
status:
  active: true
  connectedMonitors: 3
  lastHealthCheck: "2026-03-23T10:00:00Z"
```

### KeightlyMonitor — namespace-scoped, one per team/scope

```yaml
apiVersion: keightly.io/v1alpha1
kind: KeightlyMonitor
metadata:
  name: payments-monitor
  namespace: payments
spec:
  targetNamespaces:        # required, at least one namespace
    - payments
    - payments-staging
  failureTypes:            # required, at least one; "OOMKill" and "CrashLoopBackOff" supported
    - OOMKill
    - CrashLoopBackOff
  selector:                # optional; omit to watch all pods in target namespaces
    matchLabels:
      app: payments-api
  severity: critical       # critical | warning | info; default "warning"
  enabled: true            # kill switch; false pauses this monitor
status:
  phase: Active            # Active | Paused | Error
  watchedPods: 12
  diagnosesCreated: 4
  lastFailureDetected: "2026-03-23T09:45:00Z"
```

### KeightlyDiagnosis — auto-created by Operator, never manually

Lives in the target namespace where the failing pod lives, NOT the Monitor's namespace. Labels link back to the Monitor (ownerReferences don't work cross-namespace).

```yaml
apiVersion: keightly.io/v1alpha1
kind: KeightlyDiagnosis
metadata:
  name: oomkill-payments-api-7b4f9-1711188000
  namespace: payments
  labels:
    keightly.io/monitor: payments-monitor
    keightly.io/monitor-namespace: payments
    keightly.io/failure-type: OOMKill
    keightly.io/severity: critical
    keightly.io/owner-kind: Deployment
    keightly.io/owner-name: payments-api
    keightly.io/container: api
    keightly.io/revision-hash: 7b4f9c8d6
  annotations:
    keightly.io/retry: "false"
spec:
  failureType: OOMKill
  podName: payments-api-7b4f9c8d6-xkp2m
  containerName: api
  namespace: payments
  nodeName: k3d-keightly-agent-0
  ownerRef:
    kind: Deployment
    name: payments-api
  revisionHash: 7b4f9c8d6
  monitorRef:
    name: payments-monitor
    namespace: payments
  detectedAt: "2026-03-23T09:45:00Z"
  context:
    restartCount: 3
    exitCode: 137
    resources:
      requests: {"cpu": "250m", "memory": "256Mi"}
      limits: {"cpu": "500m", "memory": "512Mi"}
    sources:
      - name: logs
        data: "..."
      - name: events
        data: "..."
      - name: topology
        data: "..."
      - name: metrics
        data: "..."
status:
  phase: Diagnosed          # Pending | Gathering | Diagnosing | Diagnosed | Error
  diagnosedAt: "2026-03-23T09:45:47Z"
  retryCount: 0
  lastError: ""
  diagnosis:
    summary: "OOMKill due to memory leak in database connection pool"
    rootCause: "Memory grew linearly from 180Mi to 512Mi over 4 hours..."
    category: application   # application | infrastructure | configuration | dependency
    confidence: 0.92
    recommendations:
      - action: "Increase memory limit to 768Mi"
        type: resource      # resource | code | infrastructure | configuration
        priority: immediate # immediate | short-term | long-term
      - action: "Cap connection pool at 25 connections"
        type: code
        priority: short-term
    affectedResources:
      - kind: Deployment
        name: payments-api
        namespace: payments
```

---

## RBAC Requirements

```yaml
- apiGroups: [""]
  resources: ["pods", "events", "nodes", "namespaces"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]
- apiGroups: ["metrics.k8s.io"]
  resources: ["pods", "nodes"]
  verbs: ["get", "list"]
- apiGroups: ["keightly.io"]
  resources: ["keightlyconfigs", "keightlymonitors", "keightlydiagnoses"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["keightly.io"]
  resources: ["keightlyconfigs/status", "keightlymonitors/status", "keightlydiagnoses/status"]
  verbs: ["get", "update", "patch"]
```

No write access to core K8s resources. Keightly is read-only on the cluster. Diagnosis only, no auto-remediation in v1.

---

## Code Style and Conventions

- Standard Go formatting (`gofmt`, `goimports`)
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Structured logging with `slog` (Go 1.21+)
- Context propagation everywhere — every function that does I/O takes `context.Context`
- No global state — everything injected via struct fields
- Interfaces for testability — `DiagnosisEngine`, `Collector`, `KubectlRunner` are interfaces
- Table-driven tests
- Constants for namespace (`keightly-system`) — no magic strings
- Status comparison before write — avoid infinite reconcile loops

---

## What NOT to Do

- Do not add Python to this repo
- Do not add Redis or any external queue
- Do not use LangChain, LangGraph, or Claude Agent SDK
- Do not add auto-remediation in v1 (diagnosis and recommendations only)
- Do not create a web UI in v1 (kubectl is the UI)
- Do not add telemetry or analytics that sends data outside the cluster
- Do not use `panic()` in production code paths
- Do not store the Anthropic API key anywhere except a Kubernetes Secret reference
- Do not hand-edit files in `config/crd/` — generated by controller-gen
- Do not hand-edit `api/v1alpha1/zz_generated.deepcopy.go` — auto-generated