package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KlarityConfigSpec defines the desired state of KlarityConfig.
type KlarityConfigSpec struct {
	// AI configures the AI provider used for all diagnoses.
	// +kubebuilder:validation:Required
	AI AIConfig `json:"ai"`

	// DiagnosisRetention controls how long completed KlarityDiagnosis CRs are kept
	// before the operator deletes them. Accepts standard Go duration strings (e.g. "72h").
	// Minimum "1h".
	//
	// Retention cleanup rules:
	//   - Only KlarityDiagnosis CRs in Diagnosed or Error phase are eligible for
	//     auto-deletion once they exceed this age.
	//   - CRs in Pending, Gathering, or Diagnosing phase are NEVER auto-deleted
	//     regardless of age, so in-flight diagnoses are never torn out from under
	//     a running agent loop.
	//
	// +kubebuilder:default="72h"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1h')",message="diagnosisRetention must be at least 1h"
	DiagnosisRetention string `json:"diagnosisRetention,omitempty"`

	// MaxConcurrentDiagnoses caps how many diagnosis agent loops may run in parallel
	// across the whole cluster. Tune this to avoid overwhelming the AI API rate limits.
	//
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	MaxConcurrentDiagnoses int `json:"maxConcurrentDiagnoses,omitempty"`
}

// AIConfig holds the AI provider configuration.
type AIConfig struct {
	// Provider is the AI backend to use. Currently only "anthropic" is supported.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=anthropic
	Provider string `json:"provider"`

	// Model is the model identifier to use (e.g. "claude-opus-4-6").
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Model string `json:"model"`

	// APIKeySecretRef references a Secret in the klarity-system namespace that
	// holds the AI provider API key. The Secret must exist before KlarityConfig
	// is reconciled. There is no namespace field — the secret always lives in
	// klarity-system to keep credentials isolated from workload namespaces.
	//
	// +kubebuilder:validation:Required
	APIKeySecretRef SecretKeyRef `json:"apiKeySecretRef"`
}

// SecretKeyRef identifies a key within a Kubernetes Secret.
// The Secret is always resolved in the klarity-system namespace.
type SecretKeyRef struct {
	// Name is the name of the Secret.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key is the key within the Secret whose value is the API key.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// KlarityConfigStatus reflects the observed state of KlarityConfig.
type KlarityConfigStatus struct {
	// Active is true when the operator has successfully initialised and is
	// actively watching for failure events.
	Active bool `json:"active"`

	// ConnectedMonitors is the number of KlarityMonitor CRs currently being
	// reconciled by this operator instance.
	ConnectedMonitors int `json:"connectedMonitors"`

	// LastHealthCheck is the RFC3339 timestamp of the most recent successful
	// AI API health probe.
	LastHealthCheck string `json:"lastHealthCheck,omitempty"`

	// Message contains a human-readable explanation of why active is false.
	// Empty when active is true.
	Message string `json:"message,omitempty"`
}

// KlarityConfig is the cluster-wide singleton configuration for the Klarity operator.
// Exactly one instance must exist and it must be named "klarity".
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'klarity'",message="KlarityConfig must be named 'klarity'"
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active"
// +kubebuilder:printcolumn:name="ConnectedMonitors",type="integer",JSONPath=".status.connectedMonitors"
// +kubebuilder:printcolumn:name="LastHealthCheck",type="string",JSONPath=".status.lastHealthCheck"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type KlarityConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec   KlarityConfigSpec   `json:"spec"`
	Status KlarityConfigStatus `json:"status,omitempty"`
}

// KlarityConfigList contains a list of KlarityConfig objects.
//
// +kubebuilder:object:root=true
type KlarityConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KlarityConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KlarityConfig{}, &KlarityConfigList{})
}
