// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"

	"github.com/juju/juju/caas"
)

type ModelOperatorSuite struct {
}

var _ = gc.Suite(&ModelOperatorSuite{})

func (m *ModelOperatorSuite) Test(c *gc.C) {
	var (
		ensureConfigMapCalled  = false
		ensureDeploymentCalled = false
		ensureServiceCalled    = false
		namespaceCalled        = false
		modelUUID              = "abcd-efff-face"
		agentPath              = "/var/app/juju"
		namespace              = "test-namespace"
	)

	config := caas.ModelOperatorConfig{
		AgentConf:         []byte("testconf"),
		OperatorImagePath: "juju/juju:123",
		Port:              int32(5497),
	}

	bridge := &modelOperatorBrokerBridge{
		ensureConfigMap: func(cm *core.ConfigMap) error {
			ensureConfigMapCalled = true
			c.Assert(cm.Name, gc.Equals, modelOperatorName)
			c.Assert(cm.Namespace, gc.Equals, namespace)

			conf, ok := cm.Data[modelOperatorConfigMapAgentConfKey(modelOperatorName)]
			c.Assert(ok, gc.Equals, true)
			c.Assert(conf, jc.DeepEquals, string(config.AgentConf))
			return nil
		},
		ensureDeployment: func(d *apps.Deployment) error {
			ensureDeploymentCalled = true
			c.Assert(d.Name, gc.Equals, modelOperatorName)
			c.Assert(d.Namespace, gc.Equals, namespace)
			c.Assert(d.Spec.Template.Spec.Containers[0].Image, gc.Equals, config.OperatorImagePath)
			c.Assert(d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort, gc.Equals, config.Port)
			return nil
		},
		ensureService: func(s *core.Service) error {
			ensureServiceCalled = true
			c.Assert(s.Name, gc.Equals, modelOperatorName)
			c.Assert(s.Namespace, gc.Equals, namespace)
			c.Assert(s.Spec.Ports[0].Port, gc.Equals, config.Port)
			return nil
		},
		namespace: func() string {
			namespaceCalled = true
			return namespace
		},
	}

	err := ensureModelOperator(modelUUID, agentPath, &config, bridge)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(ensureConfigMapCalled, gc.Equals, true)
	c.Assert(ensureDeploymentCalled, gc.Equals, true)
	c.Assert(ensureServiceCalled, gc.Equals, true)
	c.Assert(namespaceCalled, gc.Equals, true)
}
