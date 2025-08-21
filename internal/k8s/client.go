// internal/k8s/client.go
package k8s

import (
	"context"
	"fmt"
	"os"
	"strings"

	vtkiov1alpha1 "github.com/Banh-Canh/maxtac/api/v1alpha1"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/sync/errgroup"
	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	k8sScheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	netwatchv1alpha1 "github.com/Banh-Canh/netwatch/api/v1alpha1"
	"github.com/Banh-Canh/netwatch/internal/utils/logger"
)

var (
	appKubeConfig *rest.Config
	appScheme     *runtime.Scheme
	appKubeClient client.Client
)

// InitKubeClient initializes the application's primary Kubernetes client and registers all necessary schemes.
func InitKubeClient() error {
	cfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("could not get kubernetes config: %w", err)
	}
	appKubeConfig = cfg

	s := runtime.NewScheme()
	k8sScheme.AddToScheme(s)        //nolint:all
	vtkiov1alpha1.AddToScheme(s)    //nolint:all
	netwatchv1alpha1.AddToScheme(s) //nolint:all
	appScheme = s

	appKubeClient, err = client.New(appKubeConfig, client.Options{
		Scheme: s,
		Cache: &client.CacheOptions{
			DisableFor: []client.Object{
				&vtkiov1alpha1.Access{},
				&vtkiov1alpha1.ExternalAccess{},
				&netwatchv1alpha1.AccessRequest{},
				&corev1.Service{},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("could not create application client: %w", err)
	}

	logger.Logger.Info("Successfully initialized Kubernetes application client.")
	return nil
}

// UserInfo holds the essential details of a user from their OIDC token.
type UserInfo struct {
	Email  string
	Groups []string
}

// GetUserInfoFromToken verifies an OIDC token and extracts the user's email and groups.
func GetUserInfoFromToken(ctx context.Context, idTokenString string) (*UserInfo, error) {
	provider, err := oidc.NewProvider(ctx, os.Getenv("OIDC_ISSUER_URL"))
	if err != nil {
		return nil, fmt.Errorf("oidc provider failed: %w", err)
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: os.Getenv("OIDC_CLIENT_ID")}).Verify(ctx, idTokenString)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}
	var claims struct {
		Email  string   `json:"email"`
		Groups []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("parsing claims failed: %w", err)
	}
	return &UserInfo{Email: claims.Email, Groups: claims.Groups}, nil
}

// GetImpersonatingKubeClient creates a new Kubernetes client that acts on behalf of the user. So we don't need extra permission for the webapp itself.
func GetImpersonatingKubeClient(idTokenString string) (client.Client, error) {
	userInfo, err := GetUserInfoFromToken(context.Background(), idTokenString)
	if err != nil {
		return nil, err
	}

	impersonatingConfig := *appKubeConfig
	impersonatingConfig.Impersonate = rest.ImpersonationConfig{
		UserName: userInfo.Email,
		Groups:   userInfo.Groups,
	}

	impersonatingClient, err := client.New(&impersonatingConfig, client.Options{Scheme: appScheme})
	if err != nil {
		return nil, fmt.Errorf("could not create impersonating client: %w", err)
	}

	return impersonatingClient, nil
}

// CanPerformAction uses a SubjectAccessReview to check if a user has permission for a single action.
func CanPerformAction(ctx context.Context, userInfo *UserInfo, verb, group, resource, namespace, name string) (bool, error) {
	review := &authv1.SubjectAccessReview{
		Spec: authv1.SubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Verb:      verb,
				Group:     group,
				Resource:  resource,
				Namespace: namespace,
				Name:      name,
			},
			User:   userInfo.Email,
			Groups: userInfo.Groups,
		},
	}

	if err := appKubeClient.Create(ctx, review); err != nil {
		return false, fmt.Errorf("failed to create subjectaccessreview: %w", err)
	}

	return review.Status.Allowed, nil
}

// PermissionRequest encapsulates a single RBAC permission check.
type PermissionRequest struct {
	Verb      string
	Group     string
	Resource  string
	Namespace string
}

// CanPerformAllActions checks if a user can perform a list of actions concurrently. It stops on the first failure.
func CanPerformAllActions(ctx context.Context, userInfo *UserInfo, perms []PermissionRequest) (bool, error) {
	g, gCtx := errgroup.WithContext(ctx)

	for _, p := range perms {
		perm := p // Capture loop variable for the goroutine
		g.Go(func() error {
			allowed, err := CanPerformAction(gCtx, userInfo, perm.Verb, perm.Group, perm.Resource, perm.Namespace, "")
			if err != nil {
				return err
			}
			if !allowed {
				return fmt.Errorf("permission denied: cannot %s %s in namespace %s", perm.Verb, perm.Resource, perm.Namespace)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		if strings.HasPrefix(err.Error(), "permission denied") {
			logger.Logger.Info("User permission check failed", "user", userInfo.Email, "reason", err.Error())
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// IsNotFound is a helper function to check for 'NotFound' errors. It's put like this for easy access in other packages.
func IsNotFound(err error) bool { return errors.IsNotFound(err) }
