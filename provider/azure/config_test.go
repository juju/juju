// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"github.com/Azure/go-autorest/autorest/mocks"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/testing"
)

const (
	fakeApplicationId     = "00000000-0000-0000-0000-000000000000"
	fakeSubscriptionId    = "22222222-2222-2222-2222-222222222222"
	fakeStorageAccountKey = "quay"
)

type configSuite struct {
	testing.BaseSuite

	provider environs.EnvironProvider
}

var _ = gc.Suite(&configSuite{})

func (s *configSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider = newProvider(c, azure.ProviderConfig{
		Sender: mocks.NewSender(),
	})
}

func (s *configSuite) TestValidateNew(c *gc.C) {
	s.assertConfigValid(c, nil)
}

func (s *configSuite) TestValidateInvalidStorageAccountType(c *gc.C) {
	s.assertConfigInvalid(
		c, testing.Attrs{"storage-account-type": "savings"},
		`invalid storage account type "savings", expected one of: \["Premium_LRS" "Premium_ZRS" "Standard_GRS" "Standard_LRS" "Standard_RAGRS" "Standard_ZRS"\]`,
	)
}

func (s *configSuite) TestValidateInvalidLoadBalancerSkuName(c *gc.C) {
	s.assertConfigInvalid(
		c, testing.Attrs{"load-balancer-sku-name": "premium"},
		`invalid load balancer SKU name "Premium", expected one of: \["Basic" "Standard"\]`,
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

func (s *configSuite) TestValidateStorageAccountTypeCantChange(c *gc.C) {
	cfgOld := makeTestModelConfig(c, testing.Attrs{"storage-account-type": "Standard_LRS"})
	_, err := s.provider.Validate(cfgOld, cfgOld)
	c.Assert(err, jc.ErrorIsNil)

	cfgNew := makeTestModelConfig(c, testing.Attrs{"storage-account-type": "Premium_LRS"})
	_, err = s.provider.Validate(cfgNew, cfgOld)
	c.Assert(err, gc.ErrorMatches, `cannot change immutable "storage-account-type" config \(Standard_LRS -> Premium_LRS\)`)
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
