// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/credentialvalidator"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&CredentialValidatorSuite{})

type CredentialValidatorSuite struct {
	testing.IsolationSuite
}

func (s *CredentialValidatorSuite) TestModelCredential(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CredentialValidator")
		c.Check(request, gc.Equals, "ModelCredential")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.ModelCredential{})
		*(result.(*params.ModelCredential)) = params.ModelCredential{
			Model:           modelTag.String(),
			CloudCredential: credentialTag.String(),
			Exists:          true,
			Valid:           true,
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	found, exists, err := client.ModelCredential(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsTrue)
	c.Assert(found, gc.DeepEquals, base.StoredCredential{CloudCredential: "cloud/user/credential", Valid: true})
}

func (s *CredentialValidatorSuite) TestModelCredentialIsNotNeeded(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ModelCredential)) = params.ModelCredential{
			Model:  modelTag.String(),
			Exists: false,
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, exists, err := client.ModelCredential(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsFalse)
}

func (s *CredentialValidatorSuite) TestModelCredentialInvalidCredentialTag(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ModelCredential)) = params.ModelCredential{
			Model:           modelTag.String(),
			Exists:          true,
			CloudCredential: "some-invalid-cloud-credential-tag-as-string",
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, exists, err := client.ModelCredential(context.Background())
	c.Assert(err, gc.ErrorMatches, `"some-invalid-cloud-credential-tag-as-string" is not a valid tag`)
	c.Assert(exists, jc.IsFalse)
}

func (s *CredentialValidatorSuite) TestModelCredentialCallError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("foo")
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, _, err := client.ModelCredential(context.Background())
	c.Assert(err, gc.ErrorMatches, "foo")
}

var (
	modelUUID = "e5757df7-c86a-4835-84bc-7174af535d25"
	modelTag  = names.NewModelTag(modelUUID)

	credentialID  = "cloud/user/credential"
	credentialTag = names.NewCloudCredentialTag(credentialID)
)

func (s *CredentialValidatorSuite) TestWatchModelCredentialError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{Error: &params.Error{Message: "foo"}}
		return nil
	})
	client := credentialvalidator.NewFacade(apitesting.BestVersionCaller{apiCaller, 2})
	_, err := client.WatchModelCredential(context.Background())
	c.Assert(err, gc.ErrorMatches, "foo")
}

func (s *CredentialValidatorSuite) TestWatchModelCredentialCallError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("foo")
	})

	client := credentialvalidator.NewFacade(apitesting.BestVersionCaller{apiCaller, 2})
	_, err := client.WatchModelCredential(context.Background())
	c.Assert(err, gc.ErrorMatches, "foo")
}
