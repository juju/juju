// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"

	"github.com/juju/juju/caas"
)

type ModelOperatorSuite struct {
}

var _ = gc.Suite(&ModelOperatorSuite{})

func (m *ModelOperatorSuite) Test(c *gc.C) {
	var (
		ensureClusterRoleCalled        = false
		ensureClusterRoleBindingCalled = false
		ensureConfigMapCalled          = false
		ensureDeploymentCalled         = false
		ensureRoleCalled               = false
		ensureRoleBindingCalled        = false
		ensureServiceCalled            = false
		ensureServiceAccountCalled     = false
		namespaceCalled                = false
		modelUUID                      = "abcd-efff-face"
		agentPath                      = "/var/app/juju"
		namespace                      = "test-namespace"
	)

	config := caas.ModelOperatorConfig{
		AgentConf:         []byte("testconf"),
		OperatorImagePath: "juju/juju:123",
		Port:              int32(5497),
	}

	bridge := &modelOperatorBrokerBridge{
		ensureClusterRole: func(cr *rbac.ClusterRole) ([]func(), error) {
			ensureClusterRoleCalled = true
			c.Assert(cr.Name, gc.Equals, modelOperatorName)
			c.Assert(cr.Rules[0].APIGroups, jc.DeepEquals, []string{""})
			c.Assert(cr.Rules[0].Resources, jc.DeepEquals, []string{"namespaces"})
			c.Assert(cr.Rules[0].Verbs, jc.DeepEquals, []string{"get", "list"})
			c.Assert(cr.Rules[1].APIGroups, jc.DeepEquals, []string{"admissionregistration.k8s.io"})
			c.Assert(cr.Rules[1].Resources, jc.DeepEquals, []string{"mutatingwebhookconfigurations"})
			c.Assert(cr.Rules[1].Verbs, jc.DeepEquals, []string{
				"create",
				"delete",
				"get",
				"list",
				"update",
			})
			return nil, nil
		},
		ensureClusterRoleBinding: func(crb *rbac.ClusterRoleBinding) ([]func(), error) {
			ensureClusterRoleBindingCalled = true
			c.Assert(crb.Name, gc.Equals, modelOperatorName)
			c.Assert(crb.RoleRef.APIGroup, gc.Equals, "rbac.authorization.k8s.io")
			c.Assert(crb.RoleRef.Kind, gc.Equals, "ClusterRole")
			c.Assert(crb.RoleRef.Name, gc.Equals, modelOperatorName)
			return nil, nil
		},
		ensureConfigMap: func(cm *core.ConfigMap) ([]func(), error) {
			ensureConfigMapCalled = true
			c.Assert(cm.Name, gc.Equals, modelOperatorName)
			c.Assert(cm.Namespace, gc.Equals, namespace)

			conf, ok := cm.Data[modelOperatorConfigMapAgentConfKey(modelOperatorName)]
			c.Assert(ok, gc.Equals, true)
			c.Assert(conf, jc.DeepEquals, string(config.AgentConf))
			return nil, nil
		},
		ensureDeployment: func(d *apps.Deployment) ([]func(), error) {
			ensureDeploymentCalled = true
			c.Assert(d.Name, gc.Equals, modelOperatorName)
			c.Assert(d.Namespace, gc.Equals, namespace)
			c.Assert(d.Spec.Template.Spec.Containers[0].Image, gc.Equals, config.OperatorImagePath)
			c.Assert(d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort, gc.Equals, config.Port)
			return nil, nil
		},
		ensureRole: func(r *rbac.Role) ([]func(), error) {
			ensureRoleCalled = true
			c.Assert(r.Name, gc.Equals, modelOperatorName)
			c.Assert(r.Namespace, gc.Equals, namespace)
			c.Assert(r.Rules[0].APIGroups, jc.DeepEquals, []string{""})
			c.Assert(r.Rules[0].Resources, jc.DeepEquals, []string{"serviceaccounts"})
			c.Assert(r.Rules[0].Verbs, jc.DeepEquals, []string{
				"get",
				"list",
				"watch",
			})
			return nil, nil
		},
		ensureRoleBinding: func(rb *rbac.RoleBinding) ([]func(), error) {
			ensureRoleBindingCalled = true
			c.Assert(rb.Name, gc.Equals, modelOperatorName)
			c.Assert(rb.Namespace, gc.Equals, namespace)
			c.Assert(rb.RoleRef.APIGroup, gc.Equals, "rbac.authorization.k8s.io")
			c.Assert(rb.RoleRef.Kind, gc.Equals, "Role")
			c.Assert(rb.RoleRef.Name, gc.Equals, modelOperatorName)
			return nil, nil
		},
		ensureServiceAccount: func(s *core.ServiceAccount) ([]func(), error) {
			trueVar := true
			ensureServiceAccountCalled = true
			c.Assert(s.Name, gc.Equals, modelOperatorName)
			c.Assert(s.Namespace, gc.Equals, namespace)
			c.Assert(s.AutomountServiceAccountToken, jc.DeepEquals, &trueVar)
			return nil, nil
		},
		ensureService: func(s *core.Service) ([]func(), error) {
			ensureServiceCalled = true
			c.Assert(s.Name, gc.Equals, modelOperatorName)
			c.Assert(s.Namespace, gc.Equals, namespace)
			c.Assert(s.Spec.Ports[0].Port, gc.Equals, config.Port)
			return nil, nil
		},
		namespace: func() string {
			namespaceCalled = true
			return namespace
		},
	}

	err := ensureModelOperator(modelUUID, agentPath, &config, bridge)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(ensureClusterRoleCalled, jc.IsTrue)
	c.Assert(ensureClusterRoleBindingCalled, jc.IsTrue)
	c.Assert(ensureConfigMapCalled, jc.IsTrue)
	c.Assert(ensureDeploymentCalled, jc.IsTrue)
	c.Assert(ensureRoleCalled, jc.IsTrue)
	c.Assert(ensureRoleBindingCalled, jc.IsTrue)
	c.Assert(ensureServiceAccountCalled, jc.IsTrue)
	c.Assert(ensureServiceCalled, jc.IsTrue)
	c.Assert(namespaceCalled, jc.IsTrue)
}
