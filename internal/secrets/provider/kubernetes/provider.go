// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	authenticationv1 "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudspec"
	internallogger "github.com/juju/juju/internal/logger"
	k8sprovider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
)

var logger = internallogger.GetLogger("juju.secrets.provider.kubernetes")

const (
	// BackendName is the name of the Kubernetes secrets backend.
	BackendName = "kubernetes"
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
func (p k8sProvider) CleanupModel(ctx context.Context, cfg *provider.ModelBackendConfig) error {
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
	return broker, errors.Trace(err)
}

func configToK8sRestConfig(cfg *backendConfig) (*rest.Config, error) {
	if cfg.preferInClusterAddress() {
		rc, err := InClusterConfig()
		if err != nil && !errors.Is(err, rest.ErrNotInCluster) {
			return nil, errors.Trace(err)
		}
		if rc != nil {
			logger.Tracef(context.TODO(), "using in-cluster config")
			return rc, nil
		}
	}

	cacerts := cfg.caCerts()
	var caData []byte
	for _, cacert := range cacerts {
		caData = append(caData, cacert...)
	}

	rcfg := &rest.Config{
		Host:        cfg.endpoint(),
		Username:    cfg.username(),
		Password:    cfg.password(),
		BearerToken: cfg.token(),
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte(cfg.clientCert()),
			KeyData:  []byte(cfg.clientKey()),
			CAData:   caData,
			Insecure: cfg.skipTLSVerify(),
		},
	}
	return rcfg, nil
}

// CleanupSecrets removes rules of the role associated with the removed secrets.
func (p k8sProvider) CleanupSecrets(ctx context.Context, cfg *provider.ModelBackendConfig, accessor coresecrets.Accessor, removed provider.SecretRevisions) error {
	if len(removed) == 0 {
		return nil
	}

	broker, err := p.getBroker(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = broker.ensureSecretAccessToken(ctx, accessor, nil, nil, removed.RevisionIDs())
	return errors.Trace(err)
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
func BuiltInConfig(controllerName, modelName string, cloudSpec cloudspec.CloudSpec) (*provider.BackendConfig, error) {
	cfg, err := cloudSpecToBackendConfig(cloudSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	namespace := modelName
	if modelName == bootstrap.ControllerModelName {
		namespace = k8sprovider.DecideControllerNamespace(controllerName)
	}
	cfg.Config[namespaceKey] = namespace
	return cfg, nil
}

// BuiltInName returns the backend name for the k8s in-model backend.
func BuiltInName(modelName string) string {
	return modelName + "-local"
}

// RestrictedConfig returns the config needed to create a
// secrets backend client restricted to manage the specified
// owned secrets and read shared secrets for the given entity tag.
func (p k8sProvider) RestrictedConfig(
	ctx context.Context,
	adminCfg *provider.ModelBackendConfig, sameController, _ bool, accessor coresecrets.Accessor, owned provider.SecretRevisions, read provider.SecretRevisions,
) (*provider.BackendConfig, error) {
	logger.Tracef(context.TODO(), "getting k8s backend config for %q, owned %v, read %v", accessor, owned, read)

	if accessor.Kind != coresecrets.UnitAccessor && accessor.Kind != coresecrets.ModelAccessor {
		return nil, errors.NotValidf("secret accessor %s", accessor)
	}

	cfg, err := newConfig(adminCfg.Config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker, err := p.getBroker(adminCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	token, err := broker.ensureSecretAccessToken(ctx, accessor, owned.RevisionIDs(), read.RevisionIDs(), nil)
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
			logger.Tracef(context.TODO(), "patching endpoint to %q", endpoint)
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
var expiresInSeconds = int64(60 * 10)

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
		if existing.Annotations[modelIdKey] != k.modelUUID {
			// sa.Name is already used for an existing service account.
			return nil, errors.Errorf("service account %q exists and is not managed by this model", sa.GetName())
		}
		return nil, errors.AlreadyExistsf("service account %q", sa.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateServiceAccount(ctx context.Context, sa *core.ServiceAccount) (*core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client.CoreV1().ServiceAccounts(k.namespace).Update(ctx, sa, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("service account %q", sa.GetName())
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

// ensureServiceAccount creates or updates a service account, disambiguating the name if necessary.
// If a new service account is created, cleanups contain funcs than can be run to delete any new
// resources on error.
func (k *kubernetesClient) ensureServiceAccount(
	ctx context.Context, serviceAccountName string, labels labels.Set, annotations map[string]string, disambiguateName bool,
) (out *core.ServiceAccount, cleanups []func(), err error) {
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
	if disambiguateName {
		out, err = k.createDisambiguatedServiceAccount(ctx, sa)
	} else {
		out, err = k.createServiceAccount(ctx, sa)
		if err != nil && !errors.Is(err, errors.AlreadyExists) {
			return nil, nil, errors.Trace(err)
		}
	}
	if err == nil {
		logger.Debugf(context.TODO(), "service account %q created", out.GetName())
		cleanups = append(cleanups, func() { _ = k.deleteServiceAccount(ctx, out.GetName(), out.GetUID()) })
		return out, cleanups, nil
	}

	// Service account already exists so update it.
	out, err = k.updateServiceAccount(ctx, sa)
	logger.Debugf(context.TODO(), "updating service account %q", sa.GetName())
	return out, cleanups, errors.Trace(err)
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

func (k *kubernetesClient) createRole(ctx context.Context, role *rbacv1.Role) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client.RbacV1().Roles(k.namespace).Create(ctx, role, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("role %q", role.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateRole(ctx context.Context, role *rbacv1.Role) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client.RbacV1().Roles(k.namespace).Update(ctx, role, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role %q", role.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getRole(ctx context.Context, name string) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client.RbacV1().Roles(k.namespace).Get(ctx, name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("role %q", name)
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

func (k *kubernetesClient) ensureRoleBinding(
	ctx context.Context, rb *rbacv1.RoleBinding,
) (_ *rbacv1.RoleBinding, cleanups []func(), err error) {
	if k.namespace == "" {
		return nil, cleanups, errNoNamespace
	}

	api := k.client.RbacV1().RoleBindings(k.namespace)
	out, err := api.Get(ctx, rb.Name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		out, err = api.Create(ctx, rb, v1.CreateOptions{
			FieldManager: resources.JujuFieldManager,
		})
		if err == nil {
			cleanups = append(cleanups, func() { _ = k.deleteRoleBinding(ctx, out.GetName(), out.GetUID()) })
		}
	}
	return out, cleanups, errors.Trace(err)
}

func (k *kubernetesClient) deleteRoleBinding(ctx context.Context, name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client.RbacV1().RoleBindings(k.namespace).Delete(ctx, name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func cleanRules(existing []rbacv1.PolicyRule, shouldRemove func(string) bool) []rbacv1.PolicyRule {
	if len(existing) == 0 {
		return nil
	}

	i := 0
	for _, r := range existing {
		if len(r.ResourceNames) == 1 && shouldRemove(r.ResourceNames[0]) {
			continue
		}
		existing[i] = r
		i++
	}
	return existing[:i]
}

func rulesForSecretAccess(
	namespace string, isControllerModel bool,
	existing []rbacv1.PolicyRule, owned, read, removed []string,
) []rbacv1.PolicyRule {
	if len(existing) == 0 {
		existing = []rbacv1.PolicyRule{
			{
				APIGroups: []string{rbacv1.APIGroupAll},
				Resources: []string{"secrets"},
				Verbs: []string{
					"create",
					"patch", // TODO: we really should only allow "create" but not patch  but currently we uses .Apply() which requres patch!!!
				},
			},
		}
		if isControllerModel {
			// We need to be able to list/get all namespaces for units in controller model.
			existing = append(existing, rbacv1.PolicyRule{
				APIGroups: []string{rbacv1.APIGroupAll},
				Resources: []string{"namespaces"},
				Verbs:     []string{"get", "list"},
			})
		} else {
			// We just need to be able to list/get our own namespace for units in other models.
			existing = append(existing, rbacv1.PolicyRule{
				APIGroups:     []string{rbacv1.APIGroupAll},
				Resources:     []string{"namespaces"},
				Verbs:         []string{"get", "list"},
				ResourceNames: []string{namespace},
			})
		}
	}

	ownedIDs := set.NewStrings(owned...)
	readIDs := set.NewStrings(read...)
	removedIDs := set.NewStrings(removed...)

	existing = cleanRules(existing,
		func(s string) bool {
			return ownedIDs.Contains(s) || readIDs.Contains(s) || removedIDs.Contains(s)
		},
	)

	for _, rName := range owned {
		if removedIDs.Contains(rName) {
			continue
		}
		existing = append(existing, rbacv1.PolicyRule{
			APIGroups:     []string{rbacv1.APIGroupAll},
			Resources:     []string{"secrets"},
			Verbs:         []string{rbacv1.VerbAll},
			ResourceNames: []string{rName},
		})
	}
	for _, rName := range read {
		if removedIDs.Contains(rName) {
			continue
		}
		existing = append(existing, rbacv1.PolicyRule{
			APIGroups:     []string{rbacv1.APIGroupAll},
			Resources:     []string{"secrets"},
			Verbs:         []string{"get"},
			ResourceNames: []string{rName},
		})
	}
	return existing
}

// ensureBindingForSecretAccessToken creates the role and role binding needed to access the supplied secrets.
// If a new role is created, cleanups contain funcs than can be run to delete any new
// resources on error.
func (k *kubernetesClient) ensureBindingForSecretAccessToken(
	ctx context.Context, sa *core.ServiceAccount, owned, read, removed []string,
) (cleanups []func(), _ error) {
	role, err := k.getRole(ctx, sa.Name)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return cleanups, errors.Trace(err)
	}
	if errors.Is(err, errors.NotFound) {
		_, err = k.createRole(ctx,
			&rbacv1.Role{
				ObjectMeta: v1.ObjectMeta{
					Name:        sa.Name,
					Namespace:   k.namespace,
					Labels:      sa.Labels,
					Annotations: sa.Annotations,
				},
				Rules: rulesForSecretAccess(k.namespace, false, nil, owned, read, removed),
			},
		)
		if err == nil {
			cleanups = append(cleanups, func() { _ = k.deleteRole(ctx, role.GetName(), role.GetUID()) })
		}
	} else {
		role.Rules = rulesForSecretAccess(k.namespace, false, role.Rules, owned, read, removed)
		_, err = k.updateRole(ctx, role)
	}
	if err != nil {
		return cleanups, errors.Trace(err)
	}
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
	out, cleanups, err := k.ensureRoleBinding(ctx, rb)
	if err != nil {
		return cleanups, errors.Trace(err)
	}

	// Ensure role binding exists before we return to avoid a race where a client
	// attempts to perform an operation before the role is allowed.
	return cleanups, errors.Trace(retry.Call(retry.CallArgs{
		Func: func() error {
			api := k.client.RbacV1().RoleBindings(k.namespace)
			_, err := api.Get(ctx, out.Name, v1.GetOptions{ResourceVersion: out.ResourceVersion})
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

func (k *kubernetesClient) updateClusterRole(ctx context.Context, clusterRole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	out, err := k.client.RbacV1().ClusterRoles().Update(ctx, clusterRole, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("cluster role %q", clusterRole.GetName())
	}
	return out, errors.Trace(err)
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

// ensureClusterBindingForSecretAccessToken creates the cluster role and role binding needed
// to access the supplied secrets.
// If a new cluster role is created, cleanups contain funcs than can be run to delete any new
// resources on error.
func (k *kubernetesClient) ensureClusterBindingForSecretAccessToken(
	ctx context.Context, saName, baseName string, labels labels.Set, annotations map[string]string, owned, read, removed []string,
) (cleanups []func(), _ error) {
	createRules := func(existing []rbacv1.PolicyRule) []rbacv1.PolicyRule {
		return rulesForSecretAccess(k.namespace, true, existing, owned, read, removed)
	}
	clusterRole, crCleanups, err := k.ensureDisambiguatedClusterRole(ctx, baseName, labels, annotations, createRules)
	if err == nil {
		cleanups = append(cleanups, crCleanups...)
	} else {
		return cleanups, errors.Annotatef(err, "disambiguating cluster role name %q", baseName)
	}

	clusterRoleBinding, crbCleanups, err := k.ensureDisambiguatedClusterRoleBinding(ctx, saName, baseName, clusterRole.Name, labels, annotations)
	if err == nil {
		cleanups = append(cleanups, crbCleanups...)
	} else {
		return cleanups, errors.Annotatef(err, "disambiguating cluster role binding name %q", baseName)
	}

	// Ensure role binding exists before we return to avoid a race where a client
	// attempts to perform an operation before the role is allowed.
	return cleanups, errors.Trace(retry.Call(retry.CallArgs{
		Func: func() error {
			api := k.client.RbacV1().ClusterRoleBindings()
			_, err := api.Get(ctx, clusterRoleBinding.Name, v1.GetOptions{ResourceVersion: clusterRoleBinding.ResourceVersion})
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

const (
	maxResourceNameLength = 63
	clusterResourcePrefix = "juju-secrets-"
)

// ensureSecretAccessToken ensures the RBAC resources created and updated for the provided resource name.
// Any revisions listed in removed have access revoked.
func (k *kubernetesClient) ensureSecretAccessToken(
	ctx context.Context, consumer coresecrets.Accessor, owned, read, removed []string,
) (_ string, err error) {
	var cleanups []func()
	defer func() {
		if err == nil {
			return
		}
		logger.Warningf(context.TODO(), "error ensuring secret service account for %s: %v", consumer, err)
		for _, f := range cleanups {
			f()
		}
	}()

	labels := labelsForServiceAccount(k.modelName, k.modelUUID)
	annotations := map[string]string{
		controllerIdKey: k.controllerUUID,
		modelIdKey:      k.modelUUID,
	}

	appName := consumer.ID
	if consumer.Kind == coresecrets.UnitAccessor {
		appName, _ = names.UnitApplication(consumer.ID)
	}
	labels = utils.LabelsMerge(labels,
		map[string]string{
			constants.LabelKubernetesAppName: appName,
		})

	// Compose the name of the service account and role and role binding.
	// We'll use the tag string, but for models we'll use the model name, since
	// the UUID will be used to disambiguate anyway if needed.
	baseResourceName := consumer.String()
	if consumer.Kind == coresecrets.ModelAccessor {
		baseResourceName = fmt.Sprintf("model-%s", k.modelName)
	}
	serviceAccountName := baseResourceName
	// For the controller model, the resources are cluster scoped so
	// given them a meaningful prefix.
	if k.isControllerModel {
		baseResourceName = clusterResourcePrefix + baseResourceName
	}
	// If the resources are going to a namespace other than that of the host model,
	// disambiguate the name.
	disambiguateName, err := k.isExternalNamespace()
	if err != nil {
		return "", errors.Trace(err)
	}

	sa, saCleanups, err := k.ensureServiceAccount(ctx, serviceAccountName, labels, annotations, disambiguateName)
	cleanups = append(cleanups, saCleanups...)
	if err != nil {
		return "", errors.Annotatef(err, "cannot ensure service account %q", serviceAccountName)
	}

	if k.isControllerModel {
		cbCleanups, err := k.ensureClusterBindingForSecretAccessToken(ctx, sa.Name, baseResourceName, labels, annotations, owned, read, removed)
		cleanups = append(cleanups, cbCleanups...)
		if err != nil {
			return "", errors.Trace(err)
		}
	} else {
		// For roles and role bindings created in the namespace set up to hold the secrets,
		// we assume that the service account, role, role binding all share the same disambiguated
		// name as the service account. This is reasonable since it's not expected that anything
		// other than Juju will be messing with such artefacts in that namespace.
		rCleanups, err := k.ensureBindingForSecretAccessToken(ctx, sa, owned, read, removed)
		cleanups = append(cleanups, rCleanups...)
		if err != nil {
			return "", errors.Trace(err)
		}
	}

	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expiresInSeconds,
		},
	}
	tr, err := k.client.CoreV1().ServiceAccounts(k.namespace).CreateToken(
		ctx, sa.Name, treq, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	if err != nil {
		return "", errors.Annotatef(err, "cannot request a token for %q", sa.Name)
	}
	return tr.Status.Token, nil
}

// createDisambiguatedServiceAccount creates a service account with a disambiguated name.
func (k *kubernetesClient) createDisambiguatedServiceAccount(
	ctx context.Context, sa *core.ServiceAccount,
) (*core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}

	listOps := v1.ListOptions{
		LabelSelector: modelLabelSelector(k.modelName).String(),
	}
	existing, err := k.client.CoreV1().ServiceAccounts(k.namespace).List(ctx, listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, existingServiceAccount := range existing.Items {
		if existingServiceAccount.Annotations[modelIdKey] == k.modelUUID {
			return &existingServiceAccount, nil
		}
	}

	suffixLength := model.DefaultSuffixDigits
	var proposedName string

	for {
		if proposedName, err = model.DisambiguateResourceNameWithSuffixLength(
			k.modelUUID, sa.Name, maxResourceNameLength, suffixLength); err != nil {
			return nil, errors.Annotatef(err, "disambiguating service account name %q", sa.Name)
		}
		_, err = k.client.CoreV1().ServiceAccounts(k.namespace).Get(ctx, proposedName, v1.GetOptions{})
		if err == nil {
			suffixLength = suffixLength + 1
			continue
		} else if !k8serrors.IsNotFound(err) {
			return nil, errors.Annotatef(err, "getting existing service account %q", proposedName)
		}
		sa.Name = proposedName
		return k.createServiceAccount(ctx, sa)
	}
}

// ensureDisambiguatedClusterRole creates a cluster role with a disambiguated name.
// cleanups contain funcs than can be run to delete any new resources on error.
func (k *kubernetesClient) ensureDisambiguatedClusterRole(
	ctx context.Context, baseName string, labels labels.Set, annotations map[string]string, createRules func(existing []rbacv1.PolicyRule) []rbacv1.PolicyRule,
) (_ *rbacv1.ClusterRole, cleanups []func(), _ error) {
	listOps := v1.ListOptions{
		LabelSelector: modelLabelSelector(k.modelName).String(),
	}
	existing, err := k.client.RbacV1().ClusterRoles().List(ctx, listOps)
	if err != nil {
		return nil, cleanups, errors.Trace(err)
	}
	for _, clusterRole := range existing.Items {
		if clusterRole.Annotations[modelIdKey] != k.modelUUID {
			continue
		}
		clusterRole.Rules = createRules(clusterRole.Rules)
		result, err := k.updateClusterRole(ctx, &clusterRole)
		if err != nil {
			return nil, cleanups, errors.Trace(err)
		}
		return result, cleanups, nil
	}

	suffixLength := model.DefaultSuffixDigits
	var proposedName string

	for {
		if proposedName, err = model.DisambiguateResourceNameWithSuffixLength(
			k.modelUUID, baseName, maxResourceNameLength, suffixLength); err != nil {
			return nil, cleanups, errors.Annotatef(err, "disambiguating cluster role name %q", baseName)
		}
		_, err = k.client.RbacV1().ClusterRoles().Get(ctx, proposedName, v1.GetOptions{})
		if err == nil {
			suffixLength = suffixLength + 1
			continue
		} else if !k8serrors.IsNotFound(err) {
			return nil, cleanups, errors.Annotatef(err, "getting existing cluster role %q", proposedName)
		}
		result, err := k.createClusterRole(ctx,
			&rbacv1.ClusterRole{
				ObjectMeta: v1.ObjectMeta{
					Name:        proposedName,
					Labels:      labels,
					Annotations: annotations,
				},
				Rules: createRules(nil),
			},
		)
		if err == nil {
			cleanups = append(cleanups, func() { _ = k.deleteClusterRole(ctx, result.GetName(), result.GetUID()) })
		}
		return result, cleanups, nil
	}
}

// ensureDisambiguatedClusterRoleBinding ensures a cluster role binding with a
// disambiguated name exists.
// cleanups contain funcs than can be run to delete any new resources on error.
func (k *kubernetesClient) ensureDisambiguatedClusterRoleBinding(
	ctx context.Context, saName, baseName, roleName string, labels labels.Set, annotations map[string]string,
) (_ *rbacv1.ClusterRoleBinding, cleanups []func(), _ error) {
	listOps := v1.ListOptions{
		LabelSelector: modelLabelSelector(k.modelName).String(),
	}
	existing, err := k.client.RbacV1().ClusterRoleBindings().List(ctx, listOps)
	if err != nil {
		return nil, cleanups, errors.Trace(err)
	}
	for _, clusterRoleBinding := range existing.Items {
		if clusterRoleBinding.Annotations[modelIdKey] == k.modelUUID {
			return &clusterRoleBinding, cleanups, nil
		}
	}

	suffixLength := model.DefaultSuffixDigits
	var proposedName string

	for {
		if proposedName, err = model.DisambiguateResourceNameWithSuffixLength(
			k.modelUUID, baseName, maxResourceNameLength, suffixLength); err != nil {
			return nil, cleanups, errors.Annotatef(err, "disambiguating cluster role name %q", baseName)
		}
		_, err = k.client.RbacV1().ClusterRoleBindings().Get(ctx, proposedName, v1.GetOptions{})
		if err == nil {
			suffixLength = suffixLength + 1
			continue
		} else if !k8serrors.IsNotFound(err) {
			return nil, cleanups, errors.Annotatef(err, "getting existing cluster role %q", proposedName)
		}
		result, err := k.createClusterRoleBinding(ctx,
			&rbacv1.ClusterRoleBinding{
				ObjectMeta: v1.ObjectMeta{
					Name:        proposedName,
					Labels:      labels,
					Annotations: annotations,
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     roleName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      saName,
						Namespace: k.namespace,
					},
				},
			},
		)
		if err == nil {
			cleanups = append(cleanups, func() { _ = k.deleteClusterRoleBinding(ctx, result.GetName(), result.GetUID()) })
		}
		return result, cleanups, errors.Trace(err)
	}
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
func (p k8sProvider) RefreshAuth(ctx context.Context, adminCfg provider.BackendConfig, validFor time.Duration) (_ *provider.BackendConfig, err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	validCfg, err := newConfig(adminCfg.Config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	restCfg, err := configToK8sRestConfig(validCfg)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid k8s config")
	}
	k8sClient, err := NewK8sClient(restCfg)
	if err != nil {
		return nil, errors.Annotate(err, "getting cluster client")
	}
	namespace := validCfg.namespace()
	serviceAccount := validCfg.serviceAccount()

	validForSeconds := int64(validFor.Truncate(time.Second).Seconds())

	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &validForSeconds,
		},
	}
	tr, err := k8sClient.CoreV1().ServiceAccounts(namespace).CreateToken(
		ctx, serviceAccount, treq, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot request a token for %q", serviceAccount)
	}

	cfgCopy := adminCfg
	cfgCopy.Config[tokenKey] = tr.Status.Token
	return &cfgCopy, nil
}
