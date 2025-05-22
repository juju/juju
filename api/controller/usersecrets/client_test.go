// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/usersecrets"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestSecretSuite(t *stdtesting.T) {
	tc.Run(t, &secretSuite{})
}

type secretSuite struct {
	coretesting.BaseSuite
}

func (s *secretSuite) TestNewClient(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := usersecrets.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func (s *secretSuite) TestWatchRevisionsToPrune(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "UserSecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchRevisionsToPrune")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := usersecrets.NewClient(apiCaller)
	_, err := client.WatchRevisionsToPrune(c.Context())
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *secretSuite) TestDeleteObsoleteUserSecretRevisions(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "UserSecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "DeleteObsoleteUserSecretRevisions")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.IsNil)
		return errors.New("boom")
	})
	client := usersecrets.NewClient(apiCaller)
	err := client.DeleteObsoleteUserSecretRevisions(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}
