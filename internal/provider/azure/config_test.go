// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provider/azure"
	"github.com/juju/juju/internal/provider/azure/internal/azuretesting"
	"github.com/juju/juju/testing"
)

const (
	fakeApplicationId         = "60a04dc9-1857-425f-8076-5ba81ca53d66"
	fakeTenantId              = "11111111-1111-1111-1111-111111111111"
	fakeSubscriptionId        = "22222222-2222-2222-2222-222222222222"
	fakeManagedSubscriptionId = "33333333-3333-3333-3333-333333333333"
)

type configSuite struct {
	testing.BaseSuite

	provider environs.EnvironProvider
}

var _ = gc.Suite(&configSuite{})

func (s *configSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider = newProvider(c, azure.ProviderConfig{
		Sender: &azuretesting.MockSender{},
	})
}

func (s *configSuite) TestValidateNew(c *gc.C) {
	s.assertConfigValid(c, nil)
}

func (s *configSuite) TestValidateInvalidLoadBalancerSkuName(c *gc.C) {
	s.assertConfigInvalid(
		c, testing.Attrs{"load-balancer-sku-name": "premium"},
		`invalid load balancer SKU name "Premium", expected one of: \["Basic" "Gateway" "Standard"\]`,
	)
}

func (s *configSuite) TestValidateInvalidFirewallMode(c *gc.C) {
	s.assertConfigInvalid(
		c, testing.Attrs{"firewall-mode": "global"},
		"global firewall mode is not supported",
	)
}

func (s *configSuite) TestValidateModelNameLength(c *gc.C) {
	s.assertConfigInvalid(
		c, testing.Attrs{"name": "someextremelyoverlylongishmodelname-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		`resource group name "juju-someextremelyoverlylongishmodelname-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-deadbeef" is too long

Please choose a model name of no more than 66 characters.`)
}

func (s *configSuite) TestValidateResourceGroupNameLength(c *gc.C) {
	s.assertConfigInvalid(
		c, testing.Attrs{"resource-group-name": "someextremelyoverlylongishresourcegroupname-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		`resource group name "someextremelyoverlylongishresourcegroupname-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" is too long

Please choose a name of no more than 80 characters.`)
}

func (s *configSuite) TestValidateLoadBalancerSkuNameCanChange(c *gc.C) {
	cfgOld := makeTestModelConfig(c, testing.Attrs{"load-balancer-sku-name": "Standard"})
	_, err := s.provider.Validate(cfgOld, cfgOld)
	c.Assert(err, jc.ErrorIsNil)

	cfgNew := makeTestModelConfig(c, testing.Attrs{"load-balancer-sku-name": "Basic"})
	_, err = s.provider.Validate(cfgNew, cfgOld)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.provider.Validate(cfgOld, cfgNew)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configSuite) TestValidateResourceGroupNameCantChange(c *gc.C) {
	cfgOld := makeTestModelConfig(c, testing.Attrs{"resource-group-name": "foo"})
	_, err := s.provider.Validate(cfgOld, cfgOld)
	c.Assert(err, jc.ErrorIsNil)

	cfgNew := makeTestModelConfig(c, testing.Attrs{"resource-group-name": "bar"})
	_, err = s.provider.Validate(cfgNew, cfgOld)
	c.Assert(err, gc.ErrorMatches, `cannot change immutable "resource-group-name" config \(foo -> bar\)`)
}

func (s *configSuite) TestValidateVirtualNetworkNameCantChange(c *gc.C) {
	cfgOld := makeTestModelConfig(c, testing.Attrs{"network": "foo"})
	_, err := s.provider.Validate(cfgOld, cfgOld)
	c.Assert(err, jc.ErrorIsNil)

	cfgNew := makeTestModelConfig(c, testing.Attrs{"network": "bar"})
	_, err = s.provider.Validate(cfgNew, cfgOld)
	c.Assert(err, gc.ErrorMatches, `cannot change immutable "network" config \(foo -> bar\)`)
}

func (s *configSuite) assertConfigValid(c *gc.C, attrs testing.Attrs) {
	cfg := makeTestModelConfig(c, attrs)
	_, err := s.provider.Validate(cfg, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configSuite) assertConfigInvalid(c *gc.C, attrs testing.Attrs, expect string) {
	cfg := makeTestModelConfig(c, attrs)
	_, err := s.provider.Validate(cfg, nil)
	c.Assert(err, gc.ErrorMatches, expect)
}

func makeTestModelConfig(c *gc.C, extra ...testing.Attrs) *config.Config {
	attrs := testing.Attrs{
		"type":          "azure",
		"agent-version": "1.2.3",
	}
	for _, extra := range extra {
		attrs = attrs.Merge(extra)
	}
	attrs = testing.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}
