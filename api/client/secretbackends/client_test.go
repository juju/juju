// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/client/secretbackends"
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
	client := secretbackends.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *SecretBackendsSuite) TestListSecretBackends(c *gc.C) {
	config := map[string]interface{}{"foo": "bar"}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretBackends")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ListSecretBackends")
		c.Check(arg, jc.DeepEquals, params.ListSecretBackendsArgs{Reveal: true})
		c.Assert(result, gc.FitsTypeOf, &params.ListSecretBackendsResults{})
		*(result.(*params.ListSecretBackendsResults)) = params.ListSecretBackendsResults{
			[]params.SecretBackend{{
				Name:                "foo",
				Backend:             "vault",
				TokenRotateInterval: 666 * time.Minute,
				Config:              config,
			}},
		}
		return nil
	})
	client := secretbackends.NewClient(apiCaller)
	result, err := client.ListSecretBackends(true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []secretbackends.SecretBackend{{
		Name:                "foo",
		Backend:             "vault",
		TokenRotateInterval: 666 * time.Minute,
		Config:              config,
	}})
}

func (s *SecretBackendsSuite) TestAddSecretsBackend(c *gc.C) {
	backend := secretbackends.SecretBackend{
		Name:                "foo",
		Backend:             "vault",
		TokenRotateInterval: 666 * time.Minute,
		Config:              map[string]interface{}{"foo": "bar"},
	}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretBackends")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "AddSecretBackends")
		c.Check(arg, jc.DeepEquals, params.AddSecretBackendArgs{
			Args: []params.SecretBackend{{
				Name:                backend.Name,
				Backend:             backend.Backend,
				TokenRotateInterval: backend.TokenRotateInterval,
				Config:              backend.Config,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	client := secretbackends.NewClient(apiCaller)
	err := client.AddSecretBackend(backend)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}
