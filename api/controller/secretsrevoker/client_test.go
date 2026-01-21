// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/secretsrevoker"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&SecretRevokerSuite{})

type SecretRevokerSuite struct {
	coretesting.BaseSuite
}

func (s *SecretRevokerSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		return nil
	})
	client := secretsrevoker.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *SecretRevokerSuite) TestWatchIssuedTokenExpiry(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Check(objType, gc.Equals, "SecretsRevoker")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchIssuedTokenExpiry")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := secretsrevoker.NewClient(apiCaller)
	_, err := client.WatchIssuedTokenExpiry()
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SecretRevokerSuite) TestRevokeIssuedTokens(c *gc.C) {
	now := time.Now()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Check(objType, gc.Equals, "SecretsRevoker")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RevokeIssuedTokens")
		c.Check(arg, jc.DeepEquals, now)
		c.Assert(result, gc.FitsTypeOf, &params.RevokeIssuedTokensResult{})
		*(result.(*params.RevokeIssuedTokensResult)) = params.RevokeIssuedTokensResult{
			Error: &params.Error{Message: "boom"},
		}
		return nil
	})
	client := secretsrevoker.NewClient(apiCaller)
	next, err := client.RevokeIssuedTokens(now)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Check(next, gc.Equals, time.Time{})
}

func (s *SecretRevokerSuite) TestRevokeIssuedTokensWithResult(c *gc.C) {
	now := time.Now()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Check(objType, gc.Equals, "SecretsRevoker")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RevokeIssuedTokens")
		c.Check(arg, jc.DeepEquals, now)
		c.Assert(result, gc.FitsTypeOf, &params.RevokeIssuedTokensResult{})
		*(result.(*params.RevokeIssuedTokensResult)) = params.RevokeIssuedTokensResult{
			Next: now,
		}
		return nil
	})
	client := secretsrevoker.NewClient(apiCaller)
	next, err := client.RevokeIssuedTokens(now)
	c.Assert(err, gc.IsNil)
	c.Check(next, gc.Equals, now)
}
