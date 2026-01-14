// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
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
	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	k8s "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8srest "k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/secrets/provider/kubernetes"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.CleanupSuite

	k8sClient *k8sfake.Clientset

	namespace string
	tokens    []string
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&kubernetes.NewK8sClient, func(config *k8srest.Config) (k8s.Interface, error) {
		return s.k8sClient, nil
	})
}

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

func (s *providerSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(k8sschema.GroupResource{}, "test")
}

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
}

func (s *providerSuite) expectEnsureControllerModelSecretAccessToken(unit string, owned, read []string, roleAlreadyExists bool) {

}

func (s *providerSuite) assertRestrictedConfigWithTag(c *gc.C, tag names.Tag, isControllerCloud, sameController bool) {
	defer s.setupK8s(c)()
	ctx := context.Background()

	appNameLabel := "gitlab"
	consumer := tag.String()
	if tag.Kind() == names.ModelTagKind {
		consumer = coretesting.ModelTag.String()
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
				"secrets.juju.is/consumer-tag": consumer,
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
				"secrets.juju.is/consumer-tag": consumer,
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
	defer s.setupK8s(c)()

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
	defer s.setupK8s(c)()

	tag := names.NewUnitTag("gitlab/0")
	consumer := tag.String() + "-06f00d"

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

	s.checkEnsureSecretAccessToken(c, consumer, "gitlab", nil, nil)
}

func (s *providerSuite) TestNewBackend(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = b.Ping()
	c.Assert(err, gc.ErrorMatches,
		`backend not reachable: checking secrets namespace: `+
			`namespaces "missing-namespace" not found`,
	)
}

func (s *providerSuite) TestEnsureSecretAccessTokenControllerModelCreate(c *gc.C) {
	s.namespace = "juju-secrets"
	defer s.setupK8s(c)()

	ownedURI := secrets.NewURI()
	readURI := secrets.NewURI()

	s.expectEnsureControllerModelSecretAccessToken(
		"unit-gitlab-0", []string{ownedURI.Name(1)},
		[]string{readURI.Name(1), readURI.Name(2)}, false)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "controller",
		BackendConfig:  s.backendConfig(),
	}

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
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]any{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     s.tokens[0],
		},
	}
	c.Assert(backendCfg, jc.DeepEquals, expected)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestEnsureSecretAccessTokenUpdate(c *gc.C) {
	defer s.setupK8s(c)()

	tag := names.NewUnitTag("gitlab/0")
	ownedURI := secrets.NewURI()
	readURI := secrets.NewURI()

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	}

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
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]any{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     s.tokens[0],
		},
	}
	c.Assert(backendCfg, jc.DeepEquals, expected)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestEnsureSecretAccessTokeControllerModelUpdate(c *gc.C) {
	defer s.setupK8s(c)()

	ownedURI := secrets.NewURI()
	readURI := secrets.NewURI()

	s.expectEnsureControllerModelSecretAccessToken(
		"unit-gitlab-0", []string{ownedURI.Name(1)},
		[]string{readURI.Name(1), readURI.Name(2)}, true,
	)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "controller",
		BackendConfig:  s.backendConfig(),
	}

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
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]any{
			"ca-certs":  []string{"cert-data"},
			"endpoint":  "http://nowhere",
			"namespace": s.namespace,
			"token":     s.tokens[0],
		},
	}
	c.Assert(backendCfg, jc.DeepEquals, expected)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestGetContent(c *gc.C) {
	defer s.setupK8s(c)()
	ctx := context.Background()

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
	c.Assert(err, jc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, jc.ErrorIsNil)

	content, err := b.GetContent(context.Background(), uri.Name(1))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(content.EncodedValues(), jc.DeepEquals, map[string]string{"foo": "YmFy"})
}

func (s *providerSuite) TestSaveContent(c *gc.C) {
	ctx := context.Background()
	defer s.setupK8s(c)()

	uri := secrets.NewURI()

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, jc.ErrorIsNil)

	sv := secrets.NewSecretValue(map[string]string{"foo": "YmFy"})
	name, err := b.SaveContent(ctx, uri, 1, sv)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, uri.Name(1))

	res, err := s.k8sClient.CoreV1().Secrets(s.namespace).List(
		ctx, metav1.ListOptions{})
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
	c.Assert(err, jc.ErrorIsNil)
	b, err := p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig:  s.backendConfig(),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = b.DeleteContent(context.Background(), uri.Name(1))
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.k8sClient.CoreV1().Secrets(s.namespace).List(
		ctx, metav1.ListOptions{})
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
	c.Assert(s.tokens, gc.HasLen, 1)
	c.Assert(newCfg.Config["token"], gc.Equals, s.tokens[0])
}
