// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
=======
	"context"
	"crypto/rand"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8s "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8srest "k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/juju/juju/core/secrets"
<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/kubernetes/mocks"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type providerSuite struct {
	testhelpers.IsolationSuite
=======
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/secrets/provider/kubernetes"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.CleanupSuite
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

	k8sClient *k8sfake.Clientset

	namespace string
	tokens    []string
}

func TestProviderSuite(t *testing.T) {
	tc.Run(t, &providerSuite{})
}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
func (s *providerSuite) SetUpTest(c *tc.C) {
	s.namespace = "test"
	s.PatchValue(&kubernetes.NewK8sClient, func(config *rest.Config) (kubernetes2.Interface, error) {
=======
func (s *providerSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&kubernetes.NewK8sClient, func(config *k8srest.Config) (k8s.Interface, error) {
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
		return s.k8sClient, nil
	})
}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
func (s *providerSuite) setupController(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
=======
func (s *providerSuite) setupK8s(c *gc.C) func() {
	ctx := context.Background()
	s.k8sClient = k8sfake.NewSimpleClientset()
	if s.namespace == "" {
		s.namespace = "test"
	}
	s.k8sClient.PrependReactor("create", "serviceaccounts", s.tokenReactor)
	_, err := s.k8sClient.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.namespace,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)
	return func() {
		s.k8sClient = nil
		s.namespace = ""
		s.tokens = nil
	}
}
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

// tokenReactor creates service account tokens to test against
func (s *providerSuite) tokenReactor(
	action k8stesting.Action,
) (handled bool, ret k8sruntime.Object, err error) {
	if action.GetSubresource() != "token" {
		return
	}
	createAction, ok := action.(k8stesting.CreateActionImpl)
	if !ok {
		return
	}
	if createAction.Object == nil {
		return
	}
	req, ok := createAction.Object.(*authenticationv1.TokenRequest)
	if !ok {
		return
	}
	_, err = s.k8sClient.Tracker().Get(
		createAction.Resource, createAction.Namespace, createAction.Name)
	if err != nil {
		return false, nil, err
	}
	res := *req
	res.Status.Token = rand.Text()
	s.tokens = append(s.tokens, res.Status.Token)
	return true, &res, nil
}

func (s *providerSuite) backendConfig() provider.BackendConfig {
	return provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]any{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
		},
	}
}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
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
		AutomountServiceAccountToken: new(true),
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
			ExpirationSeconds: new(int64(600)),
		},
	}

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().List(gomock.Any(), v1.ListOptions{
			LabelSelector: "model.juju.is/name=fred",
		}).Return(&core.ServiceAccountList{}, nil),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), consumer, v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, v1.CreateOptions{FieldManager: "juju"}).Return(sa, nil),
		s.mockRoles.EXPECT().Create(gomock.Any(), role, v1.CreateOptions{FieldManager: "juju"}).Return(role, nil),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), roleBinding, v1.CreateOptions{FieldManager: "juju"}).Return(roleBinding, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), consumer, v1.GetOptions{}).Return(roleBinding, nil),
		s.mockServiceAccounts.EXPECT().CreateToken(gomock.Any(), consumer, treq, v1.CreateOptions{FieldManager: "juju"}).
			Return(&authenticationv1.TokenRequest{
				Status: authenticationv1.TokenRequestStatus{Token: "token"},
			}, nil).AnyTimes(),
	)
=======
func (s *providerSuite) checkEnsureSecretAccessToken(c *gc.C, consumer, appNameLabel string, owned, read []string) {
	ctx := context.Background()
	roles, err := s.k8sClient.RbacV1().Roles(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(roles.Items, gc.HasLen, 0)
	roleBindings, err := s.k8sClient.RbacV1().RoleBindings(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(roleBindings.Items, gc.HasLen, 0)
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
}

func (s *providerSuite) expectEnsureControllerModelSecretAccessToken(unit string, owned, read []string, roleAlreadyExists bool) {

}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
func (s *providerSuite) assertRestrictedConfig(c *tc.C, accessor secrets.Accessor, isControllerCloud, sameController bool) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	appNameLabel := "gitlab"
	consumer := "unit-" + strings.ReplaceAll(accessor.ID, "/", "-") + "-06f00d"
	if accessor.Kind == secrets.ModelAccessor {
		consumer = "model-fred-06f00d"
=======
func (s *providerSuite) assertRestrictedConfigWithTag(c *gc.C, tag names.Tag, isControllerCloud, sameController bool) {
	defer s.setupK8s(c)()
	ctx := context.Background()

	appNameLabel := "gitlab"
	consumer := tag.String()
	if tag.Kind() == names.ModelTagKind {
		consumer = coretesting.ModelTag.String()
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
		appNameLabel = coretesting.ModelTag.Id()
	}
	ownedURI := secrets.NewURI()
	readURI := secrets.NewURI()

	s.PatchValue(&kubernetes.InClusterConfig, func() (*k8srest.Config, error) {
		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if len(host) == 0 || len(port) == 0 {
			return nil, k8srest.ErrNotInCluster
		}

		tlsClientConfig := k8srest.TLSClientConfig{
			CAFile: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		}

		return &k8srest.Config{
			Host:            "https://" + net.JoinHostPort(host, port),
			TLSClientConfig: tlsClientConfig,
			BearerToken:     "token",
			BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
		}, nil
	})
	s.PatchEnvironment("KUBERNETES_SERVICE_HOST", "8.6.8.6")
	s.PatchEnvironment("KUBERNETES_SERVICE_PORT", "8888")

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
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
<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go

	backendCfg, err := p.RestrictedConfig(c.Context(), adminCfg, sameController, false, accessor,
		provider.SecretRevisions{"owned-a": set.NewStrings("owned-rev-1")},
		provider.SecretRevisions{"read-b": set.NewStrings("read-rev-1", "read-rev-2")},
	)
	c.Assert(err, tc.ErrorIsNil)
=======
	issuedTokenUUID := "some-uuid"
	backendCfg, err := p.RestrictedConfig(
		adminCfg, sameController, false,
		issuedTokenUUID, tag,
		[]string{ownedURI.ID},
		provider.SecretRevisions{ownedURI.ID: set.NewStrings(ownedURI.Name(1))},
		provider.SecretRevisions{readURI.ID: set.NewStrings(readURI.Name(1), readURI.Name(2))},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.tokens, gc.HasLen, 1)
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]any{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     s.tokens[0],
		},
	}
	if isControllerCloud && sameController {
		expected.Config["endpoint"] = "https://8.6.8.6:8888"
	}
<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	c.Assert(backendCfg, tc.DeepEquals, expected)
=======
	c.Assert(backendCfg, jc.DeepEquals, expected)

	roles, err := s.k8sClient.RbacV1().Roles(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)

	mc := jc.NewMultiChecker()
	mc.AddExpr(`_[_].ObjectMeta.Annotations["secrets.juju.is/expire-at"]`, jc.Satisfies, func(s string) bool {
		i, err := strconv.Atoi(s)
		if !c.Check(err, jc.ErrorIsNil) {
			return false
		}
		return i > int(time.Now().Unix())
	})
	c.Check(roles.Items, mc, []rbacv1.Role{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-secret-consumer-" + issuedTokenUUID,
			Namespace: s.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "juju",
				"app.kubernetes.io/name":       appNameLabel,
				"model.juju.is/name":           "fred",
				"secrets.juju.is/consumer":     consumer,
				"secrets.juju.is/model-id":     coretesting.ModelTag.Id(),
				"secrets.juju.is/model-name":   "fred",
			},
			Annotations: map[string]string{
				"controller.juju.is/id":     coretesting.ControllerTag.Id(),
				"model.juju.is/id":          coretesting.ModelTag.Id(),
				"secrets.juju.is/expire-at": "",
			},
		},
		Rules: []rbacv1.PolicyRule{{
			Verbs:         []string{"get", "list"},
			APIGroups:     []string{"*"},
			Resources:     []string{"namespaces"},
			ResourceNames: []string{"test"},
		}, {
			Verbs:         []string{"get", "patch", "update", "replace", "delete"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{ownedURI.Name(1), ownedURI.Name(2)},
		}, {
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{readURI.Name(1), readURI.Name(2)},
		}},
	}})

	roleBindings, err := s.k8sClient.RbacV1().RoleBindings(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(roleBindings.Items, mc, []rbacv1.RoleBinding{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-secret-consumer-" + issuedTokenUUID,
			Namespace: s.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "juju",
				"app.kubernetes.io/name":       appNameLabel,
				"model.juju.is/name":           "fred",
				"secrets.juju.is/consumer":     consumer,
				"secrets.juju.is/model-id":     coretesting.ModelTag.Id(),
				"secrets.juju.is/model-name":   "fred",
			},
			Annotations: map[string]string{
				"controller.juju.is/id":     coretesting.ControllerTag.Id(),
				"model.juju.is/id":          coretesting.ModelTag.Id(),
				"secrets.juju.is/expire-at": "",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Name:     "juju-secret-consumer-" + issuedTokenUUID,
			Kind:     "Role",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      "juju-secret-consumer-" + issuedTokenUUID,
			Namespace: s.namespace,
		}},
	}})
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
}

func (s *providerSuite) TestRestrictedConfigWithUnit(c *tc.C) {
	s.assertRestrictedConfig(c, secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}, false, false)
}

func (s *providerSuite) TestRestrictedConfigWithModel(c *tc.C) {
	s.assertRestrictedConfig(c, secrets.Accessor{
		Kind: secrets.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}, false, false)
}

func (s *providerSuite) TestRestrictedConfiWithControllerCloud(c *tc.C) {
	s.assertRestrictedConfig(c, secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}, true, true)
}

func (s *providerSuite) TestRestrictedConfigWithControllerCloudDifferentController(c *tc.C) {
	s.assertRestrictedConfig(c, secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}, true, false)
}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
func (s *providerSuite) TestCleanupModel(c *tc.C) {
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
=======
func (s *providerSuite) TestCleanupModel(c *gc.C) {
	defer s.setupK8s(c)()
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

	err = p.CleanupModel(c.Context(), adminCfg)
	c.Assert(err, tc.ErrorIsNil)
}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
func (s *providerSuite) TestCleanupSecrets(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	consumer := "unit-gitlab-0-06f00d"
	s.expectEnsureSecretAccessToken(consumer, "gitlab", nil, nil)
=======
func (s *providerSuite) TestCleanupSecrets(c *gc.C) {
	defer s.setupK8s(c)()

	tag := names.NewUnitTag("gitlab/0")
	consumer := tag.String() + "-06f00d"
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	err = p.CleanupSecrets(c.Context(), adminCfg,
		secrets.Accessor{Kind: secrets.UnitAccessor, ID: "gitlab/0"},
		provider.SecretRevisions{"removed": set.NewStrings("rev-1", "rev-2")})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerSuite) TestNewBackend(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
=======
	err = p.CleanupSecrets(adminCfg, tag, provider.SecretRevisions{"removed": set.NewStrings("rev-1", "rev-2")})
	c.Assert(err, jc.ErrorIsNil)

	s.checkEnsureSecretAccessToken(c, consumer, "gitlab", nil, nil)
}

func (s *providerSuite) TestCleanupSecretsOnlyUpdatesAffectedRoles(c *gc.C) {
	defer s.setupK8s(c)()
	ctx := context.Background()

	matchingLabels := map[string]string{
		"app.kubernetes.io/managed-by": "juju",
		"model.juju.is/name":           "fred",
		"secrets.juju.is/model-name":   "fred",
		"secrets.juju.is/model-id":     coretesting.ModelTag.Id(),
	}

	// Create a role that references revisions to be removed (and one to keep).
	_, err := s.k8sClient.RbacV1().Roles(s.namespace).Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "affected-role",
			Namespace: s.namespace,
			Labels:    matchingLabels,
		},
		Rules: []rbacv1.PolicyRule{{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"rev-1", "rev-2", "rev-keep"},
		}},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create a role that does NOT reference any revisions to be removed.
	_, err = s.k8sClient.RbacV1().Roles(s.namespace).Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unaffected-role",
			Namespace: s.namespace,
			Labels:    matchingLabels,
		},
		Rules: []rbacv1.PolicyRule{{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"rev-3", "rev-4"},
		}},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Clear recorded actions from setup, this is required to make sure that no
	// call was made to patch the unaffected roles.
	s.k8sClient.ClearActions()

	tag := names.NewUnitTag("gitlab/0")
	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

	err = p.CleanupSecrets(adminCfg, tag, provider.SecretRevisions{
		"secret-1": set.NewStrings("rev-1", "rev-2"),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that only one role had a call to patch it.
	for _, action := range s.k8sClient.Actions() {
		if !action.Matches("patch", "roles") {
			continue
		}
		patched := action.(k8stesting.PatchAction)
		c.Check(patched.GetName(), gc.Equals, "affected-role")
	}

	// Check that the role now has the right resource names.
	res, err := s.k8sClient.RbacV1().Roles(s.namespace).Get(
		ctx, "affected-role", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Rules, gc.HasLen, 1)
	c.Check(res.Rules[0].ResourceNames, jc.DeepEquals, []string{"rev-keep"})

	// Verify unaffected role is unchanged.
	unaffectedRole, err := s.k8sClient.RbacV1().Roles(s.namespace).Get(
		ctx, "unaffected-role", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unaffectedRole.Rules, gc.HasLen, 1)
	c.Check(unaffectedRole.Rules[0].ResourceNames, jc.DeepEquals, []string{
		"rev-3", "rev-4",
	})
}

func (s *providerSuite) TestCleanupSecretsOnlyUpdatesAffectedClusterRoles(c *gc.C) {
	defer s.setupK8s(c)()
	ctx := context.Background()

	matchingLabels := map[string]string{
		"app.kubernetes.io/managed-by": "juju",
		"model.juju.is/name":           "fred",
		"secrets.juju.is/model-name":   "fred",
		"secrets.juju.is/model-id":     coretesting.ModelTag.Id(),
	}

	// Create a cluster role that references the revisions to be removed.
	_, err := s.k8sClient.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "affected-cluster-role",
			Labels: matchingLabels,
		},
		Rules: []rbacv1.PolicyRule{{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"rev-1", "rev-2", "rev-keep"},
		}},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create a cluster role that does NOT reference any revisions to be removed.
	_, err = s.k8sClient.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "unaffected-cluster-role",
			Labels: matchingLabels,
		},
		Rules: []rbacv1.PolicyRule{{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"rev-3", "rev-4"},
		}},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Clear recorded actions from setup, this is required to make sure that no
	// call was made to patch the unaffected roles.
	s.k8sClient.ClearActions()

	tag := names.NewUnitTag("gitlab/0")
	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

	err = p.CleanupSecrets(adminCfg, tag, provider.SecretRevisions{
		"secret-1": set.NewStrings("rev-1", "rev-2"),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that only one role had a call to patch it.
	for _, action := range s.k8sClient.Actions() {
		if !action.Matches("patch", "clusterroles") {
			continue
		}
		patched := action.(k8stesting.PatchAction)
		c.Check(patched.GetName(), gc.Equals, "affected-cluster-role")
	}

	// Check that the role now has the right resource names.
	res, err := s.k8sClient.RbacV1().ClusterRoles().Get(
		ctx, "affected-cluster-role", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Rules, gc.HasLen, 1)
	c.Check(res.Rules[0].ResourceNames, jc.DeepEquals, []string{"rev-keep"})

	// Check the other role is unchanged.
	other, err := s.k8sClient.RbacV1().ClusterRoles().Get(
		ctx, "unaffected-cluster-role", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(other.Rules, gc.HasLen, 1)
	c.Check(other.Rules[0].ResourceNames, jc.DeepEquals, []string{
		"rev-3",
		"rev-4",
	})
}

func (s *providerSuite) TestNewBackend(c *gc.C) {
	defer s.setupK8s(c)()
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

	cfg := provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]any{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": "missing-namespace",
		},
	}

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  cfg,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = b.Ping()
<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	c.Assert(err, tc.ErrorMatches, "backend not reachable: boom")
=======
	c.Assert(err, gc.ErrorMatches,
		`backend not reachable: checking secrets namespace: `+
			`namespaces "missing-namespace" not found`,
	)
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
}

func (s *providerSuite) TestEnsureSecretAccessTokenControllerModelCreate(c *tc.C) {
	s.namespace = "juju-secrets"
	defer s.setupK8s(c)()

	ownedURI := secrets.NewURI()
	readURI := secrets.NewURI()

	s.expectEnsureControllerModelSecretAccessToken(
		"unit-gitlab-0", []string{ownedURI.Name(1)},
		[]string{readURI.Name(1), readURI.Name(2)}, false)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "controller",
		BackendConfig:  s.backendConfig(),
	}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	backendCfg, err := p.RestrictedConfig(c.Context(), adminCfg, false, false, secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	},
		provider.SecretRevisions{"owned-a": set.NewStrings("owned-rev-1")},
		provider.SecretRevisions{"read-b": set.NewStrings("read-rev-1", "read-rev-2")},
	)
	c.Assert(err, tc.ErrorIsNil)
=======
	tag := names.NewUnitTag("gitlab/0")
	issuedTokenUUID := "some-uuid"
	backendCfg, err := p.RestrictedConfig(
		adminCfg, false, false,
		issuedTokenUUID, tag,
		[]string{ownedURI.ID},
		provider.SecretRevisions{ownedURI.ID: set.NewStrings(ownedURI.Name(1))},
		provider.SecretRevisions{readURI.ID: set.NewStrings(readURI.Name(1), readURI.Name(2))},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.tokens, gc.HasLen, 1)
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]any{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     s.tokens[0],
		},
	}
	c.Assert(backendCfg, tc.DeepEquals, expected)
	c.Assert(err, tc.ErrorIsNil)
}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
func (s *providerSuite) TestEnsureSecretAccessTokenUpdate(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	name := "unit-gitlab-0-06f00d"
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
		s.mockRoles.EXPECT().Create(gomock.Any(), gomock.Any(), v1.CreateOptions{FieldManager: "juju"}).Return(nil, errors.AlreadyExists),
		s.mockRoles.EXPECT().Get(gomock.Any(), name, v1.GetOptions{}).Return(role, nil),
		s.mockRoles.EXPECT().Patch(gomock.Any(), role.Name, types.StrategicMergePatchType, gomock.Any(), v1.PatchOptions{FieldManager: "juju"}).Return(role, nil),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), roleBinding, v1.CreateOptions{FieldManager: "juju"}).Return(roleBinding, nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), roleBinding.Name, v1.GetOptions{}).Return(roleBinding, nil),
		s.mockServiceAccounts.EXPECT().CreateToken(gomock.Any(), name, treq, v1.CreateOptions{FieldManager: "juju"}).Return(
			&authenticationv1.TokenRequest{Status: authenticationv1.TokenRequestStatus{Token: "token"}}, nil,
		),
	)
=======
func (s *providerSuite) TestEnsureSecretAccessTokenUpdate(c *gc.C) {
	defer s.setupK8s(c)()

	tag := names.NewUnitTag("gitlab/0")
	ownedURI := secrets.NewURI()
	readURI := secrets.NewURI()
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	backendCfg, err := p.RestrictedConfig(c.Context(), adminCfg, false, false, secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	},
		provider.SecretRevisions{"owned-a": set.NewStrings("owned-rev-1")},
		provider.SecretRevisions{"read-b": set.NewStrings("read-rev-1", "read-rev-2")},
	)
	c.Assert(err, tc.ErrorIsNil)
=======
	issuedTokenUUID := "some-uuid"
	backendCfg, err := p.RestrictedConfig(
		adminCfg, false, false,
		issuedTokenUUID, tag,
		[]string{ownedURI.ID},
		provider.SecretRevisions{ownedURI.ID: set.NewStrings(ownedURI.Name(1))},
		provider.SecretRevisions{readURI.ID: set.NewStrings(readURI.Name(1), readURI.Name(2))},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.tokens, gc.HasLen, 1)
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]any{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     s.tokens[0],
		},
	}
	c.Assert(backendCfg, tc.DeepEquals, expected)
	c.Assert(err, tc.ErrorIsNil)
}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
func (s *providerSuite) TestEnsureSecretAccessTokenControllerModelUpdate(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
=======
func (s *providerSuite) TestEnsureSecretAccessTokeControllerModelUpdate(c *gc.C) {
	defer s.setupK8s(c)()

	ownedURI := secrets.NewURI()
	readURI := secrets.NewURI()
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

	s.expectEnsureControllerModelSecretAccessToken(
		"unit-gitlab-0", []string{ownedURI.Name(1)},
		[]string{readURI.Name(1), readURI.Name(2)}, true,
	)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "controller",
		BackendConfig:  s.backendConfig(),
	}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	backendCfg, err := p.RestrictedConfig(c.Context(), adminCfg, false, false, secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	},
		provider.SecretRevisions{"owned-a": set.NewStrings("owned-rev-1")},
		provider.SecretRevisions{"read-b": set.NewStrings("read-rev-1", "read-rev-2")},
	)
	c.Assert(err, tc.ErrorIsNil)
=======
	tag := names.NewUnitTag("gitlab/0")
	issuedTokenUUID := "some-uuid"
	backendCfg, err := p.RestrictedConfig(
		adminCfg, false, false,
		issuedTokenUUID, tag,
		[]string{ownedURI.ID},
		provider.SecretRevisions{ownedURI.ID: set.NewStrings(ownedURI.Name(1))},
		provider.SecretRevisions{readURI.ID: set.NewStrings(readURI.Name(1), readURI.Name(2))},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.tokens, gc.HasLen, 1)
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]any{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     s.tokens[0],
		},
	}
	c.Assert(backendCfg, tc.DeepEquals, expected)
	c.Assert(err, tc.ErrorIsNil)
}

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
func (s *providerSuite) TestGetContent(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
=======
func (s *providerSuite) TestGetContent(c *gc.C) {
	defer s.setupK8s(c)()
	ctx := context.Background()
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

	uri := secrets.NewURI()

	_, err := s.k8sClient.CoreV1().Secrets(s.namespace).Create(ctx, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: uri.Name(1),
		},
		Data: map[string][]byte{
			"foo": []byte("bar"),
		},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, tc.ErrorIsNil)

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	content, err := b.GetContent(c.Context(), uri.ID+"-1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(content.EncodedValues(), tc.DeepEquals, map[string]string{"foo": "YmFy"})
}

func (s *providerSuite) TestSaveContent(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
=======
	content, err := b.GetContent(context.Background(), uri.Name(1))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(content.EncodedValues(), jc.DeepEquals, map[string]string{"foo": "YmFy"})
}

func (s *providerSuite) TestSaveContent(c *gc.C) {
	ctx := context.Background()
	defer s.setupK8s(c)()
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

	uri := secrets.NewURI()

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, tc.ErrorIsNil)

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	name, err := b.SaveContent(c.Context(), uri, 1, secrets.NewSecretValue(map[string]string{"foo": "YmFy"}))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(name, tc.Equals, uri.ID+"-1")
}

func (s *providerSuite) TestDeleteContent(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
=======
	sv := secrets.NewSecretValue(map[string]string{"foo": "YmFy"})
	name, err := b.SaveContent(ctx, uri, 1, sv)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, uri.Name(1))

	res, err := s.k8sClient.CoreV1().Secrets(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Items, gc.HasLen, 1)
	secret := res.Items[0]
	c.Check(secret.Name, gc.Equals, uri.Name(1))
	c.Check(secret.Labels, gc.DeepEquals, map[string]string{
		"app.kubernetes.io/managed-by": "juju",
		"secrets.juju.is/model-id":     coretesting.ModelTag.Id(),
		"model.juju.is/name":           "fred",
		"secrets.juju.is/model-name":   "fred",
	})
	c.Check(secret.StringData, gc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *providerSuite) TestDeleteContent(c *gc.C) {
	ctx := context.Background()
	defer s.setupK8s(c)()
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

	uri := secrets.NewURI()

	_, err := s.k8sClient.CoreV1().Secrets(s.namespace).Create(ctx,
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: uri.Name(1),
			},
		},
		metav1.CreateOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, tc.ErrorIsNil)

<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	err = b.DeleteContent(c.Context(), uri.ID+"-1")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerSuite) TestRefreshAuth(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: new(int64(3600)),
		},
	}
	s.mockServiceAccounts.EXPECT().CreateToken(gomock.Any(), "default", treq, v1.CreateOptions{FieldManager: "juju"}).
		Return(&authenticationv1.TokenRequest{
			Status: authenticationv1.TokenRequestStatus{Token: "token2"},
		}, nil)
=======
	err = b.DeleteContent(context.Background(), uri.Name(1))
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.k8sClient.CoreV1().Secrets(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Items, gc.HasLen, 0)
}

func (s *providerSuite) TestRefreshAuth(c *gc.C) {
	defer s.setupK8s(c)()
	ctx := context.Background()

	_, err := s.k8sClient.CoreV1().ServiceAccounts(s.namespace).Create(ctx,
		&v1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
		}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	r, ok := p.(provider.SupportAuthRefresh)
	c.Assert(ok, tc.IsTrue)

	cfg := s.backendConfig()
	cfg.Config["service-account"] = "default"

	validFor := time.Hour
<<<<<<< HEAD:internal/secrets/provider/kubernetes/provider_test.go
	newCfg, err := r.RefreshAuth(c.Context(), cfg, validFor)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newCfg.Config["token"], tc.Equals, "token2")
=======
	newCfg, err := r.RefreshAuth(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  cfg,
	}, validFor)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.tokens, gc.HasLen, 1)
	c.Assert(newCfg.Config["token"], gc.Equals, s.tokens[0])
>>>>>>> 3.6:secrets/provider/kubernetes/provider_test.go
}
