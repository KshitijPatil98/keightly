package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeightlyMonitorSpec defines the desired state of KeightlyMonitor.
type KeightlyMonitorSpec struct {
	// TargetNamespaces is the list of namespaces this Monitor watches for failures.
	// At least one namespace is required. The user must explicitly specify which
	// namespaces this Monitor watches.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	TargetNamespaces []string `json:"targetNamespaces"`

	// FailureTypes lists the Kubernetes-level failure symptoms this Monitor detects.
	// At least one value is required. Accepted values are "OOMKill" and
	// "CrashLoopBackOff". These are detection triggers (observable symptoms), not
	// root causes. Invalid values are rejected at admission time by CRD schema
	// validation.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Enum=OOMKill;CrashLoopBackOff
	FailureTypes []string `json:"failureTypes"`

	// Selector filters which pods this Monitor watches within the target namespaces.
	// Uses standard Kubernetes label selector semantics. If omitted, all pods in
	// the target namespaces are eligible for failure detection.
	//
	// +kubebuilder:validation:Optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Severity tags all KeightlyDiagnosis CRs created by this Monitor, allowing
	// operators to route or filter diagnoses by urgency. One of "critical",
	// "warning", or "info". Defaults to "warning".
	//
	// +kubebuilder:default="warning"
	// +kubebuilder:validation:Enum=critical;warning;info
	Severity string `json:"severity,omitempty"`

	// Enabled is a kill switch for this Monitor. When set to false, the operator
	// stops watching the target namespaces for this Monitor and its phase transitions
	// to Paused. Existing KeightlyDiagnosis CRs are not deleted. Defaults to true.
	//
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`
}

// KeightlyMonitorStatus reflects the observed state of KeightlyMonitor.
type KeightlyMonitorStatus struct {
	// Phase is the current lifecycle phase of this Monitor.
	//   - Active:  the operator is reconciled and actively watching for failures.
	//   - Paused:  spec.enabled is false; no failure detection is occurring.
	//   - Error:   the operator encountered a configuration or runtime error;
	//              see operator logs for details.
	Phase string `json:"phase,omitempty"`

	// WatchedPods is the count of pods currently matched by this Monitor's selector
	// across all target namespaces. Updated each reconcile cycle.
	WatchedPods int `json:"watchedPods,omitempty"`

	// DiagnosesCreated is the total number of KeightlyDiagnosis CRs created by this
	// Monitor since it was first applied. Monotonically increasing.
	DiagnosesCreated int `json:"diagnosesCreated,omitempty"`

	// LastFailureDetected is the RFC3339 timestamp of the most recent failure event
	// detected by this Monitor. Empty if no failure has been detected yet.
	LastFailureDetected string `json:"lastFailureDetected,omitempty"`
}

// KeightlyMonitor configures which namespaces and pods the Keightly operator watches
// for failures, and how those failures are classified and diagnosed. Each Monitor
// is namespace-scoped, allowing teams to own their own failure detection policy
// without cluster-admin access.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="WatchedPods",type="integer",JSONPath=".status.watchedPods"
// +kubebuilder:printcolumn:name="DiagnosesCreated",type="integer",JSONPath=".status.diagnosesCreated"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type KeightlyMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec   KeightlyMonitorSpec   `json:"spec"`
	Status KeightlyMonitorStatus `json:"status,omitempty"`
}

// KeightlyMonitorList contains a list of KeightlyMonitor objects.
//
// +kubebuilder:object:root=true
type KeightlyMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeightlyMonitor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeightlyMonitor{}, &KeightlyMonitorList{})
}
