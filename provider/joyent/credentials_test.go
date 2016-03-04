// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
)

type credentialsSuite struct {
	testing.IsolationSuite
	provider environs.EnvironProvider
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("joyent")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "userpass")
}

func (s *credentialsSuite) TestUserPassCredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "userpass", map[string]string{
		"sdc-user":     "sdc-user",
		"sdc-key-id":   "sdc-key-id",
		"manta-user":   "manta-user",
		"manta-key-id": "manta-key-id",
		"private-key":  "private-key",
		"algorithm":    "algorithm",
	})
}

func (s *credentialsSuite) TestUserPassHiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "userpass", "private-key")
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *gc.C) {
	// No environment variables set, so no credentials should be found.
	_, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
