// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsbackendmanager_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/secretsbackendmanager"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&SecretBackendsSuite{})

type SecretBackendsSuite struct {
	coretesting.BaseSuite
}

func (s *SecretBackendsSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := secretsbackendmanager.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *SecretBackendsSuite) TestWatchSecretsRotationChanges(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretBackendsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchSecretBackendsRotateChanges")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.SecretBackendRotateWatchResult{})
		*(result.(*params.SecretBackendRotateWatchResult)) = params.SecretBackendRotateWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := secretsbackendmanager.NewClient(apiCaller)
	_, err := client.WatchTokenRotationChanges()
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SecretBackendsSuite) TestRotateBackendTokens(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretBackendsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RotateBackendTokens")
		c.Check(arg, jc.DeepEquals, params.RotateSecretBackendArgs{
			BackendIDs: []string{"backend-id"},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			[]params.ErrorResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := secretsbackendmanager.NewClient(apiCaller)
	err := client.RotateBackendTokens("backend-id")
	c.Assert(err, gc.ErrorMatches, "boom")
}
