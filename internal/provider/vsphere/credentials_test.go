// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type credentialsSuite struct {
	testhelpers.IsolationSuite
	provider environs.EnvironProvider
}

func TestCredentialsSuite(t *stdtesting.T) {
	tc.Run(t, &credentialsSuite{})
}

func (s *credentialsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("vsphere")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *tc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "userpass")
}

func (s *credentialsSuite) TestUserPassCredentialsValid(c *tc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "userpass", map[string]string{
		"user":     "bob",
		"password": "dobbs",
	})
}

func (s *credentialsSuite) TestUserPassHiddenAttributes(c *tc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "userpass", "password")
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *tc.C) {
	_, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}
