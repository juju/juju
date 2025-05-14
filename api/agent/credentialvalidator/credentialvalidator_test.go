// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/credentialvalidator"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&CredentialValidatorSuite{})

type CredentialValidatorSuite struct {
	testhelpers.IsolationSuite
}

func (s *CredentialValidatorSuite) TestModelCredential(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CredentialValidator")
		c.Check(request, tc.Equals, "ModelCredential")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.ModelCredential{})
		*(result.(*params.ModelCredential)) = params.ModelCredential{
			Model:           modelTag.String(),
			CloudCredential: credentialTag.String(),
			Exists:          true,
			Valid:           true,
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	found, exists, err := client.ModelCredential(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)
	c.Assert(found, tc.DeepEquals, base.StoredCredential{CloudCredential: "cloud/user/credential", Valid: true})
}

func (s *CredentialValidatorSuite) TestModelCredentialIsNotNeeded(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ModelCredential)) = params.ModelCredential{
			Model:  modelTag.String(),
			Exists: false,
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, exists, err := client.ModelCredential(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsFalse)
}

func (s *CredentialValidatorSuite) TestModelCredentialInvalidCredentialTag(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ModelCredential)) = params.ModelCredential{
			Model:           modelTag.String(),
			Exists:          true,
			CloudCredential: "some-invalid-cloud-credential-tag-as-string",
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, exists, err := client.ModelCredential(c.Context())
	c.Assert(err, tc.ErrorMatches, `"some-invalid-cloud-credential-tag-as-string" is not a valid tag`)
	c.Assert(exists, tc.IsFalse)
}

func (s *CredentialValidatorSuite) TestModelCredentialCallError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("foo")
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, _, err := client.ModelCredential(c.Context())
	c.Assert(err, tc.ErrorMatches, "foo")
}

var (
	modelUUID = "e5757df7-c86a-4835-84bc-7174af535d25"
	modelTag  = names.NewModelTag(modelUUID)

	credentialID  = "cloud/user/credential"
	credentialTag = names.NewCloudCredentialTag(credentialID)
)

func (s *CredentialValidatorSuite) TestWatchModelCredentialError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{Error: &params.Error{Message: "foo"}}
		return nil
	})
	client := credentialvalidator.NewFacade(apitesting.BestVersionCaller{apiCaller, 2})
	_, err := client.WatchModelCredential(c.Context())
	c.Assert(err, tc.ErrorMatches, "foo")
}

func (s *CredentialValidatorSuite) TestWatchModelCredentialCallError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("foo")
	})

	client := credentialvalidator.NewFacade(apitesting.BestVersionCaller{apiCaller, 2})
	_, err := client.WatchModelCredential(c.Context())
	c.Assert(err, tc.ErrorMatches, "foo")
}
