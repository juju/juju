// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/testing"
)

type credentialsSuite struct {
	testing.BaseSuite
	provider environs.EnvironProvider
}

var _ = tc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *tc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "empty")
}

func (s *credentialsSuite) TestDetectCredentials(c *tc.C) {
	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, jc.DeepEquals, cloud.NewEmptyCloudCredential())
}
