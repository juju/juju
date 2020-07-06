// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/credentialmanager"
	commonerrors "github.com/juju/juju/apiserver/common/errors"
	"github.com/juju/juju/apiserver/params"
)

var _ = gc.Suite(&CredentialManagerSuite{})

type CredentialManagerSuite struct {
	testing.IsolationSuite
}

func (s *CredentialManagerSuite) TestInvalidateModelCredential(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CredentialManager")
		c.Check(request, gc.Equals, "InvalidateModelCredential")
		c.Assert(arg, gc.Equals, params.InvalidateCredentialArg{Reason: "auth fail"})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResult{})
		*(result.(*params.ErrorResult)) = params.ErrorResult{}
		return nil
	})

	client := credentialmanager.NewClient(apiCaller)
	err := client.InvalidateModelCredential("auth fail")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CredentialManagerSuite) TestInvalidateModelCredentialBackendFailure(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResult)) = params.ErrorResult{Error: commonerrors.ServerError(errors.New("boom"))}
		return nil
	})

	client := credentialmanager.NewClient(apiCaller)
	err := client.InvalidateModelCredential("")
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *CredentialManagerSuite) TestInvalidateModelCredentialError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("foo")
	})

	client := credentialmanager.NewClient(apiCaller)
	err := client.InvalidateModelCredential("")
	c.Assert(err, gc.ErrorMatches, "foo")
}
