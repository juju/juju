// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/testing"
)

type credentialsSuite struct {
	testing.BaseSuite
	provider environs.EnvironProvider
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	c.Assert(s.provider, gc.Implements, new(environs.ProviderCredentials))
	providerCredentials := s.provider.(environs.ProviderCredentials)

	schemas := providerCredentials.CredentialSchemas()
	c.Assert(schemas, gc.HasLen, 1)
	_, ok := schemas["empty"]
	c.Assert(ok, jc.IsTrue, gc.Commentf("expected empty auth-type schema"))
}

func (s *credentialsSuite) TestDetectCredentials(c *gc.C) {
	providerCredentials := s.provider.(environs.ProviderCredentials)
	credentials, err := providerCredentials.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, gc.HasLen, 1)
	c.Assert(credentials[0], jc.DeepEquals, cloud.NewEmptyCredential())
}
