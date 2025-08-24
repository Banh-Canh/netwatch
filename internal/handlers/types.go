package handlers

// LogEntry defines the structure for a persisted log message sent to the frontend (the activity log box !)
type LogEntry struct {
	Timestamp int64  `json:"timestamp"`
	Payload   string `json:"payload"`
	ClassName string `json:"className"`
	LogType   string `json:"logType"`
	Type      string `json:"type"`
}

// AccessRequestPayload defines the structure for a pending request to be sent to the frontend.
// It is derived from the AccessRequest CRD with more spec for diverse evaluation.
type AccessRequestPayload struct {
	RequestID      string `json:"requestID"`
	Requestor      string `json:"requestor"`
	Timestamp      int64  `json:"timestamp"`
	RequestType    string `json:"requestType"`
	SourceService  string `json:"sourceService,omitempty"`
	TargetService  string `json:"targetService,omitempty"`
	Cidr           string `json:"cidr,omitempty"`
	Service        string `json:"service,omitempty"`
	Direction      string `json:"direction"`
	Ports          string `json:"ports"`
	Duration       int64  `json:"duration"`
	Description    string `json:"description,omitempty"`
	CanSelfApprove bool   `json:"canSelfApprove"`
	Status         string `json:"status,omitempty"`
}

type ServiceInfo struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Compound  string            `json:"compound"`
	Labels    map[string]string `json:"labels"`
}

// ActiveAccessInfo defines the structure for an active access policy sent to the frontend.
type ActiveAccessInfo struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	ExpiresAt int64  `json:"expiresAt"`
	Direction string `json:"direction"`
	Ports     string `json:"ports"`
	Status    string `json:"status,omitempty"`
}

// webSocketPayload defines the structure for incoming messages from the WebSocket client.
type webSocketPayload struct {
	Command       string `json:"command"`
	RequestID     string `json:"requestID"`
	SourceService string `json:"sourceService"`
	TargetService string `json:"targetService"`
	Direction     string `json:"direction"`
	Ports         string `json:"ports"`
	Cidr          string `json:"cidr"`
	Service       string `json:"service"`
	Duration      int64  `json:"duration"`
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Description   string `json:"description"`
}

type HTTPError struct {
	Error string `json:"error" example:"Error message"`
}
