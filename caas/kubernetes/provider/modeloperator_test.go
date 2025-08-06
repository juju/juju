// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/testing"
)

type ModelOperatorSuite struct {
	client *fake.Clientset
	clock  *testclock.Clock
}

var _ = gc.Suite(&ModelOperatorSuite{})

func (m *ModelOperatorSuite) SetUpTest(c *gc.C) {
	m.client = fake.NewSimpleClientset()
	m.clock = testclock.NewClock(time.Time{})
}

func (m *ModelOperatorSuite) assertEnsure(c *gc.C, isPrivateImageRepo bool) {
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
		ImageDetails: resources.DockerImageDetails{RegistryPath: "juju/juju:123"},
		Port:         int32(5497),
	}
	if isPrivateImageRepo {
		config.ImageDetails.BasicAuthConfig.Auth = docker.NewToken("xxxxxxxx===")
	}
	bridge := &modelOperatorBrokerBridge{
		coreClient: m.client,
		ensureConfigMap: func(cm *core.ConfigMap) ([]func(), error) {
			ensureConfigMapCalled = true
			_, err := m.client.CoreV1().ConfigMaps(namespace).Create(context.TODO(), cm, meta.CreateOptions{})
			return nil, err
		},
		ensureDeployment: func(d *apps.Deployment) ([]func(), error) {
			ensureDeploymentCalled = true
			_, err := m.client.AppsV1().Deployments(namespace).Create(context.TODO(), d, meta.CreateOptions{})
			return nil, err
		},
		ensureRole: func(r *rbac.Role) ([]func(), error) {
			ensureRoleCalled = true
			_, err := m.client.RbacV1().Roles(namespace).Create(context.TODO(), r, meta.CreateOptions{})
			return nil, err
		},
		ensureRoleBinding: func(rb *rbac.RoleBinding) ([]func(), error) {
			ensureRoleBindingCalled = true
			_, err := m.client.RbacV1().RoleBindings(namespace).Create(context.TODO(), rb, meta.CreateOptions{})
			return nil, err
		},
		ensureServiceAccount: func(sa *core.ServiceAccount) ([]func(), error) {
			ensureServiceAccountCalled = true
			_, err := m.client.CoreV1().ServiceAccounts(namespace).Create(context.TODO(), sa, meta.CreateOptions{})
			return nil, err
		},
		ensureService: func(s *core.Service) ([]func(), error) {
			ensureServiceCalled = true
			_, err := m.client.CoreV1().Services(namespace).Create(context.TODO(), s, meta.CreateOptions{})
			return nil, err
		},
		modelName: model,
		namespace: namespace,
	}

	// fake k8sclient does not populate the token for secret, so we have to do it manually.
	_, err := m.client.CoreV1().Secrets(namespace).Create(context.TODO(), &core.Secret{
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
	c.Assert(err, jc.ErrorIsNil)

	errChan := make(chan error)
	go func() {
		errChan <- ensureModelOperator(modelUUID, agentPath, m.clock, &config, bridge)
	}()

	select {
	case err := <-errChan:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for ensureModelOperator return")
	}

	cm, err := m.client.CoreV1().ConfigMaps(namespace).Get(context.TODO(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cm.Name, gc.Equals, modelOperatorName)
	c.Assert(cm.Namespace, gc.Equals, namespace)
	conf, ok := cm.Data[modelOperatorConfigMapAgentConfKey(modelOperatorName)]
	c.Assert(ok, gc.Equals, true)
	c.Assert(conf, jc.DeepEquals, string(config.AgentConf))

	d, err := m.client.AppsV1().Deployments(namespace).Get(context.TODO(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d.Name, gc.Equals, modelOperatorName)
	c.Assert(d.Namespace, gc.Equals, namespace)
	c.Assert(d.Spec.Template.Spec.Containers[0].Image, gc.Equals, config.ImageDetails.RegistryPath)
	c.Assert(d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort, gc.Equals, config.Port)
	if isPrivateImageRepo {
		c.Assert(len(d.Spec.Template.Spec.ImagePullSecrets), gc.Equals, 1)
		c.Assert(d.Spec.Template.Spec.ImagePullSecrets[0].Name, gc.Equals, constants.CAASImageRepoSecretName)
	}

	r, err := m.client.RbacV1().Roles(namespace).Get(context.TODO(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name, gc.Equals, modelOperatorName)
	c.Assert(r.Namespace, gc.Equals, namespace)
	c.Assert(r.Rules[0].APIGroups, jc.DeepEquals, []string{""})
	c.Assert(r.Rules[0].Resources, jc.DeepEquals, []string{"serviceaccounts"})
	c.Assert(r.Rules[0].Verbs, jc.DeepEquals, []string{
		"get",
		"list",
		"watch",
	})

	rb, err := m.client.RbacV1().RoleBindings(namespace).Get(context.TODO(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rb.Name, gc.Equals, modelOperatorName)
	c.Assert(rb.Namespace, gc.Equals, namespace)
	c.Assert(rb.RoleRef.APIGroup, gc.Equals, "rbac.authorization.k8s.io")
	c.Assert(rb.RoleRef.Kind, gc.Equals, "Role")
	c.Assert(rb.RoleRef.Name, gc.Equals, modelOperatorName)

	sa, err := m.client.CoreV1().ServiceAccounts(namespace).Get(context.TODO(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	trueVar := true
	c.Assert(sa.Name, gc.Equals, modelOperatorName)
	c.Assert(sa.Namespace, gc.Equals, namespace)
	c.Assert(sa.AutomountServiceAccountToken, jc.DeepEquals, &trueVar)

	s, err := m.client.CoreV1().Services(namespace).Get(context.TODO(), modelOperatorName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.Name, gc.Equals, modelOperatorName)
	c.Assert(s.Namespace, gc.Equals, namespace)
	c.Assert(s.Spec.Ports[0].Port, gc.Equals, config.Port)

	clusterRole, err := m.client.RbacV1().ClusterRoles().Get(
		context.TODO(),
		"test-model-modeloperator",
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clusterRole.Name, gc.Equals, "test-model-modeloperator")
	c.Assert(clusterRole.Rules[0].APIGroups, jc.DeepEquals, []string{""})
	c.Assert(clusterRole.Rules[0].Resources, jc.DeepEquals, []string{"namespaces"})
	c.Assert(clusterRole.Rules[0].Verbs, jc.DeepEquals, []string{"get", "list"})
	c.Assert(clusterRole.Rules[1].APIGroups, jc.DeepEquals, []string{"admissionregistration.k8s.io"})
	c.Assert(clusterRole.Rules[1].Resources, jc.DeepEquals, []string{"mutatingwebhookconfigurations"})
	c.Assert(clusterRole.Rules[1].Verbs, jc.DeepEquals, []string{
		"create",
		"delete",
		"get",
		"list",
		"update",
	})

	clusterRoleBinding, err := m.client.RbacV1().ClusterRoleBindings().Get(
		context.TODO(),
		"test-model-modeloperator",
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clusterRoleBinding.Name, gc.Equals, "test-model-modeloperator")
	c.Assert(clusterRoleBinding.RoleRef.APIGroup, gc.Equals, "rbac.authorization.k8s.io")
	c.Assert(clusterRoleBinding.RoleRef.Kind, gc.Equals, "ClusterRole")
	c.Assert(clusterRoleBinding.RoleRef.Name, gc.Equals, "test-model-modeloperator")

	// The exec service account.
	sa, err = m.client.CoreV1().ServiceAccounts(namespace).Get(context.TODO(), ExecRBACResourceName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	trueVar = true
	c.Assert(sa.Name, gc.Equals, ExecRBACResourceName)
	c.Assert(sa.Namespace, gc.Equals, namespace)
	c.Assert(sa.AutomountServiceAccountToken, jc.DeepEquals, &trueVar)
	c.Assert(sa.Secrets, jc.DeepEquals, []core.ObjectReference{
		{
			Name:      ExecRBACResourceName,
			Namespace: namespace,
		},
	})

	// The exec role.
	r, err = m.client.RbacV1().Roles(namespace).Get(context.TODO(), ExecRBACResourceName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name, gc.Equals, ExecRBACResourceName)
	c.Assert(r.Namespace, gc.Equals, namespace)
	c.Assert(r.Rules, jc.DeepEquals, []rbac.PolicyRule{
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
	rb, err = m.client.RbacV1().RoleBindings(namespace).Get(context.TODO(), ExecRBACResourceName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rb.Name, gc.Equals, ExecRBACResourceName)
	c.Assert(rb.Namespace, gc.Equals, namespace)
	c.Assert(rb.RoleRef.APIGroup, gc.Equals, "rbac.authorization.k8s.io")
	c.Assert(rb.RoleRef.Kind, gc.Equals, "Role")
	c.Assert(rb.RoleRef.Name, gc.Equals, ExecRBACResourceName)

	// The exec secret.
	secret, err := m.client.CoreV1().Secrets(namespace).Get(context.TODO(), ExecRBACResourceName, meta.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secret.Name, gc.Equals, ExecRBACResourceName)
	c.Assert(secret.Type, gc.Equals, core.SecretTypeServiceAccountToken)

	c.Assert(ensureConfigMapCalled, jc.IsTrue)
	c.Assert(ensureDeploymentCalled, jc.IsTrue)
	c.Assert(ensureRoleCalled, jc.IsTrue)
	c.Assert(ensureRoleBindingCalled, jc.IsTrue)
	c.Assert(ensureServiceAccountCalled, jc.IsTrue)
	c.Assert(ensureServiceCalled, jc.IsTrue)
}

func (m *ModelOperatorSuite) TestDefaultImageRepo(c *gc.C) {
	m.assertEnsure(c, false)
}

func (m *ModelOperatorSuite) TestPrivateImageRepo(c *gc.C) {
	m.assertEnsure(c, true)
}
