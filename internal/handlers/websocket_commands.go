// internal/handlers/websocket_commands.go
package handlers

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	vtkiov1alpha1 "github.com/Banh-Canh/maxtac/api/v1alpha1"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	netwatchv1alpha1 "github.com/Banh-Canh/netwatch/api/v1alpha1"
	"github.com/Banh-Canh/netwatch/internal/k8s"
	"github.com/Banh-Canh/netwatch/internal/utils/logger"
)

// webSocketCommandProcessor holds the state and dependencies needed to process commands.
type webSocketCommandProcessor struct {
	ctx               context.Context
	idToken           string
	userInfo          *k8s.UserInfo
	sanitizedUsername string
	logAndBroadcast   func(entry LogEntry)
	sendError         func(msg string, err error, logType string)
}

// Helper function to generate a short, unique hash from a string.
func shortHash(s string) string {
	h := sha1.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func (p *webSocketCommandProcessor) createClusterAccess(userKubeClient client.Client, payload webSocketPayload) {
	var overridePorts []corev1.ServicePort
	if payload.Ports != "" {
		portStrings := strings.SplitSeq(payload.Ports, ",")
		for pStr := range portStrings {
			pStr = strings.TrimSpace(pStr)
			if pStr == "" {
				continue
			}
			port, err := strconv.Atoi(pStr)
			if err != nil {
				p.sendError("Invalid port override", fmt.Errorf("'%s' is not a valid port number", pStr), "Service")
				return
			}
			overridePorts = append(overridePorts, corev1.ServicePort{
				Name: fmt.Sprintf("port-%d", port), Protocol: corev1.ProtocolTCP, Port: int32(port),
			})
		}
	}

	sourceParts := strings.Split(payload.SourceService, "/")
	targetParts := strings.Split(payload.TargetService, "/")
	if len(sourceParts) != 2 || len(targetParts) != 2 {
		p.sendError("Invalid service format", fmt.Errorf("expected 'namespace/name'"), "Service")
		return
	}
	sourceNs, sourceName := sourceParts[0], sourceParts[1]
	targetNs, targetName := targetParts[0], targetParts[1]

	cloneID := uuid.New().String()
	randSuffix := hex.EncodeToString([]byte(cloneID))[:8]
	sourceCloneName := fmt.Sprintf("nc-%s-%s", shortHash(sourceName), randSuffix)
	targetCloneName := fmt.Sprintf("nc-%s-%s", shortHash(targetName), randSuffix)
	commonRequestLabel := map[string]string{"netwatch.vtk.io/request-id": cloneID}

	sourceClone, err := k8s.CloneService(p.ctx, userKubeClient, sourceNs, sourceName, sourceCloneName, commonRequestLabel, overridePorts)
	if err != nil {
		p.sendError("Could not clone source service (check your permissions)", err, "Service")
		return
	}

	targetClone, err := k8s.CloneService(p.ctx, userKubeClient, targetNs, targetName, targetCloneName, commonRequestLabel, overridePorts)
	if err != nil {
		p.sendError("Could not clone target service (check your permissions)", err, "Service")
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, sourceClone.Namespace, sourceClone.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup partial service clone on approval", "error", cleanupErr)
		}
		return
	}

	var durationStr string
	if payload.Duration > 0 {
		durationStr = fmt.Sprintf("%ds", payload.Duration)
	}
	commonAccessLabels := map[string]string{
		"app.kubernetes.io/managed-by": "netwatch",
		"netwatch.vtk.io/user":         p.sanitizedUsername,
		"netwatch.vtk.io/request-id":   cloneID,
	}
	sourceAccess := &vtkiov1alpha1.Access{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("access-%s", sourceCloneName), Namespace: sourceNs, Labels: commonAccessLabels},
		Spec: vtkiov1alpha1.AccessSpec{
			Duration:        durationStr,
			ServiceSelector: &metav1.LabelSelector{MatchLabels: commonRequestLabel},
			Targets:         []vtkiov1alpha1.AccessPoint{{ServiceName: targetClone.Name, Namespace: targetClone.Namespace}},
		},
	}
	targetAccess := &vtkiov1alpha1.Access{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("access-%s", targetCloneName), Namespace: targetNs, Labels: commonAccessLabels},
		Spec: vtkiov1alpha1.AccessSpec{
			Duration:        durationStr,
			ServiceSelector: &metav1.LabelSelector{MatchLabels: commonRequestLabel},
			Targets:         []vtkiov1alpha1.AccessPoint{{ServiceName: sourceClone.Name, Namespace: sourceClone.Namespace}},
		},
	}

	switch payload.Direction {
	case "ingress":
		sourceAccess.Spec.Direction = "ingress"
		targetAccess.Spec.Direction = "egress"
	case "egress":
		sourceAccess.Spec.Direction = "egress"
		targetAccess.Spec.Direction = "ingress"
	default:
		sourceAccess.Spec.Direction = "all"
		targetAccess.Spec.Direction = "all"
	}

	if err := k8s.CreateAccess(p.ctx, userKubeClient, sourceAccess); err != nil {
		p.sendError("Could not create source Access policy (check your permissions)", err, "Service")
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, sourceClone.Namespace, sourceClone.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup partial service clone on approval", "error", cleanupErr)
		}
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, targetClone.Namespace, targetClone.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup partial service clone on approval", "error", cleanupErr)
		}
		return
	}
	if err := k8s.CreateAccess(p.ctx, userKubeClient, targetAccess); err != nil {
		p.sendError("Could not create target Access policy (check your permissions)", err, "Service")
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, sourceClone.Namespace, sourceClone.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup source service clone", "error", cleanupErr)
		}
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, targetClone.Namespace, targetClone.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup target service clone", "error", cleanupErr)
		}
		if cleanupErr := k8s.DeleteAccess(p.ctx, userKubeClient, sourceAccess.Namespace, sourceAccess.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup source access object", "error", cleanupErr)
		}
		return
	}

	var msg string
	if payload.Duration > 0 {
		msg = fmt.Sprintf("SUCCESS: Temporary access policies created for %s.", durationStr)
	} else {
		msg = "SUCCESS: Infinite access policies created."
	}
	logger.Logger.Info("Successfully created temporary access package", "user", p.userInfo.Email, "duration", durationStr)
	p.logAndBroadcast(LogEntry{Payload: msg, ClassName: "log-success", LogType: "Service", Type: "applyResult"})
	p.logAndBroadcast(LogEntry{Payload: "--- Request complete ---", ClassName: "log-success", LogType: "Service", Type: "applyComplete"})
}

func (p *webSocketCommandProcessor) handleRequestClusterAccess(payload webSocketPayload) {
	logger.Logger.Info("WebSocket command received", "command", "requestClusterAccess", "user", p.userInfo.Email)

	sourceParts := strings.Split(payload.SourceService, "/")
	targetParts := strings.Split(payload.TargetService, "/")
	if len(sourceParts) != 2 || len(targetParts) != 2 {
		p.sendError("Invalid service format", fmt.Errorf("expected 'namespace/name'"), "Service")
		return
	}
	sourceNs := sourceParts[0]
	targetNs := targetParts[0]

	requiredPerms := []k8s.PermissionRequest{
		{Verb: "create", Resource: "services", Namespace: sourceNs},
		{Verb: "create", Resource: "services", Namespace: targetNs},
		{Verb: "create", Group: "maxtac.vtk.io", Resource: "accesses", Namespace: sourceNs},
		{Verb: "create", Group: "maxtac.vtk.io", Resource: "accesses", Namespace: targetNs},
	}

	canCreate, err := k8s.CanPerformAllActions(p.ctx, p.userInfo, requiredPerms)
	if err != nil {
		p.sendError("Could not verify permissions for creating access", err, "Service")
		return
	}
	if !canCreate {
		p.sendError(
			"Permission denied. You lack the necessary permissions to create this access directly. Please use 'Submit for Review' instead.",
			nil,
			"Service",
		)
		return
	}

	userKubeClient, err := k8s.GetImpersonatingKubeClient(p.idToken)
	if err != nil {
		p.sendError("Could not create user-impersonating client", err, "Service")
		return
	}
	p.createClusterAccess(userKubeClient, payload)

	var overridePorts []corev1.ServicePort
	if payload.Ports != "" {
		portStrings := strings.SplitSeq(payload.Ports, ",")
		for pStr := range portStrings {
			pStr = strings.TrimSpace(pStr)
			if pStr == "" {
				continue
			}
			port, err := strconv.Atoi(pStr)
			if err != nil {
				p.sendError("Invalid port override", fmt.Errorf("'%s' is not a valid port number", pStr), "Service")
				return
			}
			overridePorts = append(overridePorts, corev1.ServicePort{
				Name: fmt.Sprintf("port-%d", port), Protocol: corev1.ProtocolTCP, Port: int32(port),
			})
		}
	}

	sourceName := sourceParts[1]
	targetName := targetParts[1]

	cloneID := uuid.New().String()
	randSuffix := hex.EncodeToString([]byte(cloneID))[:8]
	sourceCloneName := fmt.Sprintf("netwatch-clone-%s-%s", sourceName, randSuffix)
	targetCloneName := fmt.Sprintf("netwatch-clone-%s-%s", targetName, randSuffix)
	commonRequestLabel := map[string]string{"netwatch.vtk.io/request-id": cloneID}

	sourceClone, err := k8s.CloneService(p.ctx, userKubeClient, sourceNs, sourceName, sourceCloneName, commonRequestLabel, overridePorts)
	if err != nil {
		p.sendError("Could not clone source service (check your permissions)", err, "Service")
		return
	}

	targetClone, err := k8s.CloneService(p.ctx, userKubeClient, targetNs, targetName, targetCloneName, commonRequestLabel, overridePorts)
	if err != nil {
		p.sendError("Could not clone target service (check your permissions)", err, "Service")
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, sourceClone.Namespace, sourceClone.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup source service clone after target clone failed", "error", cleanupErr)
		}
		return
	}

	var durationStr string
	if payload.Duration > 0 {
		durationStr = fmt.Sprintf("%ds", payload.Duration)
	}
	commonAccessLabels := map[string]string{
		"app.kubernetes.io/managed-by": "netwatch",
		"netwatch.vtk.io/user":         p.sanitizedUsername,
		"netwatch.vtk.io/request-id":   cloneID,
	}
	sourceAccess := &vtkiov1alpha1.Access{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("access-%s", sourceCloneName), Namespace: sourceNs, Labels: commonAccessLabels},
		Spec: vtkiov1alpha1.AccessSpec{
			Duration:        durationStr,
			ServiceSelector: &metav1.LabelSelector{MatchLabels: commonRequestLabel},
			Targets:         []vtkiov1alpha1.AccessPoint{{ServiceName: targetClone.Name, Namespace: targetClone.Namespace}},
		},
	}
	targetAccess := &vtkiov1alpha1.Access{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("access-%s", targetCloneName), Namespace: targetNs, Labels: commonAccessLabels},
		Spec: vtkiov1alpha1.AccessSpec{
			Duration:        durationStr,
			ServiceSelector: &metav1.LabelSelector{MatchLabels: commonRequestLabel},
			Targets:         []vtkiov1alpha1.AccessPoint{{ServiceName: sourceClone.Name, Namespace: sourceClone.Namespace}},
		},
	}

	switch payload.Direction {
	case "ingress":
		sourceAccess.Spec.Direction = "ingress"
		targetAccess.Spec.Direction = "egress"
	case "egress":
		sourceAccess.Spec.Direction = "egress"
		targetAccess.Spec.Direction = "ingress"
	default:
		sourceAccess.Spec.Direction = "all"
		targetAccess.Spec.Direction = "all"
	}

	if err := k8s.CreateAccess(p.ctx, userKubeClient, sourceAccess); err != nil {
		p.sendError("Could not create source Access policy (check your permissions)", err, "Service")
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, sourceClone.Namespace, sourceClone.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup source service clone", "error", cleanupErr)
		}
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, targetClone.Namespace, targetClone.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup target service clone", "error", cleanupErr)
		}
		return
	}
	if err := k8s.CreateAccess(p.ctx, userKubeClient, targetAccess); err != nil {
		p.sendError("Could not create target Access policy (check your permissions)", err, "Service")
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, sourceClone.Namespace, sourceClone.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup source service clone", "error", cleanupErr)
		}
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, targetClone.Namespace, targetClone.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup target service clone", "error", cleanupErr)
		}
		if cleanupErr := k8s.DeleteAccess(p.ctx, userKubeClient, sourceAccess.Namespace, sourceAccess.Name); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup source access object", "error", cleanupErr)
		}
		return
	}

	var msg string
	if payload.Duration > 0 {
		msg = fmt.Sprintf("SUCCESS: Temporary access policies created for %s.", durationStr)
	} else {
		msg = "SUCCESS: Infinite access policies created."
	}
	logger.Logger.Info("Successfully created temporary access package", "user", p.userInfo.Email, "duration", durationStr)
	p.logAndBroadcast(LogEntry{Payload: msg, ClassName: "log-success", LogType: "Service", Type: "applyResult"})
	p.logAndBroadcast(LogEntry{Payload: "--- Request complete ---", ClassName: "log-success", LogType: "Service", Type: "applyComplete"})
}

func (p *webSocketCommandProcessor) handleRequestExternalAccess(payload webSocketPayload) {
	logger.Logger.Info("WebSocket command received", "command", "requestExternalAccess", "user", p.userInfo.Email)

	serviceParts := strings.Split(payload.Service, "/")
	if len(serviceParts) != 2 {
		p.sendError("Invalid service format for External Access", nil, "External")
		return
	}
	serviceNs := serviceParts[0]

	requiredPerms := []k8s.PermissionRequest{
		{Verb: "create", Resource: "services", Namespace: serviceNs},
		{Verb: "create", Group: "maxtac.vtk.io", Resource: "externalaccesses", Namespace: serviceNs},
	}

	canCreate, err := k8s.CanPerformAllActions(p.ctx, p.userInfo, requiredPerms)
	if err != nil {
		p.sendError("Could not verify permissions for creating external access", err, "External")
		return
	}
	if !canCreate {
		p.sendError(
			"Permission denied. You lack the necessary permissions to create this external access directly. Please use 'Submit for Review' instead.",
			nil,
			"External",
		)
		return
	}

	userKubeClient, err := k8s.GetImpersonatingKubeClient(p.idToken)
	if err != nil {
		p.sendError("Could not create user-impersonating client", err, "External")
		return
	}

	var overridePorts []corev1.ServicePort
	if payload.Ports != "" {
		portStrings := strings.SplitSeq(payload.Ports, ",")
		for pStr := range portStrings {
			pStr = strings.TrimSpace(pStr)
			if pStr == "" {
				continue
			}
			port, err := strconv.Atoi(pStr)
			if err != nil {
				p.sendError("Invalid port override", fmt.Errorf("'%s' is not a valid port number", pStr), "External")
				return
			}
			overridePorts = append(overridePorts, corev1.ServicePort{
				Name: fmt.Sprintf("port-%d", port), Protocol: corev1.ProtocolTCP, Port: int32(port),
			})
		}
	}

	trimmedCIDR := strings.TrimSpace(payload.Cidr)
	if _, _, err := net.ParseCIDR(trimmedCIDR); err != nil {
		if net.ParseIP(trimmedCIDR) == nil {
			p.sendError("Invalid Source IP / CIDR", fmt.Errorf("'%s' is not a valid IP address or CIDR block", payload.Cidr), "External")
			return
		}
	}

	var durationStr string
	if payload.Duration > 0 {
		durationStr = fmt.Sprintf("%ds", payload.Duration)
	}

	serviceName := serviceParts[1]

	cloneID := uuid.New().String()
	randSuffix := hex.EncodeToString([]byte(cloneID))[:8]
	cloneName := fmt.Sprintf("nc-%s-%s", shortHash(serviceName), randSuffix)
	cloneLabel := map[string]string{"netwatch.vtk.io/request-id": cloneID}

	_, err = k8s.CloneService(p.ctx, userKubeClient, serviceNs, serviceName, cloneName, cloneLabel, overridePorts)
	if err != nil {
		p.sendError("Could not clone service for external access", err, "External")
		return
	}

	ea := &vtkiov1alpha1.ExternalAccess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("ea-%s", cloneName),
			Namespace: serviceNs,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "netwatch",
				"netwatch.vtk.io/user":         p.sanitizedUsername,
				"netwatch.vtk.io/request-id":   cloneID,
			},
		},
		Spec: vtkiov1alpha1.ExternalAccessSpec{
			TargetCIDRs:     []string{payload.Cidr},
			Direction:       payload.Direction,
			Duration:        durationStr,
			ServiceSelector: &metav1.LabelSelector{MatchLabels: cloneLabel},
		},
	}

	if err := k8s.CreateExternalAccess(p.ctx, userKubeClient, ea); err != nil {
		p.sendError("Could not create ExternalAccess policy", err, "External")
		if cleanupErr := k8s.DeleteService(p.ctx, userKubeClient, serviceNs, cloneName); cleanupErr != nil && !k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup service clone", "error", cleanupErr)
		}
		return
	}

	msg := "SUCCESS: ExternalAccess policy request sent."
	p.logAndBroadcast(LogEntry{Payload: msg, ClassName: "log-success", LogType: "External", Type: "applyResult"})
	p.logAndBroadcast(LogEntry{Payload: "--- Request complete ---", ClassName: "log-success", LogType: "External", Type: "applyComplete"})
}

func (p *webSocketCommandProcessor) handleSubmitAccessRequest(payload webSocketPayload) {
	logger.Logger.Info("WebSocket command received", "command", "submitAccessRequest", "user", p.userInfo.Email)

	userKubeClient, err := k8s.GetImpersonatingKubeClient(p.idToken)
	if err != nil {
		p.sendError("Could not create user-impersonating client", err, "Request")
		return
	}

	requestID := uuid.New().String()
	requestCR := &netwatchv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ar-%s-%s", p.sanitizedUsername, requestID[:8])},
		Spec: netwatchv1alpha1.AccessRequestSpec{
			Requestor:     p.userInfo.Email,
			RequestID:     requestID,
			SourceService: payload.SourceService,
			TargetService: payload.TargetService,
			Cidr:          payload.Cidr,
			Service:       payload.Service,
			Direction:     payload.Direction,
			Ports:         payload.Ports,
			Duration:      payload.Duration,
			Description:   payload.Description,
		},
	}

	if payload.TargetService != "" { // Service-to-Service request
		requestCR.Spec.RequestType = "Service"
		sourceParts := strings.Split(payload.SourceService, "/")
		targetParts := strings.Split(payload.TargetService, "/")
		if len(sourceParts) != 2 || len(targetParts) != 2 {
			p.sendError("Invalid service format", nil, "Request")
			return
		}
		sourceNs, sourceName := sourceParts[0], sourceParts[1]
		targetNs, targetName := targetParts[0], targetParts[1]

		permsSource := []k8s.PermissionRequest{
			{Verb: "create", Resource: "services", Namespace: sourceNs},
			{Verb: "create", Group: "maxtac.vtk.io", Resource: "accesses", Namespace: sourceNs},
		}
		permsTarget := []k8s.PermissionRequest{
			{Verb: "create", Resource: "services", Namespace: targetNs},
			{Verb: "create", Group: "maxtac.vtk.io", Resource: "accesses", Namespace: targetNs},
		}

		canSource, _ := k8s.CanPerformAllActions(p.ctx, p.userInfo, permsSource)
		canTarget, _ := k8s.CanPerformAllActions(p.ctx, p.userInfo, permsTarget)

		if canSource && canTarget {
			// If the user can do everything, just create a standard pending request.
			logger.Logger.Info(
				"User has full permissions but chose to submit for review. Creating full pending request.",
				"user",
				p.userInfo.Email,
			)
			requestCR.Spec.Status = "PendingFull"
		} else if canSource {
			logger.Logger.Info("User has source permissions. Creating partial request.", "user", p.userInfo.Email, "sourceNs", sourceNs)
			cloneName, err := p.createPartialAccess(userKubeClient, sourceNs, sourceName, targetNs, targetName, requestID, payload, true)
			if err != nil {
				p.sendError("Failed to create the source-side of the access policy", err, "Request")
				return
			}
			requestCR.Spec.Status = "PendingTarget"
			requestCR.Spec.SourceCloneName = cloneName
		} else if canTarget {
			logger.Logger.Info("User has target permissions. Creating partial request.", "user", p.userInfo.Email, "targetNs", targetNs)
			cloneName, err := p.createPartialAccess(userKubeClient, targetNs, targetName, sourceNs, sourceName, requestID, payload, false)
			if err != nil {
				p.sendError("Failed to create the target-side of the access policy", err, "Request")
				return
			}
			requestCR.Spec.Status = "PendingSource"
			requestCR.Spec.TargetCloneName = cloneName
		} else {
			logger.Logger.Info("User has no permissions. Creating full pending request.", "user", p.userInfo.Email)
			requestCR.Spec.Status = "PendingFull"
		}
	} else { // External Access request
		requestCR.Spec.RequestType = "External"
		requestCR.Spec.Status = "PendingFull"
	}

	if err := k8s.CreateAccessRequestAsApp(p.ctx, requestCR); err != nil {
		p.sendError("Failed to submit AccessRequest", err, "Request")
		return
	}
	p.logAndBroadcast(
		LogEntry{
			Payload:   "SUCCESS: Your access request has been submitted for review.",
			ClassName: "log-success",
			LogType:   "Request",
			Type:      "applyResult",
		},
	)
}

func (p *webSocketCommandProcessor) handleApproveAccessRequest(payload webSocketPayload) {
	logger.Logger.Info(
		"WebSocket command received",
		"command",
		"approveAccessRequest",
		"user",
		p.userInfo.Email,
		"requestID",
		payload.RequestID,
	)
	request, err := k8s.GetAccessRequestAsApp(p.ctx, payload.RequestID)
	if err != nil {
		p.sendError("Could not find pending request to approve", err, "Request")
		return
	}

	approverKubeClient, err := k8s.GetImpersonatingKubeClient(p.idToken)
	if err != nil {
		p.sendError("Could not create approver's impersonating client", err, "Request")
		return
	}

	switch request.Spec.Status {
	case "PendingFull":
		logger.Logger.Info("Approving a full request", "request", request.Name)
		if err := p.approveFullRequest(approverKubeClient, request); err != nil {
			p.sendError("Failed to approve full request", err, "Request")
			return
		}
	case "PendingTarget":
		logger.Logger.Info("Approving target half of a partial request", "request", request.Name)
		if err := p.approvePartialRequest(approverKubeClient, request, false); err != nil {
			p.sendError("Failed to approve target-side of the request", err, "Request")
			return
		}
	case "PendingSource":
		logger.Logger.Info("Approving source half of a partial request", "request", request.Name)
		if err := p.approvePartialRequest(approverKubeClient, request, true); err != nil {
			p.sendError("Failed to approve source-side of the request", err, "Request")
			return
		}
	default:
		p.sendError("Request is in an unknown or invalid state", fmt.Errorf("status: %s", request.Spec.Status), "Request")
		return
	}

	if err := k8s.DeleteAccessRequestAsApp(p.ctx, payload.RequestID); err != nil {
		logger.Logger.Error("Failed to delete approved AccessRequest CR", "error", err, "requestID", payload.RequestID)
	}

	p.logAndBroadcast(LogEntry{
		Payload:   fmt.Sprintf("SUCCESS: Request from %s approved by %s.", request.Spec.Requestor, p.userInfo.Email),
		ClassName: "log-success",
		LogType:   request.Spec.RequestType,
		Type:      "applyResult",
	})
	p.logAndBroadcast(
		LogEntry{Payload: "--- Request complete ---", ClassName: "log-success", LogType: request.Spec.RequestType, Type: "applyComplete"},
	)
}

func (p *webSocketCommandProcessor) handleDenyAccessRequest(payload webSocketPayload) {
	logger.Logger.Info(
		"WebSocket command received",
		"command",
		"denyAccessRequest",
		"user",
		p.userInfo.Email,
		"requestID",
		payload.RequestID,
	)

	request, err := k8s.GetAccessRequestAsApp(p.ctx, payload.RequestID)
	if err != nil {
		p.sendError("Could not find pending request to deny/abort", err, "Request")
		return
	}

	isOwner := p.userInfo.Email == request.Spec.Requestor
	var canProceed bool
	var logMessage string

	if isOwner {
		canProceed = true
		logMessage = fmt.Sprintf("Request from %s was aborted by the owner.", request.Spec.Requestor)
	} else {
		canDeny, err := k8s.CanPerformAction(p.ctx, p.userInfo, "delete", "netwatch.vtk.io", "accessrequests", "", request.Name)
		if err != nil {
			p.sendError("Could not verify permissions for denying the request", err, "Request")
			return
		}
		if !canDeny {
			p.sendError("Permission denied. You are not the request owner and lack permissions to deny this request.", nil, "Request")
			return
		}
		canProceed = true
		logMessage = fmt.Sprintf("Request from %s denied by %s.", request.Spec.Requestor, p.userInfo.Email)
	}

	if !canProceed {
		p.sendError("Authorization check failed.", nil, "Request")
		return
	}

	userKubeClient, err := k8s.GetImpersonatingKubeClient(p.idToken)
	if err != nil {
		p.sendError("Could not create impersonating client for cleanup", err, "Request")
		return
	}

	if request.Spec.Status == "PendingTarget" || request.Spec.Status == "PendingSource" {
		logger.Logger.Info("Denying/aborting a partial request, cleaning up created resources", "request", request.Name)
		var cloneNameToDelete, accessNameToDelete, namespaceToDelete string

		if request.Spec.Status == "PendingTarget" {
			cloneNameToDelete = request.Spec.SourceCloneName
			namespaceToDelete = strings.Split(request.Spec.SourceService, "/")[0]
		} else { // PendingSource
			cloneNameToDelete = request.Spec.TargetCloneName
			namespaceToDelete = strings.Split(request.Spec.TargetService, "/")[0]
		}
		accessNameToDelete = fmt.Sprintf("access-%s", cloneNameToDelete)

		logger.Logger.Info("Deleting orphaned Access object", "name", accessNameToDelete, "namespace", namespaceToDelete)
		if err := k8s.DeleteAccess(p.ctx, userKubeClient, namespaceToDelete, accessNameToDelete); err != nil && !k8s.IsNotFound(err) {
			p.sendError("Failed to clean up orphaned Access object. Manual cleanup may be required.", err, "Request")
		}
	}

	if err := k8s.DeleteAccessRequestAsApp(p.ctx, payload.RequestID); err != nil {
		p.sendError("Failed to delete the AccessRequest resource", err, "Request")
		return
	}

	p.logAndBroadcast(LogEntry{
		Payload:   logMessage,
		ClassName: "log-warning",
		LogType:   "Request",
		Type:      "applyResult",
	})
}

func (p *webSocketCommandProcessor) handleRevokeClusterAccess(payload webSocketPayload) {
	logger.Logger.Info("WebSocket command received", "command", "revokeClusterAccess", "name", payload.Name, "namespace", payload.Namespace)
	userKubeClient, err := k8s.GetImpersonatingKubeClient(p.idToken)
	if err != nil {
		p.sendError("Could not create user-impersonating client for revocation", err, "Service")
		return
	}

	access, err := k8s.GetAccessAsUser(p.ctx, userKubeClient, payload.Namespace, payload.Name)
	if err != nil {
		p.sendError("Could not find the specified access policy. It may have already been revoked.", err, "Service")
		return
	}

	reqID, ok := access.Labels["netwatch.vtk.io/request-id"]
	if !ok {
		p.sendError("Could not revoke access pair: request-id label is missing.", nil, "Service")
		return
	}

	accessesToDelete, err := k8s.ListAllAccessesWithLabelAsApp(p.ctx, reqID)
	if err != nil {
		p.sendError("Failed to find the full access policy pair for deletion.", err, "Service")
		return
	}

	if len(accessesToDelete.Items) == 0 {
		logger.Logger.Warn("Found request-id but no Access objects to delete, maybe already cleaned up?", "reqID", reqID)
	}

	var deletionErrors []string
	for _, accessToDelete := range accessesToDelete.Items {
		err := k8s.DeleteAccess(p.ctx, userKubeClient, accessToDelete.Namespace, accessToDelete.Name)
		if err != nil && !k8s.IsNotFound(err) {
			deletionErrors = append(
				deletionErrors,
				fmt.Sprintf("failed to delete %s/%s: %v", accessToDelete.Namespace, accessToDelete.Name, err),
			)
		}
	}

	if len(deletionErrors) > 0 {
		p.sendError("Encountered errors while deleting the access pair", fmt.Errorf("%s", strings.Join(deletionErrors, "; ")), "Service")
		return
	}

	msg := fmt.Sprintf("SUCCESS: Revocation initiated for access policies with request-id '%s'.", reqID)
	p.logAndBroadcast(LogEntry{Payload: msg, ClassName: "log-success", LogType: "Service", Type: "applyResult"})
}

func (p *webSocketCommandProcessor) handleRevokeExternalAccess(payload webSocketPayload) {
	logger.Logger.Info(
		"WebSocket command received",
		"command",
		"revokeExternalAccess",
		"name",
		payload.Name,
		"namespace",
		payload.Namespace,
	)
	userKubeClient, err := k8s.GetImpersonatingKubeClient(p.idToken)
	if err != nil {
		p.sendError("Could not create user-impersonating client for revocation", err, "External")
		return
	}
	if err := k8s.DeleteExternalAccess(p.ctx, userKubeClient, payload.Namespace, payload.Name); err != nil {
		p.sendError("Could not delete the specified external access policy.", err, "External")
		return
	}
	msg := fmt.Sprintf(
		"SUCCESS: ExternalAccess policy '%s' has been marked for deletion. The controller will clean up its resources.",
		payload.Name,
	)
	p.logAndBroadcast(LogEntry{Payload: msg, ClassName: "log-success", LogType: "External", Type: "applyResult"})
}

func getOverridePorts(ports string) ([]corev1.ServicePort, error) {
	var overridePorts []corev1.ServicePort
	if ports != "" {
		portStrings := strings.SplitSeq(ports, ",")
		for pStr := range portStrings {
			pStr = strings.TrimSpace(pStr)
			if pStr == "" {
				continue
			}
			port, err := strconv.Atoi(pStr)
			if err != nil {
				return nil, fmt.Errorf("'%s' is not a valid port number", pStr)
			}
			overridePorts = append(overridePorts, corev1.ServicePort{
				Name: fmt.Sprintf("port-%d", port), Protocol: corev1.ProtocolTCP, Port: int32(port),
			})
		}
	}
	return overridePorts, nil
}

func (p *webSocketCommandProcessor) createPartialAccess(
	userClient client.Client,
	localNs, localName, remoteNs, remoteName, reqID string,
	payload webSocketPayload,
	isSource bool,
) (string, error) {
	overridePorts, err := getOverridePorts(payload.Ports)
	if err != nil {
		return "", err
	}

	randSuffix := hex.EncodeToString([]byte(reqID))[:8]
	localCloneName := fmt.Sprintf("nc-%s-%s", localName, randSuffix)
	commonRequestLabel := map[string]string{"netwatch.vtk.io/request-id": reqID}

	_, err = k8s.CloneService(p.ctx, userClient, localNs, localName, localCloneName, commonRequestLabel, overridePorts)
	if err != nil {
		return "", fmt.Errorf("could not clone local service: %w", err)
	}

	durationStr := fmt.Sprintf("%ds", payload.Duration)
	commonAccessLabels := map[string]string{
		"app.kubernetes.io/managed-by": "netwatch",
		"netwatch.vtk.io/user":         p.sanitizedUsername,
		"netwatch.vtk.io/request-id":   reqID,
	}

	access := &vtkiov1alpha1.Access{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("access-%s", localCloneName), Namespace: localNs, Labels: commonAccessLabels},
		Spec: vtkiov1alpha1.AccessSpec{
			Duration:        durationStr,
			ServiceSelector: &metav1.LabelSelector{MatchLabels: commonRequestLabel},
			Targets: []vtkiov1alpha1.AccessPoint{
				{ServiceName: fmt.Sprintf("netwatch-clone-%s-%s", remoteName, randSuffix), Namespace: remoteNs},
			},
		},
	}

	if (payload.Direction == "egress" && isSource) || (payload.Direction == "ingress" && !isSource) {
		access.Spec.Direction = "egress"
	} else if (payload.Direction == "ingress" && isSource) || (payload.Direction == "egress" && !isSource) {
		access.Spec.Direction = "ingress"
	} else {
		access.Spec.Direction = "all"
	}

	if err := k8s.CreateAccess(p.ctx, userClient, access); err != nil {
		if cleanupErr := k8s.DeleteService(p.ctx, userClient, localNs, localCloneName); cleanupErr != nil && !k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup partial service clone", "error", cleanupErr)
		}
		return "", fmt.Errorf("could not create partial access policy: %w", err)
	}

	return localCloneName, nil
}

// approveFullRequest contains the logic for approving a full request (which applies to both Service and External).
func (p *webSocketCommandProcessor) approveFullRequest(approverClient client.Client, request *netwatchv1alpha1.AccessRequest) error {
	switch request.Spec.RequestType {
	case "Service":
		// The following variables must be defined inside this function if used here,
		// or pulled from the request object:
		sourceParts := strings.Split(request.Spec.SourceService, "/")
		targetParts := strings.Split(request.Spec.TargetService, "/")
		sourceNs, sourceName := sourceParts[0], sourceParts[1]
		targetNs, targetName := targetParts[0], targetParts[1]

		cloneID := request.Spec.RequestID // Use the ID generated at submission
		randSuffix := hex.EncodeToString([]byte(cloneID))[:8]
		sourceCloneName := fmt.Sprintf("nc-%s-%s", shortHash(sourceName), randSuffix)
		targetCloneName := fmt.Sprintf("nc-%s-%s", shortHash(targetName), randSuffix)
		commonRequestLabel := map[string]string{"netwatch.vtk.io/request-id": cloneID}

		overridePorts, err := getOverridePorts(request.Spec.Ports)
		if err != nil {
			return err
		}

		sourceClone, err := k8s.CloneService(
			p.ctx,
			approverClient,
			sourceNs,
			sourceName,
			sourceCloneName,
			commonRequestLabel,
			overridePorts,
		)
		if err != nil {
			return fmt.Errorf("could not clone source service: %w", err)
		}

		targetClone, err := k8s.CloneService(
			p.ctx,
			approverClient,
			targetNs,
			targetName,
			targetCloneName,
			commonRequestLabel,
			overridePorts,
		)
		if err != nil {
			if cleanupErr := k8s.DeleteService(p.ctx, approverClient, sourceClone.Namespace, sourceClone.Name); cleanupErr != nil &&
				!k8s.IsNotFound(cleanupErr) {
				logger.Logger.Error("Failed to cleanup source service clone", "error", cleanupErr)
			}
			return fmt.Errorf("could not clone target service: %w", err)
		}

		var durationStr string
		if request.Spec.Duration > 0 {
			durationStr = fmt.Sprintf("%ds", request.Spec.Duration)
		}
		commonAccessLabels := map[string]string{
			"app.kubernetes.io/managed-by": "netwatch",
			"netwatch.vtk.io/user":         p.sanitizedUsername,
			"netwatch.vtk.io/request-id":   cloneID,
		}
		sourceAccess := &vtkiov1alpha1.Access{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("access-%s", sourceCloneName), Namespace: sourceNs, Labels: commonAccessLabels},
			Spec: vtkiov1alpha1.AccessSpec{
				Duration:        durationStr,
				ServiceSelector: &metav1.LabelSelector{MatchLabels: commonRequestLabel},
				Targets:         []vtkiov1alpha1.AccessPoint{{ServiceName: targetClone.Name, Namespace: targetClone.Namespace}},
			},
		}
		targetAccess := &vtkiov1alpha1.Access{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("access-%s", targetCloneName), Namespace: targetNs, Labels: commonAccessLabels},
			Spec: vtkiov1alpha1.AccessSpec{
				Duration:        durationStr,
				ServiceSelector: &metav1.LabelSelector{MatchLabels: commonRequestLabel},
				Targets:         []vtkiov1alpha1.AccessPoint{{ServiceName: sourceClone.Name, Namespace: sourceClone.Namespace}},
			},
		}

		switch request.Spec.Direction {
		case "ingress":
			sourceAccess.Spec.Direction = "ingress"
			targetAccess.Spec.Direction = "egress"
		case "egress":
			sourceAccess.Spec.Direction = "egress"
			targetAccess.Spec.Direction = "ingress"
		default:
			sourceAccess.Spec.Direction = "all"
			targetAccess.Spec.Direction = "all"
		}

		if err := k8s.CreateAccess(p.ctx, approverClient, sourceAccess); err != nil {
			if cleanupErr := k8s.DeleteService(p.ctx, approverClient, sourceClone.Namespace, sourceClone.Name); cleanupErr != nil &&
				!k8s.IsNotFound(cleanupErr) {
				logger.Logger.Error("Failed to cleanup source service clone", "error", cleanupErr)
			}
			if cleanupErr := k8s.DeleteService(p.ctx, approverClient, targetClone.Namespace, targetClone.Name); cleanupErr != nil &&
				!k8s.IsNotFound(cleanupErr) {
				logger.Logger.Error("Failed to cleanup target service clone", "error", cleanupErr)
			}
			return fmt.Errorf("could not create source Access policy: %w", err)
		}
		if err := k8s.CreateAccess(p.ctx, approverClient, targetAccess); err != nil {
			if cleanupErr := k8s.DeleteService(p.ctx, approverClient, sourceClone.Namespace, sourceClone.Name); cleanupErr != nil &&
				!k8s.IsNotFound(cleanupErr) {
				logger.Logger.Error("Failed to cleanup source service clone", "error", cleanupErr)
			}
			if cleanupErr := k8s.DeleteService(p.ctx, approverClient, targetClone.Namespace, targetClone.Name); cleanupErr != nil &&
				!k8s.IsNotFound(cleanupErr) {
				logger.Logger.Error("Failed to cleanup target service clone", "error", cleanupErr)
			}
			if cleanupErr := k8s.DeleteAccess(p.ctx, approverClient, sourceAccess.Namespace, sourceAccess.Name); cleanupErr != nil &&
				!k8s.IsNotFound(cleanupErr) {
				logger.Logger.Error("Failed to cleanup source access object", "error", cleanupErr)
			}
			return fmt.Errorf("could not create target Access policy: %w", err)
		}

	case "External":
		serviceParts := strings.Split(request.Spec.Service, "/")
		serviceNs, serviceName := serviceParts[0], serviceParts[1]
		cloneID := request.Spec.RequestID
		randSuffix := hex.EncodeToString([]byte(cloneID))[:8]
		cloneName := fmt.Sprintf("nc-%s-%s", shortHash(serviceName), randSuffix)
		cloneLabel := map[string]string{"netwatch.vtk.io/request-id": cloneID}

		overridePorts, err := getOverridePorts(request.Spec.Ports)
		if err != nil {
			return err
		}

		_, err = k8s.CloneService(p.ctx, approverClient, serviceNs, serviceName, cloneName, cloneLabel, overridePorts)
		if err != nil {
			return fmt.Errorf("could not clone service: %w", err)
		}

		var durationStr string
		if request.Spec.Duration > 0 {
			durationStr = fmt.Sprintf("%ds", request.Spec.Duration)
		}
		commonAccessLabels := map[string]string{
			"app.kubernetes.io/managed-by": "netwatch",
			"netwatch.vtk.io/user":         p.sanitizedUsername,
			"netwatch.vtk.io/request-id":   cloneID,
		}

		ea := &vtkiov1alpha1.ExternalAccess{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("ea-%s", cloneName),
				Namespace: serviceNs,
				Labels:    commonAccessLabels,
			},
			Spec: vtkiov1alpha1.ExternalAccessSpec{
				TargetCIDRs:     []string{request.Spec.Cidr},
				Direction:       request.Spec.Direction,
				Duration:        durationStr,
				ServiceSelector: &metav1.LabelSelector{MatchLabels: cloneLabel},
			},
		}
		if err := k8s.CreateExternalAccess(p.ctx, approverClient, ea); err != nil {
			if cleanupErr := k8s.DeleteService(p.ctx, approverClient, serviceNs, cloneName); cleanupErr != nil &&
				!k8s.IsNotFound(cleanupErr) {
				logger.Logger.Error("Failed to cleanup service clone after ExternalAccess failed", "error", cleanupErr)
			}
			return fmt.Errorf("could not create ExternalAccess policy: %w", err)
		}
	}
	return nil
}

func (p *webSocketCommandProcessor) approvePartialRequest(
	approverClient client.Client,
	request *netwatchv1alpha1.AccessRequest,
	approvingSourceSide bool,
) error {
	var localNs, localName, remoteNs, existingCloneName string
	sourceParts := strings.Split(request.Spec.SourceService, "/")
	targetParts := strings.Split(request.Spec.TargetService, "/")

	if approvingSourceSide {
		localNs, localName = sourceParts[0], sourceParts[1]
		remoteNs = targetParts[0]
		existingCloneName = request.Spec.TargetCloneName
	} else {
		localNs, localName = targetParts[0], targetParts[1]
		remoteNs = sourceParts[0]
		existingCloneName = request.Spec.SourceCloneName
	}

	overridePorts, err := getOverridePorts(request.Spec.Ports)
	if err != nil {
		return err
	}

	randSuffix := hex.EncodeToString([]byte(request.Spec.RequestID))[:8]
	localCloneName := fmt.Sprintf("nc-%s-%s", localName, randSuffix)
	commonRequestLabel := map[string]string{"netwatch.vtk.io/request-id": request.Spec.RequestID}

	_, err = k8s.CloneService(p.ctx, approverClient, localNs, localName, localCloneName, commonRequestLabel, overridePorts)
	if err != nil {
		return fmt.Errorf("could not clone missing service: %w", err)
	}

	durationStr := fmt.Sprintf("%ds", request.Spec.Duration)
	commonAccessLabels := map[string]string{
		"app.kubernetes.io/managed-by": "netwatch",
		"netwatch.vtk.io/user":         p.sanitizedUsername,
		"netwatch.vtk.io/request-id":   request.Spec.RequestID,
	}

	access := &vtkiov1alpha1.Access{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("access-%s", localCloneName), Namespace: localNs, Labels: commonAccessLabels},
		Spec: vtkiov1alpha1.AccessSpec{
			Duration:        durationStr,
			ServiceSelector: &metav1.LabelSelector{MatchLabels: commonRequestLabel},
			Targets:         []vtkiov1alpha1.AccessPoint{{ServiceName: existingCloneName, Namespace: remoteNs}},
		},
	}

	isSource := request.Spec.Status == "PendingSource"
	if (request.Spec.Direction == "egress" && isSource) || (request.Spec.Direction == "ingress" && !isSource) {
		access.Spec.Direction = "egress"
	} else if (request.Spec.Direction == "ingress" && isSource) || (request.Spec.Direction == "egress" && !isSource) {
		access.Spec.Direction = "ingress"
	} else {
		access.Spec.Direction = "all"
	}

	if err := k8s.CreateAccess(p.ctx, approverClient, access); err != nil {
		if cleanupErr := k8s.DeleteService(p.ctx, approverClient, localNs, localCloneName); cleanupErr != nil &&
			!k8s.IsNotFound(cleanupErr) {
			logger.Logger.Error("Failed to cleanup partial service clone on approval", "error", cleanupErr)
		}
		return fmt.Errorf("could not create missing access policy: %w", err)
	}

	return nil
}
