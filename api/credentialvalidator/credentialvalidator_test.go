// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/credentialvalidator"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CredentialValidatorSuite{})

type CredentialValidatorSuite struct {
	coretesting.BaseSuite
}

func (s *CredentialValidatorSuite) TestModelCredential(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CredentialValidator")
		c.Check(request, gc.Equals, "ModelCredentials")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: modelTag.String()}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ModelCredentialResults{})
		*(result.(*params.ModelCredentialResults)) = params.ModelCredentialResults{
			Results: []params.ModelCredentialResult{
				{Result: &params.ModelCredential{
					Model:           modelTag.String(),
					CloudCredential: credentialTag.String(),
					Exists:          true,
					Valid:           true,
				}},
			},
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	found, exists, err := client.ModelCredential(modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsTrue)
	c.Assert(found, gc.Equals, base.StoredCredential{CloudCredential: "cloud/user/credential", Valid: true})
}

func (s *CredentialValidatorSuite) TestModelCredentialIsNotNeeded(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ModelCredentialResults)) = params.ModelCredentialResults{
			Results: []params.ModelCredentialResult{
				{Result: &params.ModelCredential{
					Model:  modelTag.String(),
					Exists: false,
				}},
			},
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, exists, err := client.ModelCredential(modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsFalse)
}

func (s *CredentialValidatorSuite) TestModelCredentialManyResults(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ModelCredentialResults)) = params.ModelCredentialResults{
			Results: []params.ModelCredentialResult{
				{Result: &params.ModelCredential{}},
				{Result: &params.ModelCredential{}},
			},
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, exists, err := client.ModelCredential(modelUUID)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`expected 1 model credential for model %q, got 2`, modelUUID))
	c.Assert(exists, jc.IsFalse)
}

func (s *CredentialValidatorSuite) TestModelCredentialForWrongModel(c *gc.C) {
	diffUUID := "d5757ef7-c86a-4835-84bc-7174af535e25"

	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ModelCredentialResults)) = params.ModelCredentialResults{
			Results: []params.ModelCredentialResult{
				{Result: &params.ModelCredential{
					// different model UUID than the one supplied to the call
					Model: names.NewModelTag(diffUUID).String(),
				}},
			},
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, exists, err := client.ModelCredential(modelUUID)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`unexpected credential for model %q, expected credential for model %q`, diffUUID, modelUUID))
	c.Assert(exists, jc.IsFalse)
}

func (s *CredentialValidatorSuite) TestModelCredentialInvalidModelTag(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ModelCredentialResults)) = params.ModelCredentialResults{
			Results: []params.ModelCredentialResult{
				{Result: &params.ModelCredential{
					Model: "some-invalid-string-for-uuid",
				}},
			},
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, exists, err := client.ModelCredential(modelUUID)
	c.Assert(err, gc.ErrorMatches, `"some-invalid-string-for-uuid" is not a valid tag`)
	c.Assert(exists, jc.IsFalse)
}

func (s *CredentialValidatorSuite) TestModelCredentialInvalidCredentialTag(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ModelCredentialResults)) = params.ModelCredentialResults{
			Results: []params.ModelCredentialResult{
				{Result: &params.ModelCredential{
					Model:           modelTag.String(),
					Exists:          true,
					CloudCredential: "some-invalid-cloud-credential-tag-as-string",
				}},
			},
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, exists, err := client.ModelCredential(modelUUID)
	c.Assert(err, gc.ErrorMatches, `"some-invalid-cloud-credential-tag-as-string" is not a valid tag`)
	c.Assert(exists, jc.IsFalse)
}

func (s *CredentialValidatorSuite) TestModelCredentialError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ModelCredentialResults)) = params.ModelCredentialResults{
			Results: []params.ModelCredentialResult{
				{Error: &params.Error{Message: "foo"}}},
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, exists, err := client.ModelCredential(modelUUID)
	c.Assert(err, gc.ErrorMatches, "foo")
	c.Assert(exists, jc.IsFalse)
}

func (s *CredentialValidatorSuite) TestModelCredentialCallError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("foo")
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, _, err := client.ModelCredential(modelUUID)
	c.Assert(err, gc.ErrorMatches, "foo")
}

func (s *CredentialValidatorSuite) TestWatchCredential(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CredentialValidator")
		c.Check(request, gc.Equals, "WatchCredential")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: credentialTag.String()}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{
				{NotifyWatcherId: "notify-watcher-id"},
			},
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	found, err := client.WatchCredential(credentialID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.NotNil)
}

func (s *CredentialValidatorSuite) TestWatchCredentialTooMany(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{
				{},
				{},
			},
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, err := client.WatchCredential(credentialID)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("expected 1 watcher for credential %q, got 2", credentialID))
}

func (s *CredentialValidatorSuite) TestWatchCredentialError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{
				{Error: &params.Error{Message: "foo"}}},
		}
		return nil
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, err := client.WatchCredential(credentialID)
	c.Assert(err, gc.ErrorMatches, "foo")
}

func (s *CredentialValidatorSuite) TestWatchCredentialCallError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("foo")
	})

	client := credentialvalidator.NewFacade(apiCaller)
	_, err := client.WatchCredential(credentialID)
	c.Assert(err, gc.ErrorMatches, "foo")
}

var (
	modelUUID = "e5757df7-c86a-4835-84bc-7174af535d25"
	modelTag  = names.NewModelTag(modelUUID)

	credentialID  = "cloud/user/credential"
	credentialTag = names.NewCloudCredentialTag(credentialID)
)
