// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"context"
	"net"
	"os"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubernetes2 "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/secrets/provider/kubernetes"
	"github.com/juju/juju/secrets/provider/kubernetes/mocks"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.IsolationSuite

	k8sClient               *mocks.MockInterface
	mockDiscovery           *mocks.MockDiscoveryInterface
	mockSecrets             *mocks.MockSecretInterface
	mockRbacV1              *mocks.MockRbacV1Interface
	mockNamespaces          *mocks.MockNamespaceInterface
	mockServiceAccounts     *mocks.MockServiceAccountInterface
	mockRoles               *mocks.MockRoleInterface
	mockClusterRoles        *mocks.MockClusterRoleInterface
	mockRoleBindings        *mocks.MockRoleBindingInterface
	mockClusterRoleBindings *mocks.MockClusterRoleBindingInterface

	namespace string
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.namespace = "test"
	s.PatchValue(&kubernetes.NewK8sClient, func(config *rest.Config) (kubernetes2.Interface, error) {
		return s.k8sClient, nil
	})
}

func (s *providerSuite) setupController(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.k8sClient = mocks.NewMockInterface(ctrl)

	s.mockDiscovery = mocks.NewMockDiscoveryInterface(ctrl)
	s.k8sClient.EXPECT().Discovery().AnyTimes().Return(s.mockDiscovery)

	mockCoreV1 := mocks.NewMockCoreV1Interface(ctrl)
	s.k8sClient.EXPECT().CoreV1().AnyTimes().Return(mockCoreV1)
	s.mockNamespaces = mocks.NewMockNamespaceInterface(ctrl)
	mockCoreV1.EXPECT().Namespaces().AnyTimes().Return(s.mockNamespaces)

	s.mockServiceAccounts = mocks.NewMockServiceAccountInterface(ctrl)
	mockCoreV1.EXPECT().ServiceAccounts(s.namespace).AnyTimes().Return(s.mockServiceAccounts)

	s.mockSecrets = mocks.NewMockSecretInterface(ctrl)
	mockCoreV1.EXPECT().Secrets(s.namespace).AnyTimes().Return(s.mockSecrets)

	s.mockRbacV1 = mocks.NewMockRbacV1Interface(ctrl)
	s.k8sClient.EXPECT().RbacV1().AnyTimes().Return(s.mockRbacV1)

	s.mockRoles = mocks.NewMockRoleInterface(ctrl)
	s.mockRbacV1.EXPECT().Roles(s.namespace).AnyTimes().Return(s.mockRoles)
	s.mockClusterRoles = mocks.NewMockClusterRoleInterface(ctrl)
	s.mockRbacV1.EXPECT().ClusterRoles().AnyTimes().Return(s.mockClusterRoles)
	s.mockRoleBindings = mocks.NewMockRoleBindingInterface(ctrl)
	s.mockRbacV1.EXPECT().RoleBindings(s.namespace).AnyTimes().Return(s.mockRoleBindings)
	s.mockClusterRoleBindings = mocks.NewMockClusterRoleBindingInterface(ctrl)
	s.mockRbacV1.EXPECT().ClusterRoleBindings().AnyTimes().Return(s.mockClusterRoleBindings)

	return ctrl
}

func (s *providerSuite) backendConfig() provider.BackendConfig {
	return provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]interface{}{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
		},
	}
}

func (s *providerSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(schema.GroupResource{}, "test")
}

func (s *providerSuite) expectEnsureSecretAccessToken(consumer, appNameLabel string, owned, read []string) {
	objMeta := v1.ObjectMeta{
		Name: consumer,
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "juju",
			"app.kubernetes.io/name":       appNameLabel,
			"model.juju.is/name":           "fred",
			"secrets.juju.is/model-name":   "fred",
			"secrets.juju.is/model-id":     coretesting.ModelTag.Id(),
		},
		Annotations: map[string]string{
			"model.juju.is/id":      coretesting.ModelTag.Id(),
			"controller.juju.is/id": coretesting.ControllerTag.Id(),
		},
		Namespace: s.namespace,
	}

	sa := &core.ServiceAccount{
		TypeMeta:                     v1.TypeMeta{},
		ObjectMeta:                   objMeta,
		AutomountServiceAccountToken: ptr(true),
	}
	role := &rbacv1.Role{
		ObjectMeta: objMeta,
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create", "patch"},
				APIGroups: []string{"*"},
				Resources: []string{"secrets"},
			},
			{
				Verbs:         []string{"get", "list"},
				APIGroups:     []string{"*"},
				Resources:     []string{"namespaces"},
				ResourceNames: []string{s.namespace},
			},
		},
	}
	for _, rName := range owned {
		role.Rules = append(role.Rules, rbacv1.PolicyRule{
			APIGroups:     []string{rbacv1.APIGroupAll},
			Resources:     []string{"secrets"},
			Verbs:         []string{rbacv1.VerbAll},
			ResourceNames: []string{rName},
		})
	}
	for _, rName := range read {
		role.Rules = append(role.Rules, rbacv1.PolicyRule{
			APIGroups:     []string{rbacv1.APIGroupAll},
			Resources:     []string{"secrets"},
			Verbs:         []string{"get"},
			ResourceNames: []string{rName},
		})
	}

	roleBinding := &rbacv1.RoleBinding{
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

	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: ptr(int64(600)),
		},
	}

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().List(gomock.Any(), v1.ListOptions{
			LabelSelector: "model.juju.is/name=fred",
		}).Return(&core.ServiceAccountList{}, nil),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), consumer, v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, v1.CreateOptions{FieldManager: "juju"}).Return(sa, nil),
		s.mockRoles.EXPECT().Get(gomock.Any(), consumer, v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
		s.mockRoles.EXPECT().Create(gomock.Any(), role, v1.CreateOptions{FieldManager: "juju"}).Return(role, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), consumer, v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), roleBinding, v1.CreateOptions{FieldManager: "juju"}).Return(roleBinding, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), consumer, v1.GetOptions{}).Return(roleBinding, nil),
		s.mockServiceAccounts.EXPECT().CreateToken(gomock.Any(), consumer, treq, v1.CreateOptions{FieldManager: "juju"}).
			Return(&authenticationv1.TokenRequest{
				Status: authenticationv1.TokenRequestStatus{Token: "token"},
			}, nil),
	)
}

func (s *providerSuite) expectEnsureControllerModelSecretAccessToken(unit string, owned, read []string, roleAlreadyExists bool) {
	objMeta := v1.ObjectMeta{
		Name: unit + "-06f00d",
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "juju",
			"app.kubernetes.io/name":       "gitlab",
			"model.juju.is/name":           "controller",
			"secrets.juju.is/model-name":   "controller",
			"secrets.juju.is/model-id":     coretesting.ModelTag.Id(),
		},
		Annotations: map[string]string{
			"model.juju.is/id":      coretesting.ModelTag.Id(),
			"controller.juju.is/id": coretesting.ControllerTag.Id(),
		},
		Namespace: s.namespace,
	}
	automountServiceAccountToken := true
	sa := &core.ServiceAccount{
		ObjectMeta:                   objMeta,
		AutomountServiceAccountToken: &automountServiceAccountToken,
	}

	name := "juju-secrets-" + unit + "-06f00d"
	objMeta.Name = name
	objMeta.Namespace = ""
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: objMeta,
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create", "patch"},
				APIGroups: []string{"*"},
				Resources: []string{"secrets"},
			},
			{
				Verbs:     []string{"get", "list"},
				APIGroups: []string{"*"},
				Resources: []string{"namespaces"},
			},
		},
	}
	for _, rName := range owned {
		clusterRole.Rules = append(clusterRole.Rules, rbacv1.PolicyRule{
			APIGroups:     []string{rbacv1.APIGroupAll},
			Resources:     []string{"secrets"},
			Verbs:         []string{rbacv1.VerbAll},
			ResourceNames: []string{rName},
		})
	}
	for _, rName := range read {
		clusterRole.Rules = append(clusterRole.Rules, rbacv1.PolicyRule{
			APIGroups:     []string{rbacv1.APIGroupAll},
			Resources:     []string{"secrets"},
			Verbs:         []string{"get"},
			ResourceNames: []string{rName},
		})
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
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
	expiresInSeconds := int64(60 * 10)
	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expiresInSeconds,
		},
	}
	args := []any{
		s.mockNamespaces.EXPECT().Get(gomock.Any(), s.namespace, v1.GetOptions{}).Return(&core.Namespace{
			ObjectMeta: v1.ObjectMeta{Name: s.namespace},
		}, nil),
		s.mockServiceAccounts.EXPECT().List(gomock.Any(), v1.ListOptions{
			LabelSelector: "model.juju.is/name=controller",
		}).Return(&core.ServiceAccountList{}, nil),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), sa.Name, v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, v1.CreateOptions{FieldManager: "juju"}).
			Return(sa, nil),
	}
	if roleAlreadyExists {
		args = append(args,
			s.mockClusterRoles.EXPECT().List(gomock.Any(), v1.ListOptions{
				LabelSelector: "model.juju.is/name=controller",
			}).Return(&rbacv1.ClusterRoleList{Items: []rbacv1.ClusterRole{*clusterRole}}, nil),
			s.mockClusterRoles.EXPECT().Update(gomock.Any(), clusterRole, v1.UpdateOptions{}).Return(clusterRole, nil),
		)
	} else {
		args = append(args,
			s.mockClusterRoles.EXPECT().List(gomock.Any(), v1.ListOptions{
				LabelSelector: "model.juju.is/name=controller",
			}).Return(&rbacv1.ClusterRoleList{}, nil),
			s.mockClusterRoles.EXPECT().Get(gomock.Any(), name, v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
			s.mockClusterRoles.EXPECT().Create(gomock.Any(), clusterRole, v1.CreateOptions{FieldManager: "juju"}).Return(clusterRole, nil),
		)
	}
	args = append(args,
		s.mockClusterRoleBindings.EXPECT().List(gomock.Any(), v1.ListOptions{
			LabelSelector: "model.juju.is/name=controller",
		}).Return(&rbacv1.ClusterRoleBindingList{}, nil),
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), name, v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Create(gomock.Any(), clusterRoleBinding, v1.CreateOptions{FieldManager: "juju"}).Return(clusterRoleBinding, nil),
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), name, v1.GetOptions{}).Return(clusterRoleBinding, nil),
		s.mockServiceAccounts.EXPECT().CreateToken(gomock.Any(), sa.Name, treq, v1.CreateOptions{FieldManager: "juju"}).Return(
			&authenticationv1.TokenRequest{Status: authenticationv1.TokenRequestStatus{Token: "token"}}, nil,
		),
	)
	gomock.InOrder(args...)
}

func (s *providerSuite) assertRestrictedConfigWithTag(c *gc.C, tag names.Tag, isControllerCloud, sameController bool) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	appNameLabel := "gitlab"
	consumer := tag.String() + "-06f00d"
	if tag.Kind() == names.ModelTagKind {
		consumer = "model-fred-06f00d"
		appNameLabel = coretesting.ModelTag.Id()
	}
	s.expectEnsureSecretAccessToken(consumer, appNameLabel, []string{"owned-rev-1"}, []string{"read-rev-1", "read-rev-2"})

	s.PatchValue(&kubernetes.InClusterConfig, func() (*rest.Config, error) {
		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if len(host) == 0 || len(port) == 0 {
			return nil, rest.ErrNotInCluster
		}

		tlsClientConfig := rest.TLSClientConfig{
			CAFile: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		}

		return &rest.Config{
			Host:            "https://" + net.JoinHostPort(host, port),
			TLSClientConfig: tlsClientConfig,
			BearerToken:     "token",
			BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
		}, nil
	})
	s.PatchEnvironment("KUBERNETES_SERVICE_HOST", "8.6.8.6")
	s.PatchEnvironment("KUBERNETES_SERVICE_PORT", "8888")

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	cfg := s.backendConfig()
	if isControllerCloud {
		cfg.Config["prefer-incluster-address"] = true
	}
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  cfg,
	}

	backendCfg, err := p.RestrictedConfig(adminCfg, sameController, false, tag,
		provider.SecretRevisions{"owned-a": set.NewStrings("owned-rev-1")},
		provider.SecretRevisions{"read-b": set.NewStrings("read-rev-1", "read-rev-2")},
	)
	c.Assert(err, jc.ErrorIsNil)
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]interface{}{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     "token",
		},
	}
	if isControllerCloud && sameController {
		expected.Config["endpoint"] = "https://8.6.8.6:8888"
	}
	c.Assert(backendCfg, jc.DeepEquals, expected)
}

func (s *providerSuite) TestRestrictedConfigWithUnitTag(c *gc.C) {
	s.assertRestrictedConfigWithTag(c, names.NewUnitTag("gitlab/0"), false, false)
}

func (s *providerSuite) TestRestrictedConfigWithModelTag(c *gc.C) {
	s.assertRestrictedConfigWithTag(c, coretesting.ModelTag, false, false)
}

func (s *providerSuite) TestRestrictedConfigWithTagWithControllerCloud(c *gc.C) {
	s.assertRestrictedConfigWithTag(c, names.NewUnitTag("gitlab/0"), true, true)
}

func (s *providerSuite) TestRestrictedConfigWithTagWithControllerCloudDifferentController(c *gc.C) {
	s.assertRestrictedConfigWithTag(c, names.NewUnitTag("gitlab/0"), true, false)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *providerSuite) TestCleanupModel(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	selector := "model.juju.is/name=fred"
	s.mockServiceAccounts.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: selector,
	}).Return(&core.ServiceAccountList{}, nil)
	s.mockRoles.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: selector,
	}).Return(&rbacv1.RoleList{}, nil)
	s.mockRoleBindings.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: selector,
	}).Return(&rbacv1.RoleBindingList{}, nil)
	s.mockClusterRoles.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: selector,
	}).Return(&rbacv1.ClusterRoleList{Items: []rbacv1.ClusterRole{{
		ObjectMeta: v1.ObjectMeta{Name: "juju-secrets-role", Annotations: map[string]string{
			"model.juju.is/id": coretesting.ModelTag.Id(),
		}},
	}, {
		ObjectMeta: v1.ObjectMeta{Name: "other-role"},
	}}}, nil)
	s.mockClusterRoles.EXPECT().Delete(gomock.Any(), "juju-secrets-role", v1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	s.mockClusterRoleBindings.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: selector,
	}).Return(&rbacv1.ClusterRoleBindingList{Items: []rbacv1.ClusterRoleBinding{{
		ObjectMeta: v1.ObjectMeta{Name: "juju-secrets-rolebinding", Annotations: map[string]string{
			"model.juju.is/id": coretesting.ModelTag.Id(),
		}},
	}, {
		ObjectMeta: v1.ObjectMeta{Name: "other-rolebinding"},
	}}}, nil)
	s.mockClusterRoleBindings.EXPECT().Delete(gomock.Any(), "juju-secrets-rolebinding", v1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	s.mockSecrets.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: selector,
	}).Return(&core.SecretList{Items: []core.Secret{{
		ObjectMeta: v1.ObjectMeta{Name: "some-secret", Annotations: map[string]string{
			"model.juju.is/id": coretesting.ModelTag.Id(),
		}},
	}}}, nil)
	s.mockSecrets.EXPECT().Delete(gomock.Any(), "some-secret", v1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

	err = p.CleanupModel(adminCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestCleanupSecrets(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	tag := names.NewUnitTag("gitlab/0")
	consumer := tag.String() + "-06f00d"
	s.expectEnsureSecretAccessToken(consumer, "gitlab", nil, nil)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

	err = p.CleanupSecrets(adminCfg, tag, provider.SecretRevisions{"removed": set.NewStrings("rev-1", "rev-2")})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestNewBackend(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockDiscovery.EXPECT().ServerVersion().Return(nil, errors.New("boom"))

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = b.Ping()
	c.Assert(err, gc.ErrorMatches, "backend not reachable: boom")
}

func (s *providerSuite) TestEnsureSecretAccessTokenControllerModelCreate(c *gc.C) {
	s.namespace = "juju-secrets"
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.expectEnsureControllerModelSecretAccessToken(
		"unit-gitlab-0", []string{"owned-rev-1"}, []string{"read-rev-1", "read-rev-2"}, false)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "controller",
		BackendConfig:  s.backendConfig(),
	}

	tag := names.NewUnitTag("gitlab/0")
	backendCfg, err := p.RestrictedConfig(adminCfg, false, false, tag,
		provider.SecretRevisions{"owned-a": set.NewStrings("owned-rev-1")},
		provider.SecretRevisions{"read-b": set.NewStrings("read-rev-1", "read-rev-2")},
	)
	c.Assert(err, jc.ErrorIsNil)
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]interface{}{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     "token",
		},
	}
	c.Assert(backendCfg, jc.DeepEquals, expected)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestEnsureSecretAccessTokenUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	tag := names.NewUnitTag("gitlab/0")
	name := tag.String() + "-06f00d"
	objMeta := v1.ObjectMeta{
		Name: name,
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "juju",
			"app.kubernetes.io/name":       "gitlab",
			"model.juju.is/name":           "fred",
			"secrets.juju.is/model-name":   "fred",
			"secrets.juju.is/model-id":     coretesting.ModelTag.Id(),
		},
		Annotations: map[string]string{
			"model.juju.is/id":      coretesting.ModelTag.Id(),
			"controller.juju.is/id": coretesting.ControllerTag.Id(),
		},
		Namespace: s.namespace,
	}
	automountServiceAccountToken := true
	sa := &core.ServiceAccount{
		ObjectMeta:                   objMeta,
		AutomountServiceAccountToken: &automountServiceAccountToken,
	}
	role := &rbacv1.Role{
		ObjectMeta: objMeta,
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create", "patch"},
				APIGroups: []string{"*"},
				Resources: []string{"secrets"},
			},
			{
				Verbs:         []string{"get", "list"},
				APIGroups:     []string{"*"},
				Resources:     []string{"namespaces"},
				ResourceNames: []string{"test"},
			},
		},
	}
	roleBinding := &rbacv1.RoleBinding{
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
	expiresInSeconds := int64(60 * 10)
	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expiresInSeconds,
		},
	}
	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().List(gomock.Any(), v1.ListOptions{
			LabelSelector: "model.juju.is/name=fred",
		}).Return(&core.ServiceAccountList{}, nil),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), name, v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, v1.CreateOptions{FieldManager: "juju"}).
			Return(sa, nil),
		s.mockRoles.EXPECT().Get(gomock.Any(), name, v1.GetOptions{}).Return(role, nil),
		s.mockRoles.EXPECT().Update(gomock.Any(), role, v1.UpdateOptions{}).Return(role, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), name, v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), roleBinding, v1.CreateOptions{FieldManager: "juju"}).Return(roleBinding, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), name, v1.GetOptions{}).Return(roleBinding, nil),
		s.mockServiceAccounts.EXPECT().CreateToken(gomock.Any(), name, treq, v1.CreateOptions{FieldManager: "juju"}).Return(
			&authenticationv1.TokenRequest{Status: authenticationv1.TokenRequestStatus{Token: "token"}}, nil,
		),
	)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

	backendCfg, err := p.RestrictedConfig(adminCfg, false, false, tag,
		provider.SecretRevisions{"owned-a": set.NewStrings("owned-rev-1")},
		provider.SecretRevisions{"read-b": set.NewStrings("read-rev-1", "read-rev-2")},
	)
	c.Assert(err, jc.ErrorIsNil)
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]interface{}{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     "token",
		},
	}
	c.Assert(backendCfg, jc.DeepEquals, expected)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestEnsureSecretAccessTokeControllerModelUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.expectEnsureControllerModelSecretAccessToken(
		"unit-gitlab-0", []string{"owned-rev-1"}, []string{"read-rev-1", "read-rev-2"}, true)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "controller",
		BackendConfig:  s.backendConfig(),
	}

	tag := names.NewUnitTag("gitlab/0")
	backendCfg, err := p.RestrictedConfig(adminCfg, false, false, tag,
		provider.SecretRevisions{"owned-a": set.NewStrings("owned-rev-1")},
		provider.SecretRevisions{"read-b": set.NewStrings("read-rev-1", "read-rev-2")},
	)
	c.Assert(err, jc.ErrorIsNil)
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]interface{}{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     "token",
		},
	}
	c.Assert(backendCfg, jc.DeepEquals, expected)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestGetContent(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	uri := secrets.NewURI()
	secret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      uri.ID + "-1",
			Namespace: s.namespace,
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"foo": []byte("bar"),
		},
	}
	s.mockSecrets.EXPECT().Get(gomock.Any(), uri.ID+"-1", v1.GetOptions{}).Return(secret, nil)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, jc.ErrorIsNil)

	content, err := b.GetContent(context.Background(), uri.ID+"-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(content.EncodedValues(), jc.DeepEquals, map[string]string{"foo": "YmFy"})
}

func (s *providerSuite) TestSaveContent(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	uri := secrets.NewURI()
	secret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name: uri.ID + "-1",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "juju",
				"model.juju.is/name":           "fred",
				"secrets.juju.is/model-name":   "fred",
				"secrets.juju.is/model-id":     coretesting.ModelTag.Id(),
			},
			Namespace: s.namespace,
		},
		Type: core.SecretTypeOpaque,
		StringData: map[string]string{
			"foo": "bar",
		},
	}
	s.mockSecrets.EXPECT().Create(gomock.Any(), secret, v1.CreateOptions{FieldManager: "juju"}).Return(secret, nil)
	s.mockSecrets.EXPECT().Patch(
		gomock.Any(), uri.ID+"-1", types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: "juju"}).
		Return(nil, s.k8sNotFoundError())

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, jc.ErrorIsNil)

	name, err := b.SaveContent(context.Background(), uri, 1, secrets.NewSecretValue(map[string]string{"foo": "YmFy"}))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, uri.ID+"-1")
}

func (s *providerSuite) TestDeleteContent(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	uri := secrets.NewURI()
	secret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      uri.ID + "-1",
			Namespace: s.namespace,
		},
	}
	s.mockSecrets.EXPECT().Get(gomock.Any(), uri.ID+"-1", v1.GetOptions{}).Return(secret, nil)
	s.mockSecrets.EXPECT().Delete(gomock.Any(), uri.ID+"-1", v1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy()})

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = b.DeleteContent(context.Background(), uri.ID+"-1")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestRefreshAuth(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: ptr(int64(3600)),
		},
	}
	s.mockServiceAccounts.EXPECT().CreateToken(gomock.Any(), "default", treq, v1.CreateOptions{FieldManager: "juju"}).
		Return(&authenticationv1.TokenRequest{
			Status: authenticationv1.TokenRequestStatus{Token: "token2"},
		}, nil)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	r, ok := p.(provider.SupportAuthRefresh)
	c.Assert(ok, jc.IsTrue)

	cfg := s.backendConfig()
	cfg.Config["service-account"] = "default"

	validFor := time.Hour
	newCfg, err := r.RefreshAuth(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  cfg,
	}, validFor)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newCfg.Config["token"], gc.Equals, "token2")
}
