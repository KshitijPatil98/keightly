# Klarity — CLAUDE.md

This file gives Claude Code (and human contributors) full context about the Klarity project. Read this before making any changes.

---

## What is Klarity?

Klarity is a **Kubernetes-native Operator** that automatically detects, diagnoses, and explains Kubernetes failures using AI. It specialises deeply in two failure modes:

1. **OOMKill** — pods killed by the Linux kernel OOM killer due to exceeding memory limits
2. **CrashLoopBackOff** — pods stuck in a restart loop due to application failures

When a failure is detected, Klarity autonomously runs kubectl commands, correlates signals across pod logs, events, node state, and resource metrics, and produces a structured, actionable diagnosis — posted to Slack and stored as a native Kubernetes object queryable via kubectl.

The longer term vision is to become the open source AI SRE for Kubernetes — autonomously diagnosing every class of cluster failure (Pending pods, node pressure, ImagePullBackOff, PVC binding failures, RBAC errors, network/DNS failures, and more). OOMKill and CrashLoopBackOff are the wedge.

---

## What Makes Klarity Different

- **Kubernetes-native**: results are stored as CRDs (KlarityIncident), queryable via kubectl get klarityincidents
- **Open source and self-hosted**: no data leaves the cluster, unlike SaaS alternatives
- **Single binary**: one Go binary, one Docker image, one Helm chart — helm install klarity just works
- **Expert-level diagnosis**: prompts encode real SRE operational knowledge, not generic AI output
- **Raw Anthropic Go SDK**: no Python, no LangChain, no unnecessary abstractions

---

## Architecture

Klarity is a Kubernetes Operator — a controller that encodes human SRE operational knowledge into software.

### Data flow:

```
User applies KlarityPolicy CR
    ↓
Event watcher starts watching configured namespaces
    ↓
Pod OOMKills or enters CrashLoopBackOff
    ↓
Event watcher detects K8s event (reason: OOMKilling / BackOff)
    ↓
Controller creates KlarityIncident CR
    ↓
KlarityIncident controller reconciles → triggers diagnosis engine
    ↓
Diagnosis engine runs Claude agent loop:
  - Calls kubectl_describe_pod
  - Calls kubectl_logs_previous
  - Calls kubectl_get_events
  - Calls kubectl_describe_node
  - Calls kubectl_top_pod
    ↓
Diagnosis written to KlarityIncident.Status
    ↓
Notifier sends structured Slack message
    ↓
Engineer runs: kubectl get klarityincidents
```

No Redis, no Python worker, no separate processes. Controller-runtime provides the internal work queue. CRDs in etcd provide durability.

---

## Repository Structure

Modelled after prometheus-operator and cert-manager. Standard Go project layout.

```
klarity/
├── CLAUDE.md
├── README.md
├── LICENSE                                ← Apache 2.0
├── CHANGELOG.md
├── CONTRIBUTING.md
├── SECURITY.md
├── Dockerfile
├── Taskfile.yml
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
│       ├── klaritypolicy_types.go         ← KlarityPolicy CRD spec
│       ├── klarityincident_types.go       ← KlarityIncident CRD spec
│       ├── groupversion_info.go           ← registers API group klarity.dev/v1alpha1
│       └── zz_generated.deepcopy.go      ← auto-generated, do not edit
├── internal/
│   ├── controller/
│   │   ├── klaritypolicy_controller.go
│   │   └── klarityincident_controller.go
│   ├── watcher/
│   │   └── event_watcher.go              ← watches native K8s events
│   ├── diagnosis/
│   │   ├── engine.go                     ← Anthropic Go SDK agent loop
│   │   ├── tools.go                      ← kubectl tool implementations
│   │   └── prompts.go                    ← expert SRE system prompts
│   └── notifier/
│       └── slack.go
├── config/
│   ├── crd/                              ← generated, do not hand-edit
│   └── rbac/
├── helm/
│   └── klarity/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── hack/
│   └── update-codegen.sh                 ← runs controller-gen
├── test/
│   └── e2e/
└── examples/
    ├── klaritypolicy.yaml
    └── oomkill-test.yaml
```

Key conventions:
- internal/ packages cannot be imported externally — all business logic lives here
- api/ contains only type definitions — no logic
- config/ is generated, never hand-edited
- hack/ contains codegen scripts, mirrors kubernetes/kubernetes convention
- examples/ is what users copy-paste to get started

---

## Technology Decisions

| Decision | Choice | Reason |
|---|---|---|
| Language | Go only | Single binary, K8s ecosystem native |
| Operator framework | controller-runtime | Industry standard (ArgoCD, cert-manager, Prometheus Operator) |
| AI SDK | Raw Anthropic Go SDK | No abstraction, full control, debuggable |
| AI framework | None | 5 tools + ~150 line loop = no framework needed |
| Queue | None | CRDs in etcd + controller-runtime work queue |
| Build tool | Taskfile | Simpler than Makefile |
| License | Apache 2.0 | K8s ecosystem standard, CNCF compatible |
| Deployment | Helm | Standard for K8s operators |

---

## CRD Specifications

### KlarityPolicy — user-facing configuration

```yaml
apiVersion: klarity.dev/v1alpha1
kind: KlarityPolicy
metadata:
  name: production-policy
spec:
  watchNamespaces:
    - production
    - staging
  triggers:
    - OOMKill
    - CrashLoopBackOff
  anthropicApiKeySecret:
    name: klarity-secrets
    key: anthropic-api-key
  slack:
    webhookUrlSecret:
      name: klarity-secrets
      key: slack-webhook-url
    channel: "#incidents"
  diagnosis:
    maxConcurrent: 3
    timeoutSeconds: 120
```

### KlarityIncident — auto-created by Operator, never manually

```yaml
apiVersion: klarity.dev/v1alpha1
kind: KlarityIncident
metadata:
  name: oomkill-payment-service-abc123
  namespace: production
spec:
  type: OOMKill
  podName: payment-service-7d9f8b-xkp2m
  namespace: production
  nodeName: k3d-klarity-agent-0
  detectedAt: "2026-03-06T14:23:00Z"
  policyRef: production-policy
status:
  phase: Diagnosed          # Pending | Diagnosing | Diagnosed | Failed
  rootCause: "Memory leak in database connection pool"
  evidence:
    - "Memory grew linearly from 180Mi to 512Mi over 4 hours"
    - "Connection pool size 100, avg 8MB per connection"
    - "Traffic spike at 14:23 increased connections from 12 to 67"
  recommendation: "Increase memory limit to 768Mi and cap connection pool at 25"
  confidence: High
  diagnosedAt: "2026-03-06T14:23:47Z"
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
- apiGroups: ["metrics.k8s.io"]
  resources: ["pods", "nodes"]
  verbs: ["get", "list"]
- apiGroups: ["klarity.dev"]
  resources: ["klaritypolicies", "klarityincidents"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["klarity.dev"]
  resources: ["klarityincidents/status"]
  verbs: ["get", "update", "patch"]
```

No write access to core K8s resources. Klarity is read-only on the cluster. Diagnosis only, no auto-remediation in v1.

---

## Code Style and Conventions

- Standard Go formatting (gofmt, goimports)
- Error wrapping with fmt.Errorf("context: %w", err)
- Structured logging with slog (Go 1.21+)
- Context propagation everywhere — every function that does I/O takes context.Context
- No global state — everything injected via struct fields
- Interfaces for testability — DiagnosisEngine, Notifier, KubectlRunner are interfaces
- Table-driven tests

---

## What to Build Next (in order)

1. api/v1alpha1/groupversion_info.go
2. api/v1alpha1/klaritypolicy_types.go
3. api/v1alpha1/klarityincident_types.go
4. cmd/operator/main.go
5. internal/controller/klaritypolicy_controller.go
6. internal/watcher/event_watcher.go
7. internal/controller/klarityincident_controller.go
8. internal/diagnosis/prompts.go
9. internal/diagnosis/tools.go
10. internal/diagnosis/engine.go
11. internal/notifier/slack.go
12. hack/update-codegen.sh
13. config/rbac/
14. helm/klarity/
15. examples/
16. End to end test with real OOMKill on k3d

---

## What NOT to Do

- Do not add Python to this repo
- Do not add Redis or any external queue
- Do not use LangChain, LangGraph, or Claude Agent SDK
- Do not add auto-remediation in v1 (diagnosis and recommendations only)
- Do not create a web UI in v1 (kubectl is the UI)
- Do not add telemetry or analytics that sends data outside the cluster
- Do not use panic() in production code paths
- Do not store the Anthropic API key anywhere except a Kubernetes Secret reference
- Do not hand-edit files in config/crd/ — generated by controller-gen
- Do not hand-edit api/v1alpha1/zz_generated.deepcopy.go — auto-generated
