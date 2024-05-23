// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/state"
)

type containerTestNetworkLessEnviron struct {
	environs.Environ
}

type containerTestNetworkedEnviron struct {
	environs.NetworkingEnviron

	stub                       *testing.Stub
	supportsContainerAddresses bool
}

type ContainerNetworkingSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ContainerNetworkingSuite{})

func (e *containerTestNetworkedEnviron) SupportsContainerAddresses(ctx envcontext.ProviderCallContext) (bool, error) {
	e.stub.AddCall("SupportsContainerAddresses", ctx)
	return e.supportsContainerAddresses, e.stub.NextErr()
}

var _ environs.NetworkingEnviron = (*containerTestNetworkedEnviron)(nil)

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingNetworkless(c *gc.C) {
	err := s.Model.AutoConfigureContainerNetworking(containerTestNetworkLessEnviron{}, state.NoopConfigSchemaSource)
	c.Assert(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "local")
	c.Check(attrs["fan-config"], gc.Equals, "")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingDoesntChangeDefault(c *gc.C) {
	err := s.Model.UpdateModelConfig(state.NoopConfigSchemaSource, map[string]interface{}{
		"container-networking-method": "provider",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.Model.AutoConfigureContainerNetworking(containerTestNetworkLessEnviron{}, state.NoopConfigSchemaSource)
	c.Assert(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "provider")
	c.Check(attrs["fan-config"], gc.Equals, "")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingAlreadyConfigured(c *gc.C) {
	environ := containerTestNetworkedEnviron{
		stub: &testing.Stub{},
	}
	err := s.Model.UpdateModelConfig(state.NoopConfigSchemaSource, map[string]interface{}{
		"container-networking-method": "local",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.Model.AutoConfigureContainerNetworking(&environ, state.NoopConfigSchemaSource)
	c.Check(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "local")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingSupportsContainerAddresses(c *gc.C) {
	environ := containerTestNetworkedEnviron{
		stub:                       &testing.Stub{},
		supportsContainerAddresses: true,
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ, state.NoopConfigSchemaSource)
	c.Check(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "provider")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingDefault(c *gc.C) {
	environ := containerTestNetworkedEnviron{
		stub:                       &testing.Stub{},
		supportsContainerAddresses: false,
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ, state.NoopConfigSchemaSource)
	c.Check(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "local")
}

func (s *ContainerNetworkingSuite) TestAutoConfigureContainerNetworkingIgnoresIPv6(c *gc.C) {
	environ := containerTestNetworkedEnviron{
		stub:                       &testing.Stub{},
		supportsContainerAddresses: true,
	}
	err := s.Model.AutoConfigureContainerNetworking(&environ, state.NoopConfigSchemaSource)
	c.Check(err, jc.ErrorIsNil)
	config, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	attrs := config.AllAttrs()
	c.Check(attrs["container-networking-method"], gc.Equals, "provider")
}
