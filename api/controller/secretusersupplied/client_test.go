// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretusersupplied_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/secretusersupplied"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&secretSuite{})

type secretSuite struct {
	coretesting.BaseSuite
}

func (s *secretSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := secretusersupplied.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *secretSuite) TestWatchObsoleteRevisionsNeedPrune(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretUserSuppliedManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchObsoleteRevisionsNeedPrune")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := secretusersupplied.NewClient(apiCaller)
	_, err := client.WatchObsoleteRevisionsNeedPrune()
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *secretSuite) TestDeleteRevisions(c *gc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretUserSuppliedManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "DeleteRevisions")
		c.Check(arg, jc.DeepEquals, params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{
				{
					URI:       uri.String(),
					Revisions: []int{1, 2, 3},
				},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			[]params.ErrorResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := secretusersupplied.NewClient(apiCaller)
	err := client.DeleteRevisions(uri, 1, 2, 3)
	c.Assert(err, gc.ErrorMatches, "boom")
}
