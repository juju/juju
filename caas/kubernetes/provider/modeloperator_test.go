// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/internal/testing"
)

type ModelOperatorSuite struct {
	client *fake.Clientset
	clock  *testclock.Clock
}

var _ = tc.Suite(&ModelOperatorSuite{})

func (m *ModelOperatorSuite) SetUpTest(c *tc.C) {
	m.client = fake.NewSimpleClientset()
	m.clock = testclock.NewClock(time.Time{})
}

func (m *ModelOperatorSuite) assertEnsure(c *tc.C, isPrivateImageRepo bool) {
	var (
		ensureConfigMapCalled      = false
		ensureDeploymentCalled     = false
		ensureRoleCalled           = false
		ensureRoleBindingCalled    = false
		ensureServiceCalled        = false
		ensureServiceAccountCalled = false
		modelUUID                  = "abcd-efff-face"
		agentPath                  = "/var/app/juju"
		model                      = "test-model"
		namespace                  = "test-namespace"
	)

	config := caas.ModelOperatorConfig{
		AgentConf:    []byte("testconf"),
		ImageDetails: resource.DockerImageDetails{RegistryPath: "juju/juju:123"},
		Port:         int32(5497),
	}
	if isPrivateImageRepo {
		config.ImageDetails.BasicAuthConfig.Auth = resource.NewToken("xxxxxxxx===")
	}
	bridge := &modelOperatorBrokerBridge{
		client: m.client,
		ensureConfigMap: func(ctx context.Context, cm *core.ConfigMap) ([]func(), error) {
			ensureConfigMapCalled = true
			_, err := m.client.CoreV1().ConfigMaps(namespace).Create(context.Background(), cm, meta.CreateOptions{})
			return nil, err
		},
		ensureDeployment: func(ctx context.Context, d *apps.Deployment) ([]func(), error) {
			ensureDeploymentCalled = true
			_, err := m.client.AppsV1().Deployments(namespace).Create(context.Background(), d, meta.CreateOptions{})
			return nil, err
		},
		ensureRole: func(ctx context.Context, r *rbac.Role) ([]func(), error) {
			ensureRoleCalled = true
			_, err := m.client.RbacV1().Roles(namespace).Create(context.Background(), r, meta.CreateOptions{})
			return nil, err
		},
		ensureRoleBinding: func(ctx context.Context, rb *rbac.RoleBinding) ([]func(), error) {
			ensureRoleBindingCalled = true
			_, err := m.client.RbacV1().RoleBindings(namespace).Create(context.Background(), rb, meta.CreateOptions{})
			return nil, err
		},
		ensureServiceAccount: func(ctx context.Context, sa *core.ServiceAccount) ([]func(), error) {
			ensureServiceAccountCalled = true
			_, err := m.client.CoreV1().ServiceAccounts(namespace).Create(context.Background(), sa, meta.CreateOptions{})
			return nil, err
		},
		ensureService: func(ctx context.Context, s *core.Service) ([]func(), error) {
			ensureServiceCalled = true
			_, err := m.client.CoreV1().Services(namespace).Create(context.Background(), s, meta.CreateOptions{})
			return nil, err
		},
		modelName: model,
		namespace: namespace,
	}

	// fake k8sclient does not populate the token for secret, so we have to do it manually.
	_, err := m.client.CoreV1().Secrets(namespace).Create(context.Background(), &core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name: ExecRBACResourceName,
			Annotations: map[string]string{
				core.ServiceAccountNameKey: ExecRBACResourceName,
			},
			Labels: map[string]string{"juju-modeloperator": "modeloperator"},
		},
		Type: core.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			core.ServiceAccountTokenKey: []byte("token"),
		},
	}, meta.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	errChan := make(chan error)
	go func() {
		errChan <- ensureModelOperator(context.Background(), modelUUID, agentPath, m.clock, &config, bridge)
	}()

	select {
	case err := <-errChan:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for ensureModelOperator return")
	}

	cm, err := m.client.CoreV1().ConfigMaps(namespace).Get(context.Background(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cm.Name, tc.Equals, modelOperatorName)
	c.Assert(cm.Namespace, tc.Equals, namespace)
	conf, ok := cm.Data[modelOperatorConfigMapAgentConfKey(modelOperatorName)]
	c.Assert(ok, tc.Equals, true)
	c.Assert(conf, tc.DeepEquals, string(config.AgentConf))

	d, err := m.client.AppsV1().Deployments(namespace).Get(context.Background(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(d.Name, tc.Equals, modelOperatorName)
	c.Assert(d.Namespace, tc.Equals, namespace)
	c.Assert(d.Spec.Template.Spec.Containers[0].Image, tc.Equals, config.ImageDetails.RegistryPath)
	c.Assert(d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort, tc.Equals, config.Port)
	if isPrivateImageRepo {
		c.Assert(len(d.Spec.Template.Spec.ImagePullSecrets), tc.Equals, 1)
		c.Assert(d.Spec.Template.Spec.ImagePullSecrets[0].Name, tc.Equals, constants.CAASImageRepoSecretName)
	}

	r, err := m.client.RbacV1().Roles(namespace).Get(context.Background(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Name, tc.Equals, modelOperatorName)
	c.Assert(r.Namespace, tc.Equals, namespace)
	c.Assert(r.Rules[0].APIGroups, tc.DeepEquals, []string{""})
	c.Assert(r.Rules[0].Resources, tc.DeepEquals, []string{"serviceaccounts"})
	c.Assert(r.Rules[0].Verbs, tc.DeepEquals, []string{
		"get",
		"list",
		"watch",
	})

	rb, err := m.client.RbacV1().RoleBindings(namespace).Get(context.Background(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rb.Name, tc.Equals, modelOperatorName)
	c.Assert(rb.Namespace, tc.Equals, namespace)
	c.Assert(rb.RoleRef.APIGroup, tc.Equals, "rbac.authorization.k8s.io")
	c.Assert(rb.RoleRef.Kind, tc.Equals, "Role")
	c.Assert(rb.RoleRef.Name, tc.Equals, modelOperatorName)

	sa, err := m.client.CoreV1().ServiceAccounts(namespace).Get(context.Background(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	trueVar := true
	c.Assert(sa.Name, tc.Equals, modelOperatorName)
	c.Assert(sa.Namespace, tc.Equals, namespace)
	c.Assert(sa.AutomountServiceAccountToken, tc.DeepEquals, &trueVar)

	s, err := m.client.CoreV1().Services(namespace).Get(context.Background(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.Name, tc.Equals, modelOperatorName)
	c.Assert(s.Namespace, tc.Equals, namespace)
	c.Assert(s.Spec.Ports[0].Port, tc.Equals, config.Port)

	clusterRole, err := m.client.RbacV1().ClusterRoles().Get(
		context.Background(),
		"test-model-modeloperator",
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clusterRole.Name, tc.Equals, "test-model-modeloperator")
	c.Assert(clusterRole.Rules[0].APIGroups, tc.DeepEquals, []string{""})
	c.Assert(clusterRole.Rules[0].Resources, tc.DeepEquals, []string{"namespaces"})
	c.Assert(clusterRole.Rules[0].Verbs, tc.DeepEquals, []string{"get", "list"})
	c.Assert(clusterRole.Rules[1].APIGroups, tc.DeepEquals, []string{"admissionregistration.k8s.io"})
	c.Assert(clusterRole.Rules[1].Resources, tc.DeepEquals, []string{"mutatingwebhookconfigurations"})
	c.Assert(clusterRole.Rules[1].Verbs, tc.DeepEquals, []string{
		"create",
		"delete",
		"get",
		"list",
		"update",
	})

	clusterRoleBinding, err := m.client.RbacV1().ClusterRoleBindings().Get(
		context.Background(),
		"test-model-modeloperator",
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clusterRoleBinding.Name, tc.Equals, "test-model-modeloperator")
	c.Assert(clusterRoleBinding.RoleRef.APIGroup, tc.Equals, "rbac.authorization.k8s.io")
	c.Assert(clusterRoleBinding.RoleRef.Kind, tc.Equals, "ClusterRole")
	c.Assert(clusterRoleBinding.RoleRef.Name, tc.Equals, "test-model-modeloperator")

	// The exec service account.
	sa, err = m.client.CoreV1().ServiceAccounts(namespace).Get(context.Background(), ExecRBACResourceName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	trueVar = true
	c.Assert(sa.Name, tc.Equals, ExecRBACResourceName)
	c.Assert(sa.Namespace, tc.Equals, namespace)
	c.Assert(sa.AutomountServiceAccountToken, tc.DeepEquals, &trueVar)
	c.Assert(sa.Secrets, tc.DeepEquals, []core.ObjectReference{
		{
			Name:      ExecRBACResourceName,
			Namespace: namespace,
		},
	})

	// The exec role.
	r, err = m.client.RbacV1().Roles(namespace).Get(context.Background(), ExecRBACResourceName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Name, tc.Equals, ExecRBACResourceName)
	c.Assert(r.Namespace, tc.Equals, namespace)
	c.Assert(r.Rules, tc.DeepEquals, []rbac.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces"},
			Verbs: []string{
				"get",
				"list",
			},
			ResourceNames: []string{namespace},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs: []string{
				"get",
				"list",
			},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods/exec"},
			Verbs: []string{
				"create",
			},
		},
	})

	// The exec rolebinding.
	rb, err = m.client.RbacV1().RoleBindings(namespace).Get(context.Background(), ExecRBACResourceName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rb.Name, tc.Equals, ExecRBACResourceName)
	c.Assert(rb.Namespace, tc.Equals, namespace)
	c.Assert(rb.RoleRef.APIGroup, tc.Equals, "rbac.authorization.k8s.io")
	c.Assert(rb.RoleRef.Kind, tc.Equals, "Role")
	c.Assert(rb.RoleRef.Name, tc.Equals, ExecRBACResourceName)

	// The exec secret.
	secret, err := m.client.CoreV1().Secrets(namespace).Get(context.Background(), ExecRBACResourceName, meta.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(secret.Name, tc.Equals, ExecRBACResourceName)
	c.Assert(secret.Type, tc.Equals, core.SecretTypeServiceAccountToken)

	c.Assert(ensureConfigMapCalled, tc.IsTrue)
	c.Assert(ensureDeploymentCalled, tc.IsTrue)
	c.Assert(ensureRoleCalled, tc.IsTrue)
	c.Assert(ensureRoleBindingCalled, tc.IsTrue)
	c.Assert(ensureServiceAccountCalled, tc.IsTrue)
	c.Assert(ensureServiceCalled, tc.IsTrue)
}

func (m *ModelOperatorSuite) TestDefaultImageRepo(c *tc.C) {
	m.assertEnsure(c, false)
}

func (m *ModelOperatorSuite) TestPrivateImageRepo(c *tc.C) {
	m.assertEnsure(c, true)
}
