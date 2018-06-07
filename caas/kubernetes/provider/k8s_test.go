// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/testing"
)

type K8sSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&K8sSuite{})

func (s *K8sSuite) TestMakeUnitSpecNoConfigConfig(c *gc.C) {
	podSpec := caas.PodSpec{
		Containers: []caas.ContainerSpec{{
			Name:  "test",
			Ports: []caas.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image: "juju/image",
			ProviderContainer: &provider.K8sContainerSpec{
				ImagePullPolicy: core.PullAlways,
				ReadinessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					Handler:             core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
				},
				LivenessProbe: &core.Probe{
					SuccessThreshold: 20,
					Handler:          core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
				},
			},
		}, {
			Name:  "test2",
			Ports: []caas.ContainerPort{{ContainerPort: 8080, Protocol: "TCP"}},
			Image: "juju/image2",
		}},
	}
	spec, err := provider.MakeUnitSpec(&podSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.PodSpec(spec), jc.DeepEquals, core.PodSpec{
		Containers: []core.Container{
			{
				Name:            "test",
				Image:           "juju/image",
				Ports:           []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
				ImagePullPolicy: core.PullAlways,
				ReadinessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					Handler:             core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
				},
				LivenessProbe: &core.Probe{
					SuccessThreshold: 20,
					Handler:          core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
				},
			}, {
				Name:  "test2",
				Image: "juju/image2",
				Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP}},
			},
		},
	})
}

func (s *K8sSuite) TestMakeUnitSpecConfigPairs(c *gc.C) {
	podSpec := caas.PodSpec{
		Containers: []caas.ContainerSpec{{
			Name:  "test",
			Ports: []caas.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image: "juju/image",
			Config: map[string]string{
				"foo": "bar",
			},
		}, {
			Name:  "test2",
			Ports: []caas.ContainerPort{{ContainerPort: 8080, Protocol: "TCP"}},
			Image: "juju/image2",
		}}}
	spec, err := provider.MakeUnitSpec(&podSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.PodSpec(spec), jc.DeepEquals, core.PodSpec{
		Containers: []core.Container{
			{
				Name:  "test",
				Image: "juju/image",
				Ports: []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
				Env: []core.EnvVar{
					{Name: "foo", Value: "bar"},
				},
			}, {
				Name:  "test2",
				Image: "juju/image2",
				Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP}},
			},
		},
	})
}

func (s *K8sSuite) TestOperatorPodConfig(c *gc.C) {
	pod := provider.OperatorPod("gitlab", "/var/lib/juju", "jujusolutions/caas-jujud-operator", "2.99.0")
	c.Assert(pod.Name, gc.Equals, "juju-operator-gitlab")
	c.Assert(pod.Labels, jc.DeepEquals, map[string]string{
		"juju-operator": "gitlab",
		"juju-version":  "2.99.0",
	})
	c.Assert(pod.Spec.Containers, gc.HasLen, 1)
	c.Assert(pod.Spec.Containers[0].Image, gc.Equals, "jujusolutions/caas-jujud-operator")
	c.Assert(pod.Spec.Containers[0].VolumeMounts, gc.HasLen, 1)
	c.Assert(pod.Spec.Containers[0].VolumeMounts[0].MountPath, gc.Equals, "/var/lib/juju/agents/application-gitlab/agent.conf")
}

type K8sBrokerSuite struct {
	BaseSuite
}

var _ = gc.Suite(&K8sBrokerSuite{})

func (s *K8sBrokerSuite) TestEnsureNamespace(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}}
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Update(ns).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockNamespaces.EXPECT().Create(ns).Times(1),
		// Idempotent check.
		s.mockNamespaces.EXPECT().Update(ns).Times(1),
	)

	err := s.broker.EnsureNamespace()
	c.Assert(err, jc.ErrorIsNil)

	// Check idempotent.
	err = s.broker.EnsureNamespace()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestDeleteService(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	// Delete operations below return a not found to ensure it's treated as a no-op.
	gomock.InOrder(
		s.mockServices.EXPECT().Delete("juju-test", s.deleteOptions(false)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockDeploymentInterface.EXPECT().Delete("juju-test", s.deleteOptions(false)).Times(1).
			Return(s.k8sNotFoundError()),
	)

	err := s.broker.DeleteService("test")
	c.Assert(err, jc.ErrorIsNil)
}
