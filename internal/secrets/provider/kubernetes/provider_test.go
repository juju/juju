// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"context"
	"crypto/rand"
	"strconv"
	"testing"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8s "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	coretesting "github.com/juju/juju/internal/testing"
)

type providerSuite struct {
	k8sClient *k8sfake.Clientset

	namespace string
	tokens    []string
}

func TestProviderSuite(t *testing.T) {
	tc.Run(t, &providerSuite{})
}

func (s *providerSuite) setupK8s(c *tc.C) func() {
	ctx := c.Context()
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
	c.Assert(err, tc.ErrorIsNil)
	oldNewK8sClient := kubernetes.NewK8sClient
	c.Cleanup(func() {
		kubernetes.NewK8sClient = oldNewK8sClient
	})
	kubernetes.NewK8sClient = func(config *rest.Config) (k8s.Interface, error) {
		return s.k8sClient, nil
	}
	return func() {
		s.k8sClient = nil
		s.namespace = ""
		s.tokens = nil
	}
}

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

func (s *providerSuite) checkEnsureSecretAccessToken(c *tc.C, consumer, appNameLabel string, owned, read []string) {
	ctx := c.Context()
	roles, err := s.k8sClient.RbacV1().Roles(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(roles.Items, tc.HasLen, 0)
	roleBindings, err := s.k8sClient.RbacV1().RoleBindings(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(roleBindings.Items, tc.HasLen, 0)
}

func (s *providerSuite) expectEnsureControllerModelSecretAccessToken(unit string, owned, read []string, roleAlreadyExists bool) {

}

func (s *providerSuite) assertRestrictedConfigWithTag(c *tc.C, tag secrets.Accessor, isControllerCloud, sameController bool) {
	defer s.setupK8s(c)()
	ctx := c.Context()

	appNameLabel := "gitlab"
	consumer := tag.String()
	if tag.Kind == secrets.ModelAccessor {
		consumer = coretesting.ModelTag.String()
		appNameLabel = coretesting.ModelTag.Id()
	}
	ownedURI := secrets.NewURI()
	readURI := secrets.NewURI()

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
	issuedTokenUUID := "some-uuid"
	backendCfg, err := p.RestrictedConfig(
		c.Context(),
		adminCfg, sameController, false,
		issuedTokenUUID, tag,
		[]string{ownedURI.ID},
		provider.SecretRevisions{ownedURI.ID: set.NewStrings(ownedURI.Name(1))},
		provider.SecretRevisions{readURI.ID: set.NewStrings(readURI.Name(1), readURI.Name(2))},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.tokens, tc.HasLen, 1)
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
	c.Assert(backendCfg, tc.DeepEquals, expected)

	roles, err := s.k8sClient.RbacV1().Roles(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].ObjectMeta.Annotations["secrets.juju.is/expire-at"]`, tc.Satisfies, func(s string) bool {
		i, err := strconv.Atoi(s)
		if !c.Check(err, tc.ErrorIsNil) {
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
	c.Assert(err, tc.ErrorIsNil)
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
}

func (s *providerSuite) TestCleanupModel(c *tc.C) {
	defer s.setupK8s(c)()

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

func (s *providerSuite) TestCleanupSecrets(c *tc.C) {
	defer s.setupK8s(c)()

	tag := secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	consumer := tag.String() + "-06f00d"

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

	err = p.CleanupSecrets(c.Context(), adminCfg, tag, provider.SecretRevisions{"removed": set.NewStrings("rev-1", "rev-2")})
	c.Assert(err, tc.ErrorIsNil)

	s.checkEnsureSecretAccessToken(c, consumer, "gitlab", nil, nil)
}

func (s *providerSuite) TestCleanupSecretsOnlyUpdatesAffectedRoles(c *tc.C) {
	defer s.setupK8s(c)()
	ctx := c.Context()

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
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)

	// Clear recorded actions from setup, this is required to make sure that no
	// call was made to patch the unaffected roles.
	s.k8sClient.ClearActions()

	tag := secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

	err = p.CleanupSecrets(c.Context(), adminCfg, tag, provider.SecretRevisions{
		"secret-1": set.NewStrings("rev-1", "rev-2"),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Check that only one role had a call to patch it.
	for _, action := range s.k8sClient.Actions() {
		if !action.Matches("patch", "roles") {
			continue
		}
		patched := action.(k8stesting.PatchAction)
		c.Check(patched.GetName(), tc.Equals, "affected-role")
	}

	// Check that the role now has the right resource names.
	res, err := s.k8sClient.RbacV1().Roles(s.namespace).Get(
		ctx, "affected-role", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Rules, tc.HasLen, 1)
	c.Check(res.Rules[0].ResourceNames, tc.DeepEquals, []string{"rev-keep"})

	// Verify unaffected role is unchanged.
	unaffectedRole, err := s.k8sClient.RbacV1().Roles(s.namespace).Get(
		ctx, "unaffected-role", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unaffectedRole.Rules, tc.HasLen, 1)
	c.Check(unaffectedRole.Rules[0].ResourceNames, tc.DeepEquals, []string{
		"rev-3", "rev-4",
	})
}

func (s *providerSuite) TestCleanupSecretsOnlyUpdatesAffectedClusterRoles(c *tc.C) {
	defer s.setupK8s(c)()
	ctx := c.Context()

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
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)

	// Clear recorded actions from setup, this is required to make sure that no
	// call was made to patch the unaffected roles.
	s.k8sClient.ClearActions()

	tag := secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

	err = p.CleanupSecrets(c.Context(), adminCfg, tag, provider.SecretRevisions{
		"secret-1": set.NewStrings("rev-1", "rev-2"),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Check that only one role had a call to patch it.
	for _, action := range s.k8sClient.Actions() {
		if !action.Matches("patch", "clusterroles") {
			continue
		}
		patched := action.(k8stesting.PatchAction)
		c.Check(patched.GetName(), tc.Equals, "affected-cluster-role")
	}

	// Check that the role now has the right resource names.
	res, err := s.k8sClient.RbacV1().ClusterRoles().Get(
		ctx, "affected-cluster-role", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Rules, tc.HasLen, 1)
	c.Check(res.Rules[0].ResourceNames, tc.DeepEquals, []string{"rev-keep"})

	// Check the other role is unchanged.
	other, err := s.k8sClient.RbacV1().ClusterRoles().Get(
		ctx, "unaffected-cluster-role", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(other.Rules, tc.HasLen, 1)
	c.Check(other.Rules[0].ResourceNames, tc.DeepEquals, []string{
		"rev-3",
		"rev-4",
	})
}

func (s *providerSuite) TestNewBackend(c *tc.C) {
	defer s.setupK8s(c)()

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
	c.Assert(err, tc.ErrorMatches,
		`backend not reachable: checking secrets namespace: `+
			`namespaces "missing-namespace" not found`,
	)
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

	tag := secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	issuedTokenUUID := "some-uuid"
	backendCfg, err := p.RestrictedConfig(
		c.Context(),
		adminCfg, false, false,
		issuedTokenUUID, tag,
		[]string{ownedURI.ID},
		provider.SecretRevisions{ownedURI.ID: set.NewStrings(ownedURI.Name(1))},
		provider.SecretRevisions{readURI.ID: set.NewStrings(readURI.Name(1), readURI.Name(2))},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.tokens, tc.HasLen, 1)
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

func (s *providerSuite) TestEnsureSecretAccessTokenUpdate(c *tc.C) {
	defer s.setupK8s(c)()

	tag := secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	ownedURI := secrets.NewURI()
	readURI := secrets.NewURI()

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

	issuedTokenUUID := "some-uuid"
	backendCfg, err := p.RestrictedConfig(
		c.Context(),
		adminCfg, false, false,
		issuedTokenUUID, tag,
		[]string{ownedURI.ID},
		provider.SecretRevisions{ownedURI.ID: set.NewStrings(ownedURI.Name(1))},
		provider.SecretRevisions{readURI.ID: set.NewStrings(readURI.Name(1), readURI.Name(2))},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.tokens, tc.HasLen, 1)
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

func (s *providerSuite) TestEnsureSecretAccessTokeControllerModelUpdate(c *tc.C) {
	defer s.setupK8s(c)()

	ownedURI := secrets.NewURI()
	readURI := secrets.NewURI()

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

	tag := secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	issuedTokenUUID := "some-uuid"
	backendCfg, err := p.RestrictedConfig(
		c.Context(),
		adminCfg, false, false,
		issuedTokenUUID, tag,
		[]string{ownedURI.ID},
		provider.SecretRevisions{ownedURI.ID: set.NewStrings(ownedURI.Name(1))},
		provider.SecretRevisions{readURI.ID: set.NewStrings(readURI.Name(1), readURI.Name(2))},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.tokens, tc.HasLen, 1)
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

func (s *providerSuite) TestGetContent(c *tc.C) {
	defer s.setupK8s(c)()
	ctx := c.Context()

	uri := secrets.NewURI()

	_, err := s.k8sClient.CoreV1().Secrets(s.namespace).Create(ctx, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: uri.Name(1),
		},
		Data: map[string][]byte{
			"foo": []byte("bar"),
		},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, tc.ErrorIsNil)

	content, err := b.GetContent(context.Background(), uri.Name(1))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(content.EncodedValues(), tc.DeepEquals, map[string]string{"foo": "YmFy"})
}

func (s *providerSuite) TestSaveContent(c *tc.C) {
	ctx := c.Context()
	defer s.setupK8s(c)()

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

	sv := secrets.NewSecretValue(map[string]string{"foo": "YmFy"})
	name, err := b.SaveContent(ctx, uri, 1, sv)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(name, tc.Equals, uri.Name(1))

	res, err := s.k8sClient.CoreV1().Secrets(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Items, tc.HasLen, 1)
	secret := res.Items[0]
	c.Check(secret.Name, tc.Equals, uri.Name(1))
	c.Check(secret.Labels, tc.DeepEquals, map[string]string{
		"app.kubernetes.io/managed-by": "juju",
		"secrets.juju.is/model-id":     coretesting.ModelTag.Id(),
		"model.juju.is/name":           "fred",
		"secrets.juju.is/model-name":   "fred",
	})
	c.Check(secret.StringData, tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *providerSuite) TestDeleteContent(c *tc.C) {
	ctx := c.Context()
	defer s.setupK8s(c)()

	uri := secrets.NewURI()

	_, err := s.k8sClient.CoreV1().Secrets(s.namespace).Create(ctx,
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: uri.Name(1),
			},
		},
		metav1.CreateOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = b.DeleteContent(context.Background(), uri.Name(1))
	c.Assert(err, tc.ErrorIsNil)

	res, err := s.k8sClient.CoreV1().Secrets(s.namespace).List(
		ctx, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Items, tc.HasLen, 0)
}

func (s *providerSuite) TestRefreshAuth(c *tc.C) {
	defer s.setupK8s(c)()
	ctx := c.Context()

	_, err := s.k8sClient.CoreV1().ServiceAccounts(s.namespace).Create(ctx,
		&v1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
		}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, tc.ErrorIsNil)
	r, ok := p.(provider.SupportAuthRefresh)
	c.Assert(ok, tc.IsTrue)

	cfg := s.backendConfig()
	cfg.Config["service-account"] = "default"

	validFor := time.Hour
	newCfg, err := r.RefreshAuth(c.Context(), cfg, validFor)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.tokens, tc.HasLen, 1)
	c.Assert(newCfg.Config["token"], tc.Equals, s.tokens[0])
}
