// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

type containerTestNetworkLessEnviron struct {
	environs.Environ
}

type containerTestNetworkedEnviron struct {
	environs.NetworkingEnviron

	stub                       *testing.Stub
	supportsContainerAddresses bool
	superSubnets               []string
}

type ContainerNetworkingSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ContainerNetworkingSuite{})

func (e *containerTestNetworkedEnviron) SuperSubnets(ctx context.ProviderCallContext) ([]string, error) {
	e.stub.AddCall("SuperSubnets", ctx)
	return e.superSubnets, e.stub.NextErr()
}

func (e *containerTestNetworkedEnviron) SupportsContainerAddresses(ctx context.ProviderCallContext) (bool, error) {
	e.stub.AddCall("SupportsContainerAddresses", ctx)
	return e.supportsContainerAddresses, e.stub.NextErr()
}

var _ environs.NetworkingEnviron = (*containerTestNetworkedEnviron)(nil)

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingNetworkless(c *gc.C) {
	err := s.Model.AutoConfigureContainerNetworking(containerTestNetworkLessEnviron{})
	c.Assert(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "local")
	c.Assert(attrs["fan-config"], gc.Equals, "")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingDoesntChangeDefault(c *gc.C) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{
		"container-networking-method": "provider",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.Model.AutoConfigureContainerNetworking(containerTestNetworkLessEnviron{})
	c.Assert(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "provider")
	c.Assert(attrs["fan-config"], gc.Equals, "")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingAlreadyConfigured(c *gc.C) {
	environ := containerTestNetworkedEnviron{
		stub:         &testing.Stub{},
		superSubnets: []string{"172.31.0.0/16", "192.168.1.0/24", "10.0.0.0/8"},
	}
	err := s.Model.UpdateModelConfig(map[string]interface{}{
		"container-networking-method": "local",
		"fan-config":                  "1.2.3.4/24=5.6.7.8/16",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "local")
	c.Assert(attrs["fan-config"], gc.Equals, "1.2.3.4/24=5.6.7.8/16")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingNoSuperSubnets(c *gc.C) {
	environ := containerTestNetworkedEnviron{
		stub: &testing.Stub{},
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "local")
	c.Assert(attrs["fan-config"], gc.Equals, "")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingSupportsContainerAddresses(c *gc.C) {
	environ := containerTestNetworkedEnviron{
		stub:                       &testing.Stub{},
		supportsContainerAddresses: true,
		superSubnets:               []string{"172.31.0.0/16", "192.168.1.0/24", "10.0.0.0/8"},
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "provider")
	c.Assert(attrs["fan-config"], gc.Equals, "172.31.0.0/16=252.0.0.0/8 192.168.1.0/24=253.0.0.0/8")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingDefault(c *gc.C) {
	environ := containerTestNetworkedEnviron{
		stub:                       &testing.Stub{},
		supportsContainerAddresses: false,
		superSubnets:               []string{"172.31.0.0/16", "192.168.1.0/24", "10.0.0.0/8"},
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "fan")
	c.Assert(attrs["fan-config"], gc.Equals, "172.31.0.0/16=252.0.0.0/8 192.168.1.0/24=253.0.0.0/8")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingIgnoresIPv6(c *gc.C) {
	environ := containerTestNetworkedEnviron{
		stub:                       &testing.Stub{},
		supportsContainerAddresses: true,
		superSubnets:               []string{"172.31.0.0/16", "2000::dead:beef:1/64"},
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "provider")
	c.Assert(attrs["fan-config"], gc.Equals, "172.31.0.0/16=252.0.0.0/8")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingIgnoresNonFan(c *gc.C) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{
		"container-networking-method": "provider",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	environ := containerTestNetworkedEnviron{
		stub:                       &testing.Stub{},
		supportsContainerAddresses: true,
		superSubnets:               []string{"172.31.0.0/16", "192.168.1.0/24", "10.0.0.0/8"},
	}
	err = s.Model.AutoConfigureContainerNetworking(&environ)
	c.Check(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "provider")
	c.Assert(attrs["fan-config"], gc.Equals, "")
}
