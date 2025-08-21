// api/v1alpha1/accessrequest_types.go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessRequestSpec defines the desired state of AccessRequest
type AccessRequestSpec struct {
	Requestor     string `json:"requestor"`
	RequestType   string `json:"requestType"`
	SourceService string `json:"sourceService,omitempty"`
	TargetService string `json:"targetService,omitempty"`
	Cidr          string `json:"cidr,omitempty"`
	Service       string `json:"service,omitempty"`
	Direction     string `json:"direction"`
	Ports         string `json:"ports"`
	Duration      int64  `json:"duration"`
	// +optional
	Description string `json:"description,omitempty"`
	// Status indicates the current state of the request.
	// Can be "PendingFull", "PendingTarget", "PendingSource".
	Status string `json:"status"`
	// RequestID is the unique ID shared by the final Access objects, generated at submission time.
	// +optional
	RequestID string `json:"requestID,omitempty"`
	// SourceCloneName is the name of the service clone created in the source namespace.
	// +optional
	SourceCloneName string `json:"sourceCloneName,omitempty"`
	// TargetCloneName is the name of the service clone created in the target namespace.
	// +optional
	TargetCloneName string `json:"targetCloneName,omitempty"`
}

// AccessRequestStatus defines the observed state of AccessRequest
type AccessRequestStatus struct {
	Status string `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName=ar

type AccessRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccessRequestSpec   `json:"spec,omitempty"`
	Status AccessRequestStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type AccessRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccessRequest{}, &AccessRequestList{})
}
