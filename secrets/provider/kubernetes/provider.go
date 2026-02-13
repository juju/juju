// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	authenticationv1 "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudspec"
	k8sprovider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
)

var logger = loggo.GetLogger("juju.secrets.provider.kubernetes")

const (
	// BackendType is the type of the Kubernetes secrets backend.
	BackendType = "kubernetes"
)

var (
	controllerIdKey = utils.AnnotationControllerUUIDKey(constants.LabelVersion1)
	modelIdKey      = utils.AnnotationModelUUIDKey(constants.LabelVersion1)
)

// These are patched for testing.
var (
	NewK8sClient = func(config *rest.Config) (kubernetes.Interface, error) {
		return kubernetes.NewForConfig(config)
	}
	InClusterConfig = func() (*rest.Config, error) {
		return rest.InClusterConfig()
	}
)

// NewProvider returns a Kubernetes secrets provider.
func NewProvider() provider.SecretBackendProvider {
	return k8sProvider{}
}

type k8sProvider struct {
}

// Type is the type of the backend.
func (p k8sProvider) Type() string {
	return BackendType
}

// Initialise is not used.
func (p k8sProvider) Initialise(*provider.ModelBackendConfig) error {
	return nil
}

// CleanupModel removes any secrets / ACLs / resources
// associated with the model config.
func (p k8sProvider) CleanupModel(cfg *provider.ModelBackendConfig) error {
	ctx := context.TODO()

	client, err := p.getBroker(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	// Only need to clean up namespace scoped resources if using an external namespace.
	isExternal, err := client.isExternalNamespace()
	if err != nil {
		return errors.Trace(err)
	}
	if isExternal {
		if err := client.deleteSecrets(ctx); err != nil {
			return errors.Trace(err)
		}
		if err := client.deleteRoleBindings(ctx); err != nil {
			return errors.Trace(err)
		}
		if err := client.deleteRoles(ctx); err != nil {
			return errors.Trace(err)
		}
		if err := client.deleteServiceAccounts(ctx); err != nil {
			return errors.Trace(err)
		}
	}
	if err := client.deleteClusterRoleBindings(ctx); err != nil {
		return errors.Trace(err)
	}
	if err := client.deleteClusterRoles(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p k8sProvider) getBroker(cfg *provider.ModelBackendConfig) (*kubernetesClient, error) {
	validCfg, err := newConfig(cfg.Config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	restCfg, err := configToK8sRestConfig(validCfg)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid k8s config")
	}
	k8sClient, err := NewK8sClient(restCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker := &kubernetesClient{
		controllerUUID:    cfg.ControllerUUID,
		modelUUID:         cfg.ModelUUID,
		modelName:         cfg.ModelName,
		namespace:         validCfg.namespace(),
		serviceAccount:    validCfg.serviceAccount(),
		isControllerModel: cfg.ModelName == bootstrap.ControllerModelName,
		client:            k8sClient,
	}
	return broker, nil
}

func configToK8sRestConfig(cfg *backendConfig) (*rest.Config, error) {
	if cfg.preferInClusterAddress() {
		rc, err := InClusterConfig()
		if err != nil && !errors.Is(err, rest.ErrNotInCluster) {
			return nil, errors.Trace(err)
		}
		if rc != nil {
			logger.Tracef("using in-cluster config")
			return rc, nil
		}
	}

	cacerts := cfg.caCerts()
	var CAData []byte
	for _, cacert := range cacerts {
		CAData = append(CAData, cacert...)
	}

	rcfg := &rest.Config{
		Host:        cfg.endpoint(),
		Username:    cfg.username(),
		Password:    cfg.password(),
		BearerToken: cfg.token(),
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte(cfg.clientCert()),
			KeyData:  []byte(cfg.clientKey()),
			CAData:   CAData,
			Insecure: cfg.skipTLSVerify(),
		},
	}
	return rcfg, nil
}

// CleanupSecrets removes rules of the role associated with the removed secrets.
func (p k8sProvider) CleanupSecrets(cfg *provider.ModelBackendConfig, tag names.Tag, removed provider.SecretRevisions) error {
	if tag == nil {
		// This should never happen.
		// Because this method is used for uniter facade only.
		return errors.New("empty tag")
	}
	if len(removed) == 0 {
		return nil
	}

	broker, err := p.getBroker(cfg)
	if err != nil {
		return errors.Trace(err)
	}

	ctx := context.TODO()
	err = broker.dropSecretAccess(ctx, removed.RevisionIDs())
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func cloudSpecToBackendConfig(spec cloudspec.CloudSpec) (*provider.BackendConfig, error) {
	cfg := map[string]interface{}{
		endpointKey: spec.Endpoint,
		caCertsKey:  spec.CACertificates,
	}
	if spec.IsControllerCloud {
		cfg[preferInClusterAddressKey] = true
	}
	if v, ok := spec.Credential.Attributes()[k8scloud.CredAttrUsername]; ok {
		cfg[usernameKey] = v
	}
	if v, ok := spec.Credential.Attributes()[k8scloud.CredAttrPassword]; ok {
		cfg[passwordKey] = v
	}
	if v, ok := spec.Credential.Attributes()[k8scloud.CredAttrClientCertificateData]; ok {
		cfg[clientCertKey] = v
	}
	if v, ok := spec.Credential.Attributes()[k8scloud.CredAttrClientKeyData]; ok {
		cfg[clientKeyKey] = v
	}
	if v, ok := spec.Credential.Attributes()[k8scloud.CredAttrToken]; ok {
		cfg[tokenKey] = v
	}
	return &provider.BackendConfig{
		BackendType: BackendType,
		Config:      cfg,
	}, nil
}

// BuiltInConfig returns the config needed to create a k8s secrets backend
// using the same namespace as the specified model.
func BuiltInConfig(modelName string, controllerUUID string, cloudSpec cloudspec.CloudSpec) (*provider.BackendConfig, error) {
	cfg, err := cloudSpecToBackendConfig(cloudSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	k8sRestConfig, err := k8sprovider.CloudSpecToK8sRestConfig(cloudSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}

	namespace, err := k8sprovider.NamespaceForModel(modelName, controllerUUID, k8sRestConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg.Config[namespaceKey] = namespace
	return cfg, nil
}

// BuiltInName returns the backend name for the k8s in-model backend.
func BuiltInName(modelName string) string {
	return modelName + "-local"
}

// IsBuiltInName returns true of the backend name is the built-in one.
func IsBuiltInName(backendName string) bool {
	return strings.HasSuffix(backendName, "-local")
}

// IssuesTokens returns true if this secret backend provider needs to issue
// a token to provide a restricted (delegated) config.
func (p k8sProvider) IssuesTokens() bool {
	return true
}

// CleanupIssuedTokens removes all ACLs/tokens related to the given issued
// token UUIDs. It returns, even during error, the list of tokens it revoked
// so far.
func (p k8sProvider) CleanupIssuedTokens(
	adminCfg *provider.ModelBackendConfig, issuedTokenUUIDs []string,
) ([]string, error) {
	broker, err := p.getBroker(adminCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctx := context.TODO()

	for i, uuid := range issuedTokenUUIDs {
		err = broker.revokeSecretAccessToken(ctx, uuid)
		if err != nil {
			// Return the tokens deleted so far.
			return issuedTokenUUIDs[:i], errors.New(
				"removing k8s secret backend issued tokens",
			)
		}
	}

	return issuedTokenUUIDs, nil
}

// RestrictedConfig returns the config needed to create a
// secrets backend client restricted to manage the specified
// owned secrets and read shared secrets for the given entity tag.
func (p k8sProvider) RestrictedConfig(
	adminCfg *provider.ModelBackendConfig,
	sameController, forDrain bool,
	issuedTokenUUID string,
	consumer names.Tag,
	owned []string,
	ownedRevs provider.SecretRevisions,
	readRevs provider.SecretRevisions,
) (*provider.BackendConfig, error) {
	logger.Tracef("getting k8s backend config for %q, owned %v, readRevs %v",
		consumer, owned, readRevs)

	if consumer == nil {
		return &adminCfg.BackendConfig, nil
	}

	cfg, err := newConfig(adminCfg.Config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker, err := p.getBroker(adminCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctx := context.TODO()

	// Kubernetes secrets cannot restrict create operations by name. To ensure
	// a restricted config cannot create secrets with other names, we must add
	// an extra pre-created secret object for the next revision. For secrets
	// that have not yet been created, we must make the first revision secret
	// object.
	maxOwnedRev := make(map[string]int)
	for _, rev := range ownedRevs.RevisionIDs() {
		id, rev, err := coresecrets.ParseRevisionName(rev)
		if err != nil {
			return nil, errors.Trace(err)
		}
		maxOwnedRev[id] = max(maxOwnedRev[id], rev)
	}
	preCreateRevisions := make([]string, 0, len(owned))
	for _, id := range owned {
		nextRev := maxOwnedRev[id] + 1
		preCreateRevisions = append(preCreateRevisions,
			coresecrets.RevisionName(id, nextRev))
	}
	err = broker.precreateSecretRevs(ctx, preCreateRevisions)
	if err != nil {
		return nil, errors.Trace(err)
	}

	writeRevs := slices.Concat(ownedRevs.RevisionIDs(), preCreateRevisions)
	token, err := broker.createSecretAccessToken(
		ctx, issuedTokenUUID, consumer, writeRevs, readRevs.RevisionIDs(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	endpoint := cfg.endpoint()
	if sameController && cfg.preferInClusterAddress() {
		// The cloudspec used for controller has a fake endpoint (address and port)
		// because we ignore the endpoint and load the in-cluster credential instead.
		// So we have to clean up the endpoint here for uniter to use.

		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if len(host) != 0 && len(port) != 0 {
			endpoint = "https://" + net.JoinHostPort(host, port)
			logger.Tracef("patching endpoint to %q", endpoint)
		}
	}

	attrs := map[string]interface{}{
		endpointKey:  endpoint,
		namespaceKey: cfg.namespace(),
		caCertsKey:   cfg.caCerts(),
		tokenKey:     token,
	}
	if v := cfg.clientCert(); v != "" {
		attrs[clientCertKey] = v
	}
	if v := cfg.clientKey(); v != "" {
		attrs[clientKeyKey] = v
	}
	if v := cfg.username(); v != "" {
		attrs[usernameKey] = v
	}
	if v := cfg.password(); v != "" {
		attrs[passwordKey] = v
	}
	return &provider.BackendConfig{
		BackendType: BackendType,
		Config:      attrs,
	}, nil
}

type kubernetesClient struct {
	client kubernetes.Interface

	controllerUUID string
	modelUUID      string
	modelName      string

	serviceAccount    string
	namespace         string
	isControllerModel bool
}

// isExternalNamespace returns true if the specified namespace is not used to
// hold the model's resources.
func (k *kubernetesClient) isExternalNamespace() (bool, error) {
	// We will need to revisit this optimisation if we ever support models being
	// able to use a namespace which is different to their name.
	if !k.isControllerModel && k.modelName != k.namespace {
		return true, nil
	}
	ns, err := k.client.CoreV1().Namespaces().Get(context.TODO(), k.namespace, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}
	isExternal := ns.Annotations[modelIdKey] != k.modelUUID ||
		ns.Labels[constants.LabelKubernetesAppManaged] != resources.JujuFieldManager
	return isExternal, nil
}

// TODO: make this configurable.
const (
	minExpireSeconds = 600
)

func (k *kubernetesClient) createServiceAccount(ctx context.Context, sa *core.ServiceAccount) (*core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}

	api := k.client.CoreV1().ServiceAccounts(k.namespace)
	out, err := api.Create(ctx, sa, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	if k8serrors.IsAlreadyExists(err) {
		// Ensure that an existing service account hasn't been created
		// by something other than Juju.
		existing, err := api.Get(ctx, sa.Name, v1.GetOptions{})
		if err != nil {
			return nil, errors.Trace(err)
		}
		if existing.Labels[constants.LabelKubernetesAppManaged] != resources.JujuFieldManager {
			// sa.Name is already used for an existing service account.
			return nil, errors.Errorf("service account %q exists and is not managed by Juju", sa.GetName())
		}
		if existingModelIdValue, ok := existing.Annotations[modelIdKey]; ok && existingModelIdValue != k.modelUUID {
			// sa.Name is already used for an existing service account from a different model.
			return nil, errors.Errorf("service account %q exists and is not managed by this model", sa.GetName())
		}
		return nil, errors.AlreadyExistsf("service account %q", sa.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteServiceAccount(ctx context.Context, name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client.CoreV1().ServiceAccounts(k.namespace).Delete(ctx, name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteServiceAccounts(ctx context.Context) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	listOps := v1.ListOptions{
		LabelSelector: modelLabelSelector(k.modelName).String(),
	}
	api := k.client.CoreV1().ServiceAccounts(k.namespace)
	results, err := api.List(ctx, listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, sa := range results.Items {
		if sa.Annotations[modelIdKey] != k.modelUUID {
			continue
		}
		err := api.Delete(ctx, sa.Name, v1.DeleteOptions{
			PropagationPolicy: constants.DefaultPropagationPolicy(),
		})
		if err == nil || k8serrors.IsNotFound(err) {
			continue
		}
		return errors.Trace(err)
	}
	return nil
}

func (k *kubernetesClient) deleteSecrets(ctx context.Context) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	listOps := v1.ListOptions{
		LabelSelector: modelLabelSelector(k.modelName).String(),
	}
	api := k.client.CoreV1().Secrets(k.namespace)
	results, err := api.List(ctx, listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, secret := range results.Items {
		if secret.Annotations[modelIdKey] != k.modelUUID {
			continue
		}
		err := api.Delete(ctx, secret.Name, v1.DeleteOptions{
			PropagationPolicy: constants.DefaultPropagationPolicy(),
		})
		if err == nil || k8serrors.IsNotFound(err) {
			continue
		}
		return errors.Trace(err)
	}
	return nil
}

func (k *kubernetesClient) createRole(
	ctx context.Context, role *rbacv1.Role,
) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client.RbacV1().Roles(k.namespace).Create(
		ctx, role, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("role %q", role.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteRoles(ctx context.Context) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	listOps := v1.ListOptions{
		LabelSelector: modelLabelSelector(k.modelName).String(),
	}
	api := k.client.RbacV1().Roles(k.namespace)
	results, err := api.List(ctx, listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, role := range results.Items {
		if role.Annotations[modelIdKey] != k.modelUUID {
			continue
		}
		err := api.Delete(ctx, role.Name, v1.DeleteOptions{
			PropagationPolicy: constants.DefaultPropagationPolicy(),
		})
		if err == nil || k8serrors.IsNotFound(err) {
			continue
		}
		return errors.Trace(err)
	}
	return nil
}

func (k *kubernetesClient) deleteRoleBindings(ctx context.Context) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	listOps := v1.ListOptions{
		LabelSelector: modelLabelSelector(k.modelName).String(),
	}
	api := k.client.RbacV1().RoleBindings(k.namespace)
	results, err := api.List(ctx, listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, roleBinding := range results.Items {
		if roleBinding.Annotations[modelIdKey] != k.modelUUID {
			continue
		}
		err := api.Delete(ctx, roleBinding.Name, v1.DeleteOptions{
			PropagationPolicy: constants.DefaultPropagationPolicy(),
		})
		if err == nil || k8serrors.IsNotFound(err) {
			continue
		}
		return errors.Trace(err)
	}
	return nil
}

func (k *kubernetesClient) updateRole(
	ctx context.Context, role *rbacv1.Role,
) (*rbacv1.Role, error) {
	api := k.client.RbacV1().Roles(k.namespace)

	var out *rbacv1.Role
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			patch := map[string]interface{}{
				"rules": role.Rules,
			}
			data, err := json.Marshal(patch)
			if err != nil {
				return errors.Annotatef(err, "marshaling role patch")
			}
			out, err = api.Patch(
				ctx, role.GetName(), types.StrategicMergePatchType, data,
				v1.PatchOptions{
					FieldManager: resources.JujuFieldManager,
				},
			)
			if k8serrors.IsNotFound(err) {
				return errors.NotFoundf("role %q", role.GetName())
			}
			return errors.Annotatef(err, "patching role %q", role.GetName())
		},
		IsFatalError: func(err error) bool {
			return !k8serrors.IsConflict(err)
		},
		Clock:       jujuclock.WallClock,
		Attempts:    5,
		Delay:       time.Second,
		BackoffFunc: retry.ExpBackoff(time.Second, 5*time.Second, 1.5, true),
	})

	return out, errors.Annotatef(err, "updating role %q", role.GetName())
}

func (k *kubernetesClient) deleteRole(ctx context.Context, name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client.RbacV1().Roles(k.namespace).Delete(ctx, name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) createRoleBinding(
	ctx context.Context, rb *rbacv1.RoleBinding,
) (_ *rbacv1.RoleBinding, cleanups []func(), err error) {
	if k.namespace == "" {
		return nil, nil, errNoNamespace
	}

	api := k.client.RbacV1().RoleBindings(k.namespace)
	out, err := api.Create(ctx, rb, v1.CreateOptions{
		FieldManager: resources.JujuFieldManager,
	})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	cleanups = append(cleanups, func() {
		_ = k.deleteRoleBinding(ctx, out.GetName(), out.GetUID())
	})

	return out, cleanups, nil
}

func (k *kubernetesClient) deleteRoleBinding(
	ctx context.Context, name string, uid types.UID,
) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client.RbacV1().RoleBindings(k.namespace).Delete(
		ctx, name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

// policyRulesForSecretAccess returns the full policy rules required for
// secrets.
func policyRulesForSecretAccess(
	namespace string, owned, read []string,
) []rbacv1.PolicyRule {
	rules := []rbacv1.PolicyRule{{
		APIGroups:     []string{rbacv1.APIGroupAll},
		Resources:     []string{"namespaces"},
		Verbs:         []string{"get", "list"},
		ResourceNames: []string{namespace},
	}}
	if len(owned) > 0 {
		// owned cannot be empty, otherwise this policy rule grants access to
		// all secrets.
		rules = append(rules, rbacv1.PolicyRule{
			APIGroups: []string{rbacv1.APIGroupAll},
			Resources: []string{"secrets"},
			Verbs: []string{
				// NOTE: create is not given here as it cannot be enforced due
				// to kubernetes rbac limitation.
				"get", "patch", "update", "replace", "delete",
			},
			ResourceNames: owned,
		})
	}
	if len(read) > 0 {
		// read cannot be empty, otherwise this policy rule grants access to
		// all secrets.
		rules = append(rules, rbacv1.PolicyRule{
			APIGroups:     []string{rbacv1.APIGroupAll},
			Resources:     []string{"secrets"},
			Verbs:         []string{"get"},
			ResourceNames: read,
		})
	}
	return rules
}

func (k *kubernetesClient) createRoleAndBinding(
	ctx context.Context, sa *core.ServiceAccount, rules []rbacv1.PolicyRule,
) (cleanups []func(), _ error) {
	role, err := k.createRole(ctx,
		&rbacv1.Role{
			ObjectMeta: v1.ObjectMeta{
				Name:        sa.Name,
				Namespace:   k.namespace,
				Labels:      sa.Labels,
				Annotations: sa.Annotations,
			},
			Rules: rules,
		},
	)
	if err != nil {
		return cleanups, errors.Annotatef(err, "creating role %q", sa.Name)
	}
	cleanups = append(cleanups, func() {
		_ = k.deleteRole(ctx, role.GetName(), role.GetUID())
	})

	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        sa.Name,
			Namespace:   k.namespace,
			Labels:      sa.Labels,
			Annotations: sa.Annotations,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     sa.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}
	out, rbCleanups, err := k.createRoleBinding(ctx, rb)
	if err != nil {
		return cleanups, errors.Trace(err)
	}
	cleanups = append(cleanups, rbCleanups...)

	// Ensure role binding exists before we return to avoid a race where a
	// client attempts to perform an operation before the role is allowed.
	return cleanups, errors.Trace(retry.Call(retry.CallArgs{
		Func: func() error {
			api := k.client.RbacV1().RoleBindings(k.namespace)
			_, err := api.Get(ctx, out.Name, v1.GetOptions{
				ResourceVersion: out.ResourceVersion,
			})
			if k8serrors.IsNotFound(err) {
				return errors.NewNotFound(err, "k8s")
			}
			return errors.Trace(err)
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotFound)
		},
		Clock:    jujuclock.WallClock,
		Attempts: 5,
		Delay:    time.Second,
	}))
}

func (k *kubernetesClient) createClusterRole(ctx context.Context, clusterRole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	out, err := k.client.RbacV1().ClusterRoles().Create(ctx, clusterRole, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("cluster role %q", clusterRole.GetName())
	}
	return out, errors.Trace(err)
}

// updateClusterRole fetches the latest version of the specified ClusterRole,
// replaces its Rules with those from the provided clusterRole, and updates it
// in the cluster. This method retries on conflicts using exponential backoff
// to handle concurrent modifications by other controllers.
// Note that only the Rules field is updated, all other fields from the latest ClusterRole are preserved.
func (k *kubernetesClient) updateClusterRole(ctx context.Context, clusterRole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	api := k.client.RbacV1().ClusterRoles()

	var out *rbacv1.ClusterRole
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			patch := map[string]interface{}{
				"rules": clusterRole.Rules,
			}
			data, err := json.Marshal(patch)
			if err != nil {
				return errors.Annotatef(err, "marshaling cluster role patch")
			}
			out, err = api.Patch(ctx, clusterRole.GetName(), types.StrategicMergePatchType, data, v1.PatchOptions{
				FieldManager: resources.JujuFieldManager,
			})
			if k8serrors.IsNotFound(err) {
				return errors.NotFoundf("cluster role %q", clusterRole.GetName())
			}
			return errors.Annotatef(err, "patching cluster role %q", clusterRole.GetName())
		},
		IsFatalError: func(err error) bool {
			return !k8serrors.IsConflict(err)
		},
		Clock:       jujuclock.WallClock,
		Attempts:    5,
		Delay:       time.Second,
		BackoffFunc: retry.ExpBackoff(time.Second, 5*time.Second, 1.5, true),
	})

	return out, errors.Annotatef(err, "updating cluster role %q", clusterRole.GetName())
}

func (k *kubernetesClient) deleteClusterRole(ctx context.Context, name string, uid types.UID) error {
	err := k.client.RbacV1().ClusterRoles().Delete(ctx, name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) createClusterRoleBinding(ctx context.Context, clusterRoleBinding *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, error) {
	out, err := k.client.RbacV1().ClusterRoleBindings().Create(ctx, clusterRoleBinding, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("cluster role binding %q", clusterRoleBinding.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteClusterRoleBinding(ctx context.Context, name string, uid types.UID) error {
	err := k.client.RbacV1().ClusterRoleBindings().Delete(ctx, name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteClusterRoles(ctx context.Context) error {
	listOps := v1.ListOptions{
		LabelSelector: modelLabelSelector(k.modelName).String(),
	}
	results, err := k.client.RbacV1().ClusterRoles().List(ctx, listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, clusterRole := range results.Items {
		if clusterRole.Annotations[modelIdKey] != k.modelUUID {
			continue
		}
		err := k.client.RbacV1().ClusterRoles().Delete(ctx, clusterRole.Name, v1.DeleteOptions{
			PropagationPolicy: constants.DefaultPropagationPolicy(),
		})
		if err == nil || k8serrors.IsNotFound(err) {
			continue
		}
		return errors.Trace(err)
	}
	return nil
}

func (k *kubernetesClient) deleteClusterRoleBindings(ctx context.Context) error {
	listOps := v1.ListOptions{
		LabelSelector: modelLabelSelector(k.modelName).String(),
	}
	results, err := k.client.RbacV1().ClusterRoleBindings().List(ctx, listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, clusterRoleBinding := range results.Items {
		if clusterRoleBinding.Annotations[modelIdKey] != k.modelUUID {
			continue
		}
		err := k.client.RbacV1().ClusterRoleBindings().Delete(ctx, clusterRoleBinding.Name, v1.DeleteOptions{
			PropagationPolicy: constants.DefaultPropagationPolicy(),
		})
		if err == nil || k8serrors.IsNotFound(err) {
			continue
		}
		return errors.Trace(err)
	}
	return nil
}

// ensureControllerClusterBindingForSecretAccessToken creates the cluster role
// and role binding needed to access the supplied secrets for the controller.
// If a new cluster role is created, cleanups contain funcs than can be run to
// delete any new resources on error.
func (k *kubernetesClient) createClusterRoleAndBinding(
	ctx context.Context, sa *core.ServiceAccount,
	rules []rbacv1.PolicyRule,
) (cleanups []func(), _ error) {
	cr, err := k.createClusterRole(ctx,
		&rbacv1.ClusterRole{
			ObjectMeta: v1.ObjectMeta{
				Name:        sa.Name,
				Labels:      sa.Labels,
				Annotations: sa.Annotations,
			},
			Rules: rules,
		},
	)
	if err != nil {
		return cleanups, errors.Annotatef(
			err, "creating cluster role %q", sa.Name)
	}
	cleanups = append(cleanups, func() {
		_ = k.deleteClusterRole(ctx, cr.GetName(), cr.GetUID())
	})

	crb, err := k.createClusterRoleBinding(ctx,
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: v1.ObjectMeta{
				Name:        sa.Name,
				Labels:      sa.Labels,
				Annotations: sa.Annotations,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     sa.Name,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      sa.Name,
					Namespace: sa.Namespace,
				},
			},
		},
	)
	if err != nil {
		return cleanups, errors.Annotatef(
			err, "creating cluster role binding %q", sa.Name)
	}
	cleanups = append(cleanups, func() {
		_ = k.deleteClusterRoleBinding(ctx, crb.GetName(), crb.GetUID())
	})

	// Ensure role binding exists before we return to avoid a race where a
	// client attempts to perform an operation before the role is allowed.
	return cleanups, errors.Trace(retry.Call(retry.CallArgs{
		Func: func() error {
			api := k.client.RbacV1().ClusterRoleBindings()
			_, err := api.Get(ctx, crb.Name, v1.GetOptions{
				ResourceVersion: crb.ResourceVersion,
			})
			if k8serrors.IsNotFound(err) {
				return errors.NewNotFound(err, "k8s")
			}
			return errors.Trace(err)
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotFound)
		},
		Clock:    jujuclock.WallClock,
		Attempts: 5,
		Delay:    time.Second,
	}))
}

// precreateSecretRevs ensures that a secret exists for a secret revision.
func (k *kubernetesClient) precreateSecretRevs(
	ctx context.Context, revs []string,
) error {
	labels := labelsForSecretRevision(k.modelName, k.modelUUID)
	client := k.client.CoreV1().Secrets(k.namespace)
	existingSecrets, err := client.List(ctx, v1.ListOptions{
		LabelSelector: labels.AsSelector().String(),
	})
	if err != nil {
		return errors.Trace(err)
	}

	existing := set.NewStrings()
	for _, secret := range existingSecrets.Items {
		existing.Add(secret.Name)
	}

	tmpl := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: k.namespace,
			Labels:    labels,
		},
		Type: core.SecretTypeOpaque,
	}
	for _, name := range revs {
		if existing.Contains(name) {
			continue
		}
		tmpl.Name = name
		_, err := client.Create(ctx, tmpl, v1.CreateOptions{
			FieldManager: resources.JujuFieldManager,
		})
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return errors.Trace(err)
		}
	}

	return nil
}

func (k *kubernetesClient) createSecretAccessToken(
	ctx context.Context,
	issuedTokenUUID string,
	consumer names.Tag,
	ownedRevs []string,
	readRevs []string,
) (_ string, err error) {
	var cleanups []func()
	defer func() {
		if err == nil {
			return
		}
		logger.Warningf("error ensuring secret service account for %q: %v", consumer, err)
		for _, f := range cleanups {
			f()
		}
	}()

	expireAt := time.Now().Add(coresecrets.IssuedTokenValidity)

	labels := labelsForServiceAccount(k.modelName, k.modelUUID, consumer)
	annotations := map[string]string{
		controllerIdKey:              k.controllerUUID,
		modelIdKey:                   k.modelUUID,
		annotationJujuSecretExpireAt: strconv.FormatInt(expireAt.Unix(), 10),
	}

	appName := consumer.Id()
	if consumer.Kind() == names.UnitTagKind {
		appName, _ = names.UnitApplication(consumer.Id())
	}
	labels = utils.LabelsMerge(labels,
		map[string]string{
			constants.LabelKubernetesAppName: appName,
		})

	// Service Account name and all the ACLs for this SA are derived from the
	// issued token UUID. This allows juju to revoke the issued token and
	// perform cleanup of tokens.
	serviceAccountName := fmt.Sprintf(
		"juju-secret-consumer-%s", issuedTokenUUID,
	)

	automountServiceAccountToken := true
	sa := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        serviceAccountName,
			Labels:      labels,
			Annotations: annotations,
			Namespace:   k.namespace,
		},
		AutomountServiceAccountToken: &automountServiceAccountToken,
	}
	sa, err = k.createServiceAccount(ctx, sa)
	if err != nil {
		return "", errors.Annotatef(err, "cannot ensure service account %q", serviceAccountName)
	}
	cleanups = append(cleanups, func() {
		_ = k.deleteServiceAccount(ctx, sa.Name, sa.UID)
	})

	rules := policyRulesForSecretAccess(k.namespace, ownedRevs, readRevs)
	rCleanups, err := k.createRoleAndBinding(ctx, sa, rules)
	cleanups = append(cleanups, rCleanups...)
	if err != nil {
		return "", errors.Annotatef(err, "cannot ensure role binding for secret access token for %q", sa.Name)
	}

	if k.isControllerModel {
		// We need to be able to list/get all namespaces for units in controller
		// model.
		clusterRules := append([]rbacv1.PolicyRule{{
			APIGroups: []string{rbacv1.APIGroupAll},
			Resources: []string{"namespaces"},
			Verbs:     []string{"get", "list"},
		}}, rules...)
		cbCleanups, err := k.createClusterRoleAndBinding(ctx, sa, clusterRules)
		cleanups = append(cleanups, cbCleanups...)
		if err != nil {
			return "", errors.Annotatef(err, "cannot ensure cluster binding for secret access token for %q", sa.Name)
		}
	}

	treq := &authenticationv1.TokenRequest{
		ObjectMeta: v1.ObjectMeta{
			Name: sa.Name,
		},
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: func() *int64 {
				until := time.Until(expireAt)
				seconds := int64(max(minExpireSeconds, until.Seconds()))
				return &seconds
			}(),
		},
	}
	tr, err := k.client.CoreV1().ServiceAccounts(k.namespace).CreateToken(
		ctx, sa.Name, treq, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	if err != nil {
		return "", errors.Annotatef(err, "cannot request a token for %q", sa.Name)
	}
	return tr.Status.Token, nil
}

// revokeSecretAccessTokens removes all the roles, role bindings and service
// accounts related to the named issued token UUID.
func (k *kubernetesClient) revokeSecretAccessToken(
	ctx context.Context, issuedTokenUUID string,
) error {
	if k.namespace == "" {
		return errNoNamespace
	}

	serviceAccountName := fmt.Sprintf(
		"juju-secret-consumer-%s", issuedTokenUUID,
	)

	err := k.client.RbacV1().ClusterRoleBindings().Delete(
		ctx, serviceAccountName, *v1.NewDeleteOptions(0))
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}

	err = k.client.RbacV1().ClusterRoles().Delete(
		ctx, serviceAccountName, v1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}

	err = k.client.RbacV1().RoleBindings(k.namespace).Delete(
		ctx, serviceAccountName, *v1.NewDeleteOptions(0))
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}

	err = k.client.RbacV1().Roles(k.namespace).Delete(
		ctx, serviceAccountName, v1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}

	err = k.client.CoreV1().ServiceAccounts(k.namespace).Delete(
		ctx, serviceAccountName, v1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}

	return nil
}

// filterRemovedSecretsPolicyRules removes from the given rules access to the
// specified secret revisions. When the second return value is false, the policy
// is already up to date.
func filterRemovedSecretsPolicyRules(
	rules []rbacv1.PolicyRule, removed []string,
) ([]rbacv1.PolicyRule, bool) {
	toRemove := set.NewStrings(removed...)
	needChange := false
	for _, rule := range rules {
		if slices.Contains(rule.Resources, "secrets") &&
			slices.ContainsFunc(rule.ResourceNames, toRemove.Contains) {
			needChange = true
			break
		}
	}
	if !needChange {
		return nil, false
	}
	var out []rbacv1.PolicyRule
	for _, rule := range rules {
		if slices.Contains(rule.Resources, "secrets") {
			rule.ResourceNames = slices.DeleteFunc(
				rule.ResourceNames, toRemove.Contains)
			if len(rule.ResourceNames) == 0 {
				continue
			}
		}
		out = append(out, rule)
	}
	return out, true
}

func (k *kubernetesClient) dropSecretAccess(
	ctx context.Context, removed []string,
) error {
	labels := labelsForServiceAccount(k.modelName, k.modelUUID, nil)

	listOps := v1.ListOptions{
		LabelSelector: labels.AsSelector().String(),
	}

	clusterRoles, err := k.client.RbacV1().ClusterRoles().List(ctx, listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, clusterRole := range clusterRoles.Items {
		var changed bool
		clusterRole.Rules, changed = filterRemovedSecretsPolicyRules(
			clusterRole.Rules, removed)
		if !changed {
			continue
		}
		_, err := k.updateClusterRole(ctx, &clusterRole)
		if errors.Is(err, errors.NotFound) {
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
	}

	roles, err := k.client.RbacV1().Roles(k.namespace).List(ctx, listOps)
	if err != nil {
		return errors.Trace(err)
	}
	for _, role := range roles.Items {
		var changed bool
		role.Rules, changed = filterRemovedSecretsPolicyRules(role.Rules, removed)
		if !changed {
			continue
		}
		_, err := k.updateRole(ctx, &role)
		if errors.Is(err, errors.NotFound) {
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

var errNoNamespace = errors.ConstError("no namespace")

// NewBackend returns a k8s backed secrets backend.
func (p k8sProvider) NewBackend(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
	broker, err := p.getBroker(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "getting cluster client")
	}
	return &k8sBackend{
		modelName:      cfg.ModelName,
		modelUUID:      cfg.ModelUUID,
		namespace:      broker.namespace,
		serviceAccount: broker.serviceAccount,
		client:         broker.client,
	}, nil
}

func maybePermissionDenied(err error) error {
	if k8serrors.IsForbidden(err) || k8serrors.IsUnauthorized(err) {
		return errors.WithType(err, secrets.PermissionDenied)
	}
	return err
}

// RefreshAuth implements SupportAuthRefresh.
func (p k8sProvider) RefreshAuth(adminCfg *provider.ModelBackendConfig, validFor time.Duration) (_ *provider.BackendConfig, err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	broker, err := p.getBroker(adminCfg)
	if err != nil {
		return nil, errors.Annotate(err, "getting cluster client")
	}
	validForSeconds := int64(validFor.Truncate(time.Second).Seconds())

	treq := &authenticationv1.TokenRequest{
		ObjectMeta: v1.ObjectMeta{
			Name: broker.serviceAccount,
		},
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &validForSeconds,
		},
	}
	tr, err := broker.client.CoreV1().ServiceAccounts(broker.namespace).CreateToken(
		context.TODO(), broker.serviceAccount, treq, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot request a token for %q", broker.serviceAccount)
	}

	cfgCopy := adminCfg.BackendConfig
	cfgCopy.Config[tokenKey] = tr.Status.Token
	return &cfgCopy, nil
}
