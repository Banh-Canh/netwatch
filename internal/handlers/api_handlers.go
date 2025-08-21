// internal/handlers/api_handlers.go
package handlers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	vtkiov1alpha1 "github.com/Banh-Canh/maxtac/api/v1alpha1"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"

	"github.com/Banh-Canh/netwatch/internal/k8s"
	"github.com/Banh-Canh/netwatch/internal/utils/logger"
)

// GetLogs handles fetching persisted logs from Redis.
// GetLogs godoc
// @Summary      Get global activity log
// @Description  Retrieves all persisted log entries from the application.
// @Tags         System
// @Produce      json
// @Success      200  {array}   LogEntry
// @Failure      401  {object}  handlers.HTTPError
// @Failure      500  {object}  handlers.HTTPError
// @Security     ApiKeyAuth
// @Router       /logs [get]
func GetLogs(c *gin.Context) {
	ctx := context.Background()
	logData, err := redisClient.ZRange(ctx, logKey, 0, -1).Result()
	if err != nil {
		if err == redis.Nil {
			c.JSON(http.StatusOK, []LogEntry{})
			return
		}
		logger.Logger.Error("Failed to fetch logs from Redis", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve logs"})
		return
	}

	var logEntries []LogEntry
	for _, entryJSON := range logData {
		var entry LogEntry
		if err := json.Unmarshal([]byte(entryJSON), &entry); err == nil {
			logEntries = append(logEntries, entry)
		} else {
			logger.Logger.Warn("Failed to unmarshal a log entry from Redis", "error", err, "data", entryJSON)
		}
	}

	c.JSON(http.StatusOK, logEntries)
}

// GetPendingRequests lists AccessRequest CRs and enriches them with the current user's permissions.
// GetPendingRequests godoc
// @Summary      List pending access requests
// @Description  Retrieves all pending AccessRequest custom resources and enriches them with the current user's permissions.
// @Tags         Requests
// @Produce      json
// @Success      200  {array}   AccessRequestPayload
// @Failure      401  {object}  handlers.HTTPError
// @Failure      500  {object}  handlers.HTTPError
// @Security     ApiKeyAuth
// @Router       /pending-requests [get]
func GetPendingRequests(c *gin.Context) {
	ctx := c.Request.Context()

	idToken, err := getUserIdToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	userInfo, err := k8s.GetUserInfoFromToken(ctx, idToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token: " + err.Error()})
		return
	}

	requestList, err := k8s.ListAccessRequestsAsApp(ctx)
	if err != nil {
		logger.Logger.Error("Failed to list AccessRequests from cluster", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve pending requests"})
		return
	}

	var pendingRequests []AccessRequestPayload
	g, gCtx := errgroup.WithContext(ctx)
	resultsChan := make(chan AccessRequestPayload, len(requestList.Items))

	for _, item := range requestList.Items {
		request := item
		g.Go(func() error {
			var canSelfApprove bool
			var requiredPerms []k8s.PermissionRequest

			if request.Spec.RequestType == "Service" {
				sourceParts := strings.Split(request.Spec.SourceService, "/")
				targetParts := strings.Split(request.Spec.TargetService, "/")
				if len(sourceParts) == 2 && len(targetParts) == 2 {
					requiredPerms = append(requiredPerms,
						k8s.PermissionRequest{Verb: "create", Resource: "services", Namespace: sourceParts[0]},
						k8s.PermissionRequest{Verb: "create", Resource: "services", Namespace: targetParts[0]},
						k8s.PermissionRequest{Verb: "create", Group: "maxtac.vtk.io", Resource: "accesses", Namespace: sourceParts[0]},
						k8s.PermissionRequest{Verb: "create", Group: "maxtac.vtk.io", Resource: "accesses", Namespace: targetParts[0]},
					)
				}
			} else {
				serviceParts := strings.Split(request.Spec.Service, "/")
				if len(serviceParts) == 2 {
					requiredPerms = append(requiredPerms,
						k8s.PermissionRequest{Verb: "create", Resource: "services", Namespace: serviceParts[0]},
						k8s.PermissionRequest{Verb: "create", Group: "maxtac.vtk.io", Resource: "externalaccesses", Namespace: serviceParts[0]},
					)
				}
			}

			if len(requiredPerms) > 0 {
				allowed, checkErr := k8s.CanPerformAllActions(gCtx, userInfo, requiredPerms)
				if checkErr != nil {
					logger.Logger.Error("Failed to check self-approval permissions", "error", checkErr, "request", request.Name)
					canSelfApprove = false
				} else {
					canSelfApprove = allowed
				}
			}

			resultsChan <- AccessRequestPayload{
				RequestID:      request.Name,
				Requestor:      request.Spec.Requestor,
				Timestamp:      request.CreationTimestamp.Unix(),
				RequestType:    request.Spec.RequestType,
				SourceService:  request.Spec.SourceService,
				TargetService:  request.Spec.TargetService,
				Cidr:           request.Spec.Cidr,
				Service:        request.Spec.Service,
				Direction:      request.Spec.Direction,
				Ports:          request.Spec.Ports,
				Duration:       request.Spec.Duration,
				Description:    request.Spec.Description,
				CanSelfApprove: canSelfApprove,
				Status:         request.Spec.Status,
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		logger.Logger.Error("Error during permission checks for pending requests", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not process pending requests"})
		return
	}
	close(resultsChan)

	for payload := range resultsChan {
		pendingRequests = append(pendingRequests, payload)
	}

	sort.Slice(pendingRequests, func(i, j int) bool {
		return pendingRequests[i].Timestamp < pendingRequests[j].Timestamp
	})

	c.JSON(http.StatusOK, pendingRequests)
}

// GetServices lists all usable services in the cluster.
// GetServices godoc
// @Summary      List all Kubernetes services
// @Description  Retrieves a list of all services in the cluster, filtered to exclude system and clone services.
// @Tags         System
// @Produce      json
// @Success      200  {array}   ServiceInfo
// @Failure      401  {object}  handlers.HTTPError
// @Failure      500  {object}  handlers.HTTPError
// @Security     ApiKeyAuth
// @Router       /services [get]
func GetServices(c *gin.Context) {
	serviceList, err := k8s.ListAllServices(c.Request.Context())
	if err != nil {
		logger.Logger.Error("Failed to list services", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve services from cluster"})
		return
	}

	var serviceInfos []ServiceInfo
	for _, svc := range serviceList.Items {
		if svc.Namespace == "kube-system" || strings.HasPrefix(svc.Name, "netwatch-clone-") {
			continue
		}
		serviceInfos = append(serviceInfos, ServiceInfo{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Compound:  fmt.Sprintf("%s/%s", svc.Namespace, svc.Name),
			Labels:    svc.Labels,
		})
	}

	sort.Slice(serviceInfos, func(i, j int) bool {
		return serviceInfos[i].Compound < serviceInfos[j].Compound
	})

	c.JSON(http.StatusOK, serviceInfos)
}

// GetActiveAccesses lists all active Netwatch-managed access policies.
// GetActiveAccesses godoc
// @Summary      List active access policies
// @Description  Retrieves all active and partially-created (pending) access policies managed by Netwatch.
// @Tags         Access Policies
// @Produce      json
// @Success      200  {array}   ActiveAccessInfo
// @Failure      401  {object}  handlers.HTTPError
// @Failure      500  {object}  handlers.HTTPError
// @Security     ApiKeyAuth
// @Router       /active-accesses [get]
func GetActiveAccesses(c *gin.Context) {
	ctx := c.Request.Context()
	infos := make([]ActiveAccessInfo, 0)

	allServices, err := k8s.ListAllServices(ctx)
	if err != nil {
		logger.Logger.Error("Failed to list services for active access list", "error", err)
		c.JSON(http.StatusOK, []ActiveAccessInfo{})
		return
	}

	clonesByReqID := make(map[string][]corev1.Service)
	for _, svc := range allServices.Items {
		if reqID, ok := svc.Labels["netwatch.vtk.io/request-id"]; ok {
			clonesByReqID[reqID] = append(clonesByReqID[reqID], svc)
		}
	}

	var accessList vtkiov1alpha1.AccessList
	if err := k8s.ListNetwatchAccesses(ctx, &accessList); err != nil {
		logger.Logger.Error("Failed to list active service accesses", "error", err)
	} else {
		processedAccesses := make(map[string]ActiveAccessInfo)

		for _, access := range accessList.Items {
			reqID, ok := access.Labels["netwatch.vtk.io/request-id"]
			if !ok {
				continue
			}

			clones, clonesFound := clonesByReqID[reqID]
			if !clonesFound || len(clones) == 0 {
				logger.Logger.Warn("Found Access object with no corresponding Service clones, skipping display.", "request-id", reqID, "access-name", access.Name)
				continue
			}

			var expiresAt int64 = -1
			if access.Spec.Duration != "" {
				duration, err := time.ParseDuration(access.Spec.Duration)
				if err == nil {
					expiresAt = access.CreationTimestamp.Time.Add(duration).Unix()
				}
			}

			info := ActiveAccessInfo{
				Type:      "Service",
				Name:      access.Name,
				Namespace: access.Namespace,
				ExpiresAt: expiresAt,
				Direction: access.Spec.Direction,
			}

			if len(clones) == 2 {
				// This is a complete pair.
				info.Status = "Active"
				clone1, clone2 := clones[0], clones[1]
				if access.Spec.Targets[0].ServiceName == clone1.Name && access.Spec.Targets[0].Namespace == clone1.Namespace {
					info.Target = clone1.Annotations["netwatch.vtk.io/cloned-from"]
					info.Source = clone2.Annotations["netwatch.vtk.io/cloned-from"]
					info.Ports = clone2.Annotations["netwatch.vtk.io/ports"]
				} else if access.Spec.Targets[0].ServiceName == clone2.Name && access.Spec.Targets[0].Namespace == clone2.Namespace {
					info.Target = clone2.Annotations["netwatch.vtk.io/cloned-from"]
					info.Source = clone1.Annotations["netwatch.vtk.io/cloned-from"]
					info.Ports = clone1.Annotations["netwatch.vtk.io/ports"]
				}
			} else if len(clones) == 1 {
				// This is a partial (pending) access.
				info.Status = "Pending"
				clone := clones[0]
				info.Source = clone.Annotations["netwatch.vtk.io/cloned-from"]
				targetSvcString := fmt.Sprintf("%s/%s", access.Spec.Targets[0].Namespace, strings.Replace(strings.Replace(access.Spec.Targets[0].ServiceName, "netwatch-clone-", "", 1), "-"+hex.EncodeToString([]byte(reqID))[:8], "", 1))
				info.Target = fmt.Sprintf("%s (Pending Approval)", targetSvcString)
				info.Ports = clone.Annotations["netwatch.vtk.io/ports"]
			} else {
				// Any other state ( >2 clones) is inconsistent and should be skipped.
				logger.Logger.Warn("Found Access object with an inconsistent number of clones, skipping display.", "request-id", reqID, "clone-count", len(clones))
				continue
			}

			processedAccesses[reqID] = info
		}

		for _, info := range processedAccesses {
			infos = append(infos, info)
		}
	}

	var extList vtkiov1alpha1.ExternalAccessList
	if err := k8s.ListNetwatchExternalAccesses(ctx, &extList); err != nil {
		logger.Logger.Error("Failed to list active external accesses", "error", err)
	} else {
		for _, access := range extList.Items {
			var expiresAt int64 = -1
			if access.Spec.Duration != "" {
				duration, err := time.ParseDuration(access.Spec.Duration)
				if err == nil {
					expiresAt = access.CreationTimestamp.Time.Add(duration).Unix()
				}
			}

			targetInfo := fmt.Sprintf("%s/* (label)", access.Namespace)
			portsInfo := "All"

			if reqID, ok := access.Labels["netwatch.vtk.io/request-id"]; ok {
				if clones, ok := clonesByReqID[reqID]; ok && len(clones) == 1 {
					clone := clones[0]
					targetInfo = clone.Annotations["netwatch.vtk.io/cloned-from"]
					portsInfo = clone.Annotations["netwatch.vtk.io/ports"]
				}
			}

			infos = append(infos, ActiveAccessInfo{
				Type: "External", Name: access.Name, Namespace: access.Namespace, Source: strings.Join(access.Spec.TargetCIDRs, ", "), Target: targetInfo, ExpiresAt: expiresAt, Direction: access.Spec.Direction, Ports: portsInfo, Status: "Active",
			})
		}
	}

	sort.Slice(infos, func(i, j int) bool {
		if infos[i].ExpiresAt == -1 {
			return false
		}
		if infos[j].ExpiresAt == -1 {
			return true
		}
		return infos[i].ExpiresAt < infos[j].ExpiresAt
	})

	c.JSON(http.StatusOK, infos)
}

// StartLogJanitor runs a background goroutine to clean up old logs from Redis.
func StartLogJanitor(ctx context.Context, client *redis.Client, interval, retention time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger.Logger.Info("Starting Redis log janitor", "interval", interval, "retention", retention)

	for {
		select {
		case <-ticker.C:
			maxScore := time.Now().Add(-retention).UnixMilli()
			if err := client.ZRemRangeByScore(ctx, logKey, "-inf", strconv.FormatInt(maxScore, 10)).Err(); err != nil {
				logger.Logger.Error("Failed to clean up old logs from Redis", "error", err)
			}
		case <-ctx.Done():
			logger.Logger.Info("Stopping Redis log janitor.")
			return
		}
	}
}
