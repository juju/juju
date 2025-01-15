// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"net"
	"os"
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
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
)

var logger = loggo.GetLogger("juju.secrets.provider.kubernetes")

const (
	// BackendType is the type of the Kubernetes secrets backend.
	BackendType = "kubernetes"
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

// CleanupModel is not used.
func (p k8sProvider) CleanupModel(*provider.ModelBackendConfig) error {
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
	_, err = broker.ensureSecretAccessToken(context.TODO(), tag, nil, nil, removed.RevisionIDs())
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

// RestrictedConfig returns the config needed to create a
// secrets backend client restricted to manage the specified
// owned secrets and read shared secrets for the given entity tag.
func (p k8sProvider) RestrictedConfig(
	adminCfg *provider.ModelBackendConfig, sameController, forDrain bool, consumer names.Tag, owned provider.SecretRevisions, read provider.SecretRevisions,
) (*provider.BackendConfig, error) {
	logger.Tracef("getting k8s backend config for %q, owned %v, read %v", consumer, owned, read)

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
	token, err := broker.ensureSecretAccessToken(ctx, consumer, owned.RevisionIDs(), read.RevisionIDs(), nil)
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

	serviceAccount    string
	namespace         string
	isControllerModel bool
}

// TODO: make this configurable.
var expiresInSeconds = int64(60 * 10)

func (k *kubernetesClient) createServiceAccount(ctx context.Context, sa *core.ServiceAccount) (*core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(sa)
	out, err := k.client.CoreV1().ServiceAccounts(k.namespace).Create(ctx, sa, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
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

func (k *kubernetesClient) listServiceAccount(ctx context.Context, labels map[string]string) ([]core.ServiceAccount, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	listOps := v1.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	}
	saList, err := k.client.CoreV1().ServiceAccounts(k.namespace).List(ctx, listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(saList.Items) == 0 {
		return nil, errors.NotFoundf("service account with labels %v", labels)
	}
	return saList.Items, nil
}

func (k *kubernetesClient) ensureServiceAccount(ctx context.Context, sa *core.ServiceAccount) (out *core.ServiceAccount, cleanups []func(), err error) {
	out, err = k.createServiceAccount(ctx, sa)
	if err == nil {
		logger.Debugf("service account %q created", out.GetName())
		cleanups = append(cleanups, func() { _ = k.deleteServiceAccount(ctx, out.GetName(), out.GetUID()) })
		return out, cleanups, nil
	}
	if !errors.IsAlreadyExists(err) {
		return nil, cleanups, errors.Trace(err)
	}
	_, err = k.listServiceAccount(ctx, sa.GetLabels())
	if err != nil {
		if errors.IsNotFound(err) {
			// sa.Name is already used for an existing service account.
			return nil, cleanups, errors.AlreadyExistsf("service account %q", sa.GetName())
		}
		return nil, cleanups, errors.Trace(err)
	}
	out, err = k.updateServiceAccount(ctx, sa)
	logger.Debugf("updating service account %q", sa.GetName())
	return out, cleanups, errors.Trace(err)
}

func (k *kubernetesClient) createRole(ctx context.Context, role *rbacv1.Role) (*rbacv1.Role, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(role)
	out, err := k.client.RbacV1().Roles(k.namespace).Create(ctx, role, v1.CreateOptions{})
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

func (k *kubernetesClient) ensureBindingForSecretAccessToken(ctx context.Context, sa *core.ServiceAccount, objMeta v1.ObjectMeta, owned, read, removed []string) error {
	if k.isControllerModel {
		return k.ensureClusterBindingForSecretAccessToken(ctx, sa, objMeta, owned, read, removed)
	}

	role, err := k.getRole(ctx, objMeta.Name)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	if errors.Is(err, errors.NotFound) {
		role, err = k.createRole(ctx,
			&rbacv1.Role{
				ObjectMeta: objMeta,
				Rules:      rulesForSecretAccess(k.namespace, false, nil, owned, read, removed),
			},
		)
	} else {
		role.Rules = rulesForSecretAccess(k.namespace, false, role.Rules, owned, read, removed)
		role, err = k.updateRole(ctx, role)
	}
	if err != nil {
		return errors.Trace(err)
	}
	bindingSpec := &rbacv1.RoleBinding{
		ObjectMeta: objMeta,
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}
	roleBinding := resources.NewRoleBinding(bindingSpec.Name, bindingSpec.Namespace, bindingSpec)
	err = roleBinding.Apply(ctx, k.client)
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure role binding exists before we return to avoid a race where a client
	// attempts to perform an operation before the role is allowed.
	return errors.Trace(retry.Call(retry.CallArgs{
		Func: func() error {
			api := k.client.RbacV1().RoleBindings(k.namespace)
			_, err := api.Get(ctx, roleBinding.Name, v1.GetOptions{ResourceVersion: roleBinding.ResourceVersion})
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

func (k *kubernetesClient) createClusterRole(ctx context.Context, clusterrole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(clusterrole)
	out, err := k.client.RbacV1().ClusterRoles().Create(ctx, clusterrole, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("clusterrole %q", clusterrole.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateClusterRole(ctx context.Context, clusterrole *rbacv1.ClusterRole) (*rbacv1.ClusterRole, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client.RbacV1().ClusterRoles().Update(ctx, clusterrole, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("clusterrole %q", clusterrole.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getClusterRole(ctx context.Context, name string) (*rbacv1.ClusterRole, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client.RbacV1().ClusterRoles().Get(ctx, name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("clusterrole %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) ensureClusterBindingForSecretAccessToken(ctx context.Context, sa *core.ServiceAccount, objMeta v1.ObjectMeta, owned, read, removed []string) error {
	objMeta.Name = fmt.Sprintf("%s-%s", k.namespace, objMeta.Name)
	clusterRole, err := k.getClusterRole(ctx, objMeta.Name)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	if errors.Is(err, errors.NotFound) {
		clusterRole, err = k.createClusterRole(ctx,
			&rbacv1.ClusterRole{
				ObjectMeta: objMeta,
				Rules:      rulesForSecretAccess(k.namespace, true, nil, owned, read, removed),
			},
		)
	} else {
		clusterRole.Rules = rulesForSecretAccess(k.namespace, true, clusterRole.Rules, owned, read, removed)
		clusterRole, err = k.updateClusterRole(ctx, clusterRole)
	}
	if err != nil {
		return errors.Trace(err)
	}
	bindingSpec := &rbacv1.ClusterRoleBinding{
		ObjectMeta: objMeta,
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRole.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}
	clusterRoleBinding := resources.NewClusterRoleBinding(bindingSpec.Name, bindingSpec)
	_, err = clusterRoleBinding.Ensure(ctx, k.client, resources.ClaimJujuOwnership)
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure role binding exists before we return to avoid a race where a client
	// attempts to perform an operation before the role is allowed.
	return errors.Trace(retry.Call(retry.CallArgs{
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

func (k *kubernetesClient) ensureSecretAccessToken(ctx context.Context, tag names.Tag, owned, read, removed []string) (string, error) {
	appName := tag.Id()
	if tag.Kind() == names.UnitTagKind {
		appName, _ = names.UnitApplication(tag.Id())
	}
	labels := utils.LabelsForApp(appName, false)

	objMeta := v1.ObjectMeta{
		Name:      tag.String(),
		Labels:    labels,
		Namespace: k.namespace,
	}

	automountServiceAccountToken := true
	sa := &core.ServiceAccount{
		ObjectMeta:                   objMeta,
		AutomountServiceAccountToken: &automountServiceAccountToken,
	}
	_, _, err := k.ensureServiceAccount(ctx, sa)
	if err != nil {
		return "", errors.Annotatef(err, "cannot ensure service account %q", sa.GetName())
	}

	if err := k.ensureBindingForSecretAccessToken(ctx, sa, objMeta, owned, read, removed); err != nil {
		return "", errors.Trace(err)
	}

	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expiresInSeconds,
		},
	}
	tr, err := k.client.CoreV1().ServiceAccounts(k.namespace).CreateToken(ctx, sa.Name, treq, v1.CreateOptions{})
	if err != nil {
		return "", errors.Annotatef(err, "cannot request a token for %q", sa.Name)
	}
	return tr.Status.Token, nil
}

var errNoNamespace = errors.ConstError("no namespace")

// NewBackend returns a k8s backed secrets backend.
func (p k8sProvider) NewBackend(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
	broker, err := p.getBroker(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "getting cluster client")
	}
	return &k8sBackend{
		model:          cfg.ModelName,
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
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &validForSeconds,
		},
	}
	tr, err := broker.client.CoreV1().ServiceAccounts(broker.namespace).CreateToken(
		context.TODO(), broker.serviceAccount, treq, v1.CreateOptions{})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot request a token for %q", broker.serviceAccount)
	}

	cfgCopy := adminCfg.BackendConfig
	cfgCopy.Config[tokenKey] = tr.Status.Token
	return &cfgCopy, nil
}
