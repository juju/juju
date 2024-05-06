// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix_test

import (
	"context"
	stdcontext "context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/provider/equinix"
)

type providerSuite struct {
	provider environs.CloudEnvironProvider
	testing.IsolationSuite
	dialStub testing.Stub
	callCtx  envcontext.ProviderCallContext
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.dialStub.ResetCalls()
	s.provider = equinix.NewProvider()
	s.callCtx = envcontext.WithoutCredentialInvalidator(stdcontext.Background())
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) TestRegistered(c *gc.C) {
	provider, err := environs.Provider("equinix")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider, gc.NotNil)
}

func (s *providerSuite) TestOpen(c *gc.C) {
	config := fakeConfig(c)
	env, err := environs.Open(envcontext.WithoutCredentialInvalidator(stdcontext.Background()), s.provider, environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: config,
	})
	c.Check(err, jc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), gc.Equals, "testmodel")
}

func (s *providerSuite) TestPrepareConfig(c *gc.C) {
	cfg, err := s.provider.PrepareConfig(context.Background(), environs.PrepareConfigParams{
		Config: fakeConfig(c),
		Cloud:  fakeCloudSpec(),
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *providerSuite) TestValidate(c *gc.C) {
	config := fakeConfig(c)
	validCfg, err := s.provider.Validate(context.Background(), config, nil)
	c.Assert(err, jc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(config.AllAttrs(), gc.DeepEquals, validAttrs)
}
