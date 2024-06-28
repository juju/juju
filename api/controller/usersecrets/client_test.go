// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets_test

import (
	"context"

	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/usersecrets"
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
	client := usersecrets.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *secretSuite) TestWatchRevisionsToPrune(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "UserSecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchRevisionsToPrune")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := usersecrets.NewClient(apiCaller)
	_, err := client.WatchRevisionsToPrune()
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *secretSuite) TestDeleteObsoleteUserSecretRevisions(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "UserSecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "DeleteObsoleteUserSecretRevisions")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.IsNil)
		return errors.New("boom")
	})
	client := usersecrets.NewClient(apiCaller)
	err := client.DeleteObsoleteUserSecretRevisions(context.Background())
	c.Assert(err, gc.ErrorMatches, "boom")
}
