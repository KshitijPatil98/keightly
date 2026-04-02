package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeightlyDiagnosisSpec captures the immutable snapshot of a detected pod failure.
// All fields are set by the operator at creation time and never updated thereafter.
type KeightlyDiagnosisSpec struct {
	// FailureType is the Kubernetes-level failure symptom detected, e.g. "OOMKill"
	// or "CrashLoopBackOff". This is the authoritative source of truth; the
	// keightly.io/failure-type label is a queryable projection of this field.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	FailureType string `json:"failureType"`

	// PodName is the full name of the failing pod.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PodName string `json:"podName"`

	// ContainerName is the name of the specific container within the pod that failed.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ContainerName string `json:"containerName"`

	// Namespace is the namespace of the failing pod. This is also the namespace
	// the KeightlyDiagnosis CR lives in.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// NodeName is the node the pod was scheduled on at time of failure.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	NodeName string `json:"nodeName"`

	// OwnerRef is the immediate workload owner of the pod, resolved by the operator
	// at detection time. For standalone pods with no owner, Kind is "Pod" and Name
	// is the pod name.
	//
	// +kubebuilder:validation:Required
	OwnerRef OwnerRef `json:"ownerRef"`

	// RevisionHash is a version identifier for the pod's workload revision.
	// The operator populates this from the pod's labels:
	//   - Deployments: pod-template-hash label
	//   - StatefulSets: controller-revision-hash label
	//   - DaemonSets: controller-revision-hash label
	//   - Jobs/CronJobs: empty (each Job is inherently unique)
	//   - Standalone Pods: empty
	// Used for existence-based deduplication. A new hash means the workload
	// was updated, which triggers a new Diagnosis even if a previous one
	// exists for the same workload + container + failure type.
	// When empty, dedup is based on workload + container + failure type only.
	//
	// +kubebuilder:validation:Optional
	RevisionHash string `json:"revisionHash,omitempty"`

	// MonitorRef is the authoritative reference to the KeightlyMonitor that triggered
	// this Diagnosis. The keightly.io/monitor and keightly.io/monitor-namespace labels
	// are queryable projections of this field.
	//
	// +kubebuilder:validation:Required
	MonitorRef MonitorRef `json:"monitorRef"`

	// DetectedAt is the RFC3339 timestamp of when the operator detected the failure event.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	DetectedAt string `json:"detectedAt"`

	// Context is a snapshot of runtime context gathered at detection time.
	//
	// +kubebuilder:validation:Required
	Context DiagnosisContext `json:"context"`
}

// OwnerRef identifies the immediate workload owner of a pod.
type OwnerRef struct {
	// Kind is the workload kind, e.g. "Deployment", "StatefulSet", "DaemonSet", "Job".
	// For standalone pods, Kind is "Pod".
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name is the workload name, e.g. "payments-api".
	// For standalone pods, Name is the pod name.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// MonitorRef identifies the KeightlyMonitor that triggered a Diagnosis.
type MonitorRef struct {
	// Name is the name of the KeightlyMonitor.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace where the KeightlyMonitor lives.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
}

// DiagnosisContext holds the runtime context snapshot gathered at detection time.
// The sources list is populated during the Gathering phase and may be empty when
// the Diagnosis is first created in Pending phase.
type DiagnosisContext struct {
	// RestartCount is the container restart count at time of failure.
	//
	// +kubebuilder:validation:Required
	RestartCount int `json:"restartCount"`

	// ExitCode is the container exit code at time of failure, e.g. 137 for OOMKill.
	//
	// +kubebuilder:validation:Required
	ExitCode int `json:"exitCode"`

	// Resources holds the pod resource requests and limits at time of failure.
	//
	// +kubebuilder:validation:Optional
	Resources *ResourceValues `json:"resources,omitempty"`

	// Sources is a flexible list of collector outputs gathered during the Gathering
	// phase. Each entry names a collector (e.g. "logs", "events", "topology",
	// "metrics") and holds its output as text. The list is intentionally unstructured
	// — new collectors add new entries without any schema change.
	//
	// +kubebuilder:validation:Optional
	Sources []ContextSource `json:"sources,omitempty"`
}

// ResourceValues holds Kubernetes resource requests and limits for a container.
type ResourceValues struct {
	// Requests maps resource names to their requested quantities,
	// e.g. {"cpu": "250m", "memory": "256Mi"}.
	//
	// +kubebuilder:validation:Optional
	Requests map[string]string `json:"requests,omitempty"`

	// Limits maps resource names to their limit quantities,
	// e.g. {"cpu": "500m", "memory": "512Mi"}.
	//
	// +kubebuilder:validation:Optional
	Limits map[string]string `json:"limits,omitempty"`
}

// ContextSource is a single collector output captured during the Gathering phase.
type ContextSource struct {
	// Name identifies the collector, e.g. "logs", "events", "topology", "metrics".
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Data is the collector output as plain text.
	//
	// +kubebuilder:validation:Required
	Data string `json:"data"`
}

// KeightlyDiagnosisStatus reflects the observed state of a KeightlyDiagnosis.
type KeightlyDiagnosisStatus struct {
	// Phase is the current lifecycle phase of this Diagnosis.
	//   - Pending:    detected, waiting for a processing slot (maxConcurrentDiagnoses).
	//   - Gathering:  collectors are fetching context (logs, events, topology, metrics).
	//   - Diagnosing: context sent to AI, waiting for response.
	//   - Diagnosed:  AI response received and written to status.diagnosis.
	//   - Error:      something failed (AI unavailable, context gathering failed, etc.).
	//
	// +kubebuilder:validation:Enum=Pending;Gathering;Diagnosing;Diagnosed;Error
	Phase string `json:"phase,omitempty"`

	// Diagnosis is the AI output. Empty until phase reaches Diagnosed.
	//
	// +kubebuilder:validation:Optional
	Diagnosis *DiagnosisResult `json:"diagnosis,omitempty"`

	// DiagnosedAt is the RFC3339 timestamp of when the AI diagnosis completed.
	// Empty until the Diagnosed phase is reached.
	DiagnosedAt string `json:"diagnosedAt,omitempty"`

	// RetryCount is the number of times this Diagnosis has been re-attempted via
	// the keightly.io/retry annotation. Starts at 0 and is incremented each time
	// the annotation triggers a re-diagnosis.
	RetryCount int `json:"retryCount,omitempty"`

	// LastError is the error message when phase is Error. Empty otherwise. Recorded
	// to aid debugging without requiring access to operator logs.
	LastError string `json:"lastError,omitempty"`
}

// DiagnosisResult holds the structured AI diagnosis output.
type DiagnosisResult struct {
	// Summary is a one-line description of what happened.
	Summary string `json:"summary,omitempty"`

	// RootCause is a detailed multi-line explanation of why the failure occurred.
	RootCause string `json:"rootCause,omitempty"`

	// Category classifies the root cause of the failure.
	//   - application:    bug in the app code (memory leak, unhandled exception, etc.)
	//   - infrastructure: cluster or node level issue (node pressure, disk full, etc.)
	//   - configuration:  misconfigured resource limits, env vars, ConfigMaps, etc.
	//   - dependency:     failure caused by another service or resource (db down, PVC stuck, etc.)
	//
	// +kubebuilder:validation:Enum=application;infrastructure;configuration;dependency
	Category string `json:"category,omitempty"`

	// Confidence is the AI's confidence in the diagnosis, expressed as a value
	// between 0.0 (no confidence) and 1.0 (certain).
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	Confidence float64 `json:"confidence,omitempty"`

	// Recommendations is an ordered list of recommended actions to resolve the failure.
	//
	// +kubebuilder:validation:Optional
	Recommendations []Recommendation `json:"recommendations,omitempty"`

	// AffectedResources lists other Kubernetes resources involved in or contributing
	// to the failure.
	//
	// +kubebuilder:validation:Optional
	AffectedResources []AffectedResource `json:"affectedResources,omitempty"`
}

// Recommendation is a single recommended action to resolve a diagnosed failure.
type Recommendation struct {
	// Action is a human-readable description of what to do.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Action string `json:"action"`

	// Type classifies what kind of change is recommended.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=resource;code;infrastructure;configuration
	Type string `json:"type"`

	// Priority indicates the urgency of this recommendation.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=immediate;short-term;long-term
	Priority string `json:"priority"`
}

// AffectedResource identifies a Kubernetes resource involved in or contributing to a failure.
type AffectedResource struct {
	// Kind is the resource kind, e.g. "Deployment", "Service", "PVC".
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name is the resource name.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace the resource lives in.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
}

// KeightlyDiagnosis is created automatically by the Keightly operator when it detects a
// pod failure matching a KeightlyMonitor's failureTypes. It is never created manually.
// The spec captures the immutable failure snapshot; the status tracks lifecycle and
// holds the AI diagnosis output.
//
// Re-diagnosis can be triggered by setting the keightly.io/retry annotation to "true".
// The operator resets the phase to Pending, increments retryCount, clears the previous
// diagnosis, re-runs the full pipeline, and resets the annotation to "false" when done.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="FailureType",type="string",JSONPath=".spec.failureType"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Category",type="string",JSONPath=".status.diagnosis.category"
// +kubebuilder:printcolumn:name="Confidence",type="number",JSONPath=".status.diagnosis.confidence"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type KeightlyDiagnosis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec   KeightlyDiagnosisSpec   `json:"spec"`
	Status KeightlyDiagnosisStatus `json:"status,omitempty"`
}

// KeightlyDiagnosisList contains a list of KeightlyDiagnosis objects.
//
// +kubebuilder:object:root=true
type KeightlyDiagnosisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeightlyDiagnosis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeightlyDiagnosis{}, &KeightlyDiagnosisList{})
}
