// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsbackendmanager_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/secretsbackendmanager"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestSecretBackendsSuite(t *stdtesting.T) { tc.Run(t, &SecretBackendsSuite{}) }

type SecretBackendsSuite struct {
	coretesting.BaseSuite
}

func (s *SecretBackendsSuite) TestNewClient(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := secretsbackendmanager.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func (s *SecretBackendsSuite) TestWatchSecretsRotationChanges(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretBackendsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchSecretBackendsRotateChanges")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.SecretBackendRotateWatchResult{})
		*(result.(*params.SecretBackendRotateWatchResult)) = params.SecretBackendRotateWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := secretsbackendmanager.NewClient(apiCaller)
	_, err := client.WatchTokenRotationChanges(c.Context())
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *SecretBackendsSuite) TestRotateBackendTokens(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretBackendsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RotateBackendTokens")
		c.Check(arg, tc.DeepEquals, params.RotateSecretBackendArgs{
			BackendIDs: []string{"backend-id"},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := secretsbackendmanager.NewClient(apiCaller)
	err := client.RotateBackendTokens(c.Context(), "backend-id")
	c.Assert(err, tc.ErrorMatches, "boom")
}
