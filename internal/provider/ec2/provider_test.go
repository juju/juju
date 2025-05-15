// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"github.com/aws/smithy-go"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/ec2"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type ProviderSuite struct {
	testhelpers.IsolationSuite
	spec     environscloudspec.CloudSpec
	provider environs.EnvironProvider
}

var _ = tc.Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	credential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "foo",
			"secret-key": "bar",
		},
	)
	s.spec = environscloudspec.CloudSpec{
		Type:       "ec2",
		Name:       "aws",
		Region:     "us-east-1",
		Credential: &credential,
	}

	provider, err := environs.Provider("ec2")
	c.Assert(err, tc.ErrorIsNil)
	s.provider = provider
}

func (s *ProviderSuite) TestOpen(c *tc.C) {
	env, err := environs.Open(c.Context(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: coretesting.ModelConfig(c),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env, tc.NotNil)
}

func (s *ProviderSuite) TestOpenMissingCredential(c *tc.C) {
	s.spec.Credential = nil
	s.testOpenError(c, s.spec, `validating cloud spec: missing credential not valid`)
}

func (s *ProviderSuite) TestOpenUnsupportedCredential(c *tc.C) {
	credential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "userpass" auth-type not supported`)
}

func (s *ProviderSuite) testOpenError(c *tc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := environs.Open(c.Context(), s.provider, environs.OpenParams{
		Cloud:  spec,
		Config: coretesting.ModelConfig(c),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorMatches, expect)
}

func (s *ProviderSuite) TestVerifyCredentialsErrs(c *tc.C) {
	err := ec2.VerifyCredentials(c.Context(), nil)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Assert(err, tc.Not(tc.ErrorIs), common.ErrorCredentialNotValid)
}

func (s *ProviderSuite) TestIsAuthorizationErrorIgnoresNil(c *tc.C) {
	isAuthErr := ec2.IsAuthorizationError(nil)
	c.Assert(isAuthErr, tc.IsFalse)
}

func (s *ProviderSuite) TestIsAuthorizationErrorConvertsCredentialRelatedFailures(c *tc.C) {
	for _, code := range []string{
		"AuthFailure",
		"InvalidClientTokenId",
		"MissingAuthenticationToken",
		"Blocked",
		"CustomerKeyHasBeenRevoked",
		"PendingVerification",
		"SignatureDoesNotMatch",
	} {
		isAuthErr := ec2.IsAuthorizationError(
			&smithy.GenericAPIError{Code: code})
		c.Assert(isAuthErr, tc.IsTrue)
	}
}

func (s *ProviderSuite) TestIsAuthorizationErrorConvertsCredentialRelatedFailuresWrapped(c *tc.C) {
	for _, code := range []string{
		"AuthFailure",
		"InvalidClientTokenId",
		"MissingAuthenticationToken",
		"Blocked",
		"CustomerKeyHasBeenRevoked",
		"PendingVerification",
		"SignatureDoesNotMatch",
	} {
		isAuthErr := ec2.IsAuthorizationError(
			errors.Annotatef(&smithy.GenericAPIError{Code: code}, "wrapped"))
		c.Assert(isAuthErr, tc.IsTrue)
	}
}

func (s *ProviderSuite) TestIsAuthorizationErrorNotInvalidCredential(c *tc.C) {
	for _, code := range []string{
		"OptInRequired",
		"UnauthorizedOperation",
	} {
		isAuthErr := ec2.IsAuthorizationError(
			&smithy.GenericAPIError{Code: code})
		c.Assert(isAuthErr, tc.IsFalse)
	}
}

func (s *ProviderSuite) TestIsAuthorizationErrorHandlesOtherProviderErrors(c *tc.C) {
	// Any other ec2.Error is returned unwrapped.
	isAuthErr := ec2.IsAuthorizationError(&smithy.GenericAPIError{Code: "DryRunOperation"})
	c.Assert(isAuthErr, tc.IsFalse)
}

func (s *ProviderSuite) TestConvertAuthorizationErrorsCredentialRelatedFailures(c *tc.C) {
	for _, code := range []string{
		"AuthFailure",
		"InvalidClientTokenId",
		"MissingAuthenticationToken",
		"Blocked",
		"CustomerKeyHasBeenRevoked",
		"PendingVerification",
		"SignatureDoesNotMatch",
	} {
		authErr := ec2.ConvertAuthorizationError(
			&smithy.GenericAPIError{Code: code})
		c.Assert(authErr, tc.ErrorIs, common.ErrorCredentialNotValid)
	}
}

func (s *ProviderSuite) TestConvertAuthorizationErrorsNotInvalidCredential(c *tc.C) {
	for _, code := range []string{
		"OptInRequired",
		"UnauthorizedOperation",
	} {
		authErr := ec2.ConvertAuthorizationError(
			&smithy.GenericAPIError{Code: code})
		c.Assert(authErr, tc.Not(tc.ErrorIs), common.ErrorCredentialNotValid)
	}
}

func (s *ProviderSuite) TestConvertAuthorizationErrorsIsNil(c *tc.C) {
	authErr := ec2.ConvertAuthorizationError(nil)
	c.Assert(authErr, tc.ErrorIsNil)
}

func (s *ProviderSuite) TestConvertedCredentialError(c *tc.C) {
	// Trace() will keep error type
	inner := ec2.ConvertAuthorizationError(
		&smithy.GenericAPIError{Code: "Blocked"})
	traced := errors.Trace(inner)
	c.Assert(traced, tc.NotNil)
	c.Assert(traced, tc.ErrorIs, common.ErrorCredentialNotValid)

	// Annotate() will keep error type
	annotated := errors.Annotate(inner, "annotation")
	c.Assert(annotated, tc.NotNil)
	c.Assert(annotated, tc.ErrorIs, common.ErrorCredentialNotValid)

	// Running a CredentialNotValid through conversion call again is a no-op.
	again := ec2.ConvertAuthorizationError(inner)
	c.Assert(again, tc.NotNil)
	c.Assert(again, tc.ErrorIs, common.ErrorCredentialNotValid)
	c.Assert(again.Error(), tc.Contains, "\nYour Amazon account is currently blocked.: api error Blocked:")

	// Running an annotated CredentialNotValid through conversion call again is a no-op too.
	againAnotated := ec2.ConvertAuthorizationError(annotated)
	c.Assert(againAnotated, tc.NotNil)
	c.Assert(againAnotated, tc.ErrorIs, common.ErrorCredentialNotValid)
	c.Assert(againAnotated.Error(), tc.Contains, "\nYour Amazon account is currently blocked.: api error Blocked:")
}
