// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/ecs"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
)

type credentialsSuite struct {
	testing.FakeHomeSuite
	provider environs.EnvironProvider
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("ecs")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "access-key")
}

func (s *credentialsSuite) TestCredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "access-key", map[string]string{
		"access-key":   "access-key",
		"secret-key":   "secret-key",
		"cluster-name": "cluster-name",
		"region":       "ap-southeast-2",
	})
}

func (s *credentialsSuite) TestHiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "access-key", "secret-key")
}

func (s *credentialsSuite) TestDetectCredentials(c *gc.C) {
	_, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

type cloudCredentailTestCase struct {
	credential     *cloud.Credential
	expectedErrStr string
}

func newCredential(authType cloud.AuthType, attributes map[string]string) *cloud.Credential {
	cred := cloud.NewCredential(authType, attributes)
	return &cred
}

func (s *credentialsSuite) TestValidateCloudCredential(c *gc.C) {
	for i, tc := range []cloudCredentailTestCase{
		{
			credential:     nil,
			expectedErrStr: `missing credential not valid`,
		},
		{
			credential:     newCredential("", nil),
			expectedErrStr: `missing auth-type not valid`,
		},
		{
			credential:     newCredential(cloud.AccessKeyAuthType, nil),
			expectedErrStr: `empty credential attributes not valid`,
		},
		{
			credential: newCredential(cloud.AccessKeyAuthType, map[string]string{
				"access-key": "access-key",
			}),
			expectedErrStr: `empty "secret-key" not valid`,
		},
		{
			credential: newCredential(cloud.AccessKeyAuthType, map[string]string{
				"access-key": "access-key",
				"secret-key": "secret-key",
			}),
			expectedErrStr: `empty "region" not valid`,
		},
		{
			credential: newCredential(cloud.AccessKeyAuthType, map[string]string{
				"access-key": "access-key",
				"secret-key": "secret-key",
				"region":     "ap-southeast-2",
			}),
			expectedErrStr: `empty "cluster-name" not valid`,
		},
		{
			credential: newCredential(cloud.AccessKeyAuthType, map[string]string{
				"access-key":   "access-key",
				"secret-key":   "secret-key",
				"cluster-name": "cluster-name",
			}),
			expectedErrStr: `empty "region" not valid`,
		},
		{
			credential: newCredential(cloud.AccessKeyAuthType, map[string]string{
				"access-key":   "access-key",
				"secret-key":   "secret-key",
				"cluster-name": "cluster-name",
				"region":       "ap-southeast-2",
			}),
			expectedErrStr: ``,
		},
	} {
		c.Logf("%v: %v", i, tc.expectedErrStr)
		err := ecs.ValidateCloudCredential(tc.credential)
		if len(tc.expectedErrStr) == 0 {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, tc.expectedErrStr)
		}
	}
}
