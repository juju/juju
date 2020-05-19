// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/amz.v3/aws"
	ec2cloud "gopkg.in/amz.v3/ec2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/ec2"
	coretesting "github.com/juju/juju/testing"
)

type ProviderSuite struct {
	testing.IsolationSuite
	spec     environs.CloudSpec
	provider environs.EnvironProvider
}

var _ = gc.Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	credential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "foo",
			"secret-key": "bar",
		},
	)
	s.spec = environs.CloudSpec{
		Type:       "ec2",
		Name:       "aws",
		Region:     "us-east-1",
		Credential: &credential,
	}

	provider, err := environs.Provider("ec2")
	c.Assert(err, jc.ErrorIsNil)
	s.provider = provider
}

func (s *ProviderSuite) TestOpen(c *gc.C) {
	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: coretesting.ModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (s *ProviderSuite) TestOpenUnknownRegion(c *gc.C) {
	// This test shows that we do *not* check the region names against
	// anything in the client. That means that when new regions are
	// added to AWS, we'll be able to support them.
	s.spec.Region = "foobar"
	_, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: coretesting.ModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ProviderSuite) TestOpenKnownRegionInvalidEndpoint(c *gc.C) {
	s.PatchValue(&aws.Regions, map[string]aws.Region{
		"us-east-1": {
			EC2Endpoint: "https://testing.invalid",
		},
	})
	s.spec.Endpoint = "https://us-east-1.aws.amazon.com/v1.2/"

	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: coretesting.ModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	ec2Client := ec2.EnvironEC2(env)
	c.Assert(ec2Client.Region.EC2Endpoint, gc.Equals, "https://testing.invalid")
}

func (s *ProviderSuite) TestOpenKnownRegionValidEndpoint(c *gc.C) {
	// If the endpoint in the cloudspec is not known to be invalid,
	// we ignore whatever is in aws.Regions. This way, if the AWS
	// endpoints do ever change, we can update public-clouds.yaml
	// and have it picked up.
	s.PatchValue(&aws.Regions, map[string]aws.Region{
		"us-east-1": {
			EC2Endpoint: "https://testing.invalid",
		},
	})
	s.spec.Endpoint = "https://ec2.us-east-1.amazonaws.com"

	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: coretesting.ModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	ec2Client := ec2.EnvironEC2(env)
	c.Assert(ec2Client.Region.EC2Endpoint, gc.Equals, "https://ec2.us-east-1.amazonaws.com")
}

func (s *ProviderSuite) TestOpenMissingCredential(c *gc.C) {
	s.spec.Credential = nil
	s.testOpenError(c, s.spec, `validating cloud spec: missing credential not valid`)
}

func (s *ProviderSuite) TestOpenUnsupportedCredential(c *gc.C) {
	credential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "userpass" auth-type not supported`)
}

func (s *ProviderSuite) testOpenError(c *gc.C, spec environs.CloudSpec, expect string) {
	_, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  spec,
		Config: coretesting.ModelConfig(c),
	})
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *ProviderSuite) TestVerifyCredentialsErrs(c *gc.C) {
	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: coretesting.ModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)

	err = ec2.VerifyCredentials(env, context.NewCloudCallContext())
	c.Assert(err, gc.Not(jc.ErrorIsNil))
	c.Assert(err, gc.Not(jc.Satisfies), common.IsCredentialNotValid)
}

func (s *ProviderSuite) TestMaybeConvertCredentialErrorIgnoresNil(c *gc.C) {
	err := ec2.MaybeConvertCredentialError(nil, context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ProviderSuite) TestMaybeConvertCredentialErrorConvertsCredentialRelatedFailures(c *gc.C) {
	for _, code := range []string{
		"AuthFailure",
		"InvalidClientTokenId",
		"MissingAuthenticationToken",
		"Blocked",
		"CustomerKeyHasBeenRevoked",
		"PendingVerification",
		"SignatureDoesNotMatch",
	} {
		err := ec2.MaybeConvertCredentialError(&ec2cloud.Error{Code: code}, context.NewCloudCallContext())
		c.Assert(err, gc.NotNil)
		c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	}
}

func (s *ProviderSuite) TestMaybeConvertCredentialErrorNotInvalidCredential(c *gc.C) {
	for _, code := range []string{
		"OptInRequired",
		"UnauthorizedOperation",
	} {
		err := ec2.MaybeConvertCredentialError(&ec2cloud.Error{Code: code}, context.NewCloudCallContext())
		c.Assert(err, gc.NotNil)
		c.Assert(err, gc.Not(jc.Satisfies), common.IsCredentialNotValid)
	}
}

func (s *ProviderSuite) TestMaybeConvertCredentialErrorHandlesOtherProviderErrors(c *gc.C) {
	// Any other ec2.Error is returned unwrapped.
	err := ec2.MaybeConvertCredentialError(&ec2cloud.Error{Code: "DryRunOperation"}, context.NewCloudCallContext())
	c.Assert(err, gc.Not(jc.ErrorIsNil))
	c.Assert(err, gc.Not(jc.Satisfies), common.IsCredentialNotValid)
}

func (s *ProviderSuite) TestConvertedCredentialError(c *gc.C) {
	// Trace() will keep error type
	inner := ec2.MaybeConvertCredentialError(&ec2cloud.Error{Code: "Blocked"}, context.NewCloudCallContext())
	traced := errors.Trace(inner)
	c.Assert(traced, gc.NotNil)
	c.Assert(traced, jc.Satisfies, common.IsCredentialNotValid)

	// Annotate() will keep error type
	annotated := errors.Annotate(inner, "annotation")
	c.Assert(annotated, gc.NotNil)
	c.Assert(annotated, jc.Satisfies, common.IsCredentialNotValid)

	// Running a CredentialNotValid through conversion call again is a no-op.
	again := ec2.MaybeConvertCredentialError(inner, context.NewCloudCallContext())
	c.Assert(again, gc.NotNil)
	c.Assert(again, jc.Satisfies, common.IsCredentialNotValid)
	c.Assert(again.Error(), jc.Contains, "\nYour Amazon account is currently blocked.:  (Blocked)")

	// Running an annotated CredentialNotValid through conversion call again is a no-op too.
	againAnotated := ec2.MaybeConvertCredentialError(annotated, context.NewCloudCallContext())
	c.Assert(againAnotated, gc.NotNil)
	c.Assert(againAnotated, jc.Satisfies, common.IsCredentialNotValid)
	c.Assert(againAnotated.Error(), jc.Contains, "\nYour Amazon account is currently blocked.:  (Blocked)")
}
