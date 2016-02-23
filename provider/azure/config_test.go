// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/mocks"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/testing"
)

const (
	fakeApplicationId     = "00000000-0000-0000-0000-000000000000"
	fakeTenantId          = "11111111-1111-1111-1111-111111111111"
	fakeSubscriptionId    = "22222222-2222-2222-2222-222222222222"
	fakeStorageAccount    = "mrblobby"
	fakeStorageAccountKey = "quay"
)

type configSuite struct {
	testing.BaseSuite

	provider environs.EnvironProvider
}

var _ = gc.Suite(&configSuite{})

func (s *configSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider, _ = newProviders(c, azure.ProviderConfig{
		Sender: mocks.NewSender(),
	})
}

func (s *configSuite) TestValidateNew(c *gc.C) {
	s.assertConfigValid(c, nil)
}

func (s *configSuite) TestValidateInvalidStorageAccountType(c *gc.C) {
	s.assertConfigInvalid(
		c, testing.Attrs{"storage-account-type": "savings"},
		`invalid storage account type "savings", expected one of: \["Standard_LRS" "Standard_GRS" "Standard_RAGRS" "Standard_ZRS" "Premium_LRS"\]`,
	)
}

func (s *configSuite) TestValidateInvalidFirewallMode(c *gc.C) {
	s.assertConfigInvalid(
		c, testing.Attrs{"firewall-mode": "global"},
		"global firewall mode is not supported",
	)
}

func (s *configSuite) TestValidateLocation(c *gc.C) {
	s.assertConfigInvalid(c, testing.Attrs{"location": ""}, `"location" config not specified`)
	// We don't validate locations, because new locations may be added.
	// Azure will complain if the location is invalid anyway.
	s.assertConfigValid(c, testing.Attrs{"location": "eurasia"})
}

func (s *configSuite) TestValidateInvalidCredentials(c *gc.C) {
	s.assertConfigInvalid(c, testing.Attrs{"application-id": ""}, `"application-id" config not specified`)
	s.assertConfigInvalid(c, testing.Attrs{"application-password": ""}, `"application-password" config not specified`)
	s.assertConfigInvalid(c, testing.Attrs{"tenant-id": ""}, `"tenant-id" config not specified`)
	s.assertConfigInvalid(c, testing.Attrs{"subscription-id": ""}, `"subscription-id" config not specified`)
	s.assertConfigInvalid(c, testing.Attrs{"controller-resource-group": ""}, `"controller-resource-group" config not specified`)
}

func (s *configSuite) TestValidateStorageAccountCantChange(c *gc.C) {
	cfgOld := makeTestModelConfig(c, testing.Attrs{"storage-account": "abc"})
	_, err := s.provider.Validate(cfgOld, cfgOld)
	c.Assert(err, jc.ErrorIsNil)

	cfgNew := makeTestModelConfig(c) // no storage-account attribute
	_, err = s.provider.Validate(cfgNew, cfgOld)
	c.Assert(err, gc.ErrorMatches, `cannot remove immutable "storage-account" config`)

	cfgNew = makeTestModelConfig(c, testing.Attrs{"storage-account": "def"})
	_, err = s.provider.Validate(cfgNew, cfgOld)
	c.Assert(err, gc.ErrorMatches, `cannot change immutable "storage-account" config \(abc -> def\)`)
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
		"type":                      "azure",
		"application-id":            fakeApplicationId,
		"tenant-id":                 fakeTenantId,
		"application-password":      "opensezme",
		"subscription-id":           fakeSubscriptionId,
		"location":                  "westus",
		"endpoint":                  "https://api.azurestack.local",
		"storage-endpoint":          "https://storage.azurestack.local",
		"controller-resource-group": "arbitrary",
		"agent-version":             "1.2.3",
	}
	for _, extra := range extra {
		attrs = attrs.Merge(extra)
	}
	attrs = testing.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}
