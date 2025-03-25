// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/sshserver"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/rpc/params"
)

type sshserverSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&sshserverSuite{})

func newClient(f basetesting.APICallerFunc) (*sshserver.Client, error) {
	return sshserver.NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: 1})
}

func (s *sshserverSuite) TestControllerConfig(c *gc.C) {
	client, err := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SSHServer")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ControllerConfig")
			c.Assert(arg, gc.IsNil)
			c.Assert(result, gc.FitsTypeOf, &params.ControllerConfigResult{})

			*(result.(*params.ControllerConfigResult)) = params.ControllerConfigResult{
				Config: params.ControllerConfig{
					"ssh-server-port":                96,
					"ssh-max-concurrent-connections": 96,
				},
			}
			return nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := client.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		cfg,
		jc.DeepEquals,
		controller.Config{
			"ssh-server-port":                96,
			"ssh-max-concurrent-connections": 96,
		},
	)
}

func (s *sshserverSuite) TestSSHServerHostKey(c *gc.C) {
	client, err := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SSHServer")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SSHServerHostKey")
			c.Assert(arg, gc.IsNil)
			c.Assert(result, gc.FitsTypeOf, &params.StringResult{})

			*(result.(*params.StringResult)) = params.StringResult{
				Result: "key",
			}
			return nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	key, err := client.SSHServerHostKey()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, gc.Equals, "key")
}

func (s *sshserverSuite) TestSSHServerHostKeyError(c *gc.C) {
	client, err := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SSHServer")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SSHServerHostKey")
			c.Assert(arg, gc.IsNil)
			c.Assert(result, gc.FitsTypeOf, &params.StringResult{})

			*(result.(*params.StringResult)) = params.StringResult{
				Result: "",
				Error:  apiservererrors.ServerError(errors.New("blah")),
			}
			return nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = client.SSHServerHostKey()
	c.Assert(err, gc.ErrorMatches, "blah")
}

func (s *sshserverSuite) TestHostKeyForTarget(c *gc.C) {
	client, err := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SSHServer")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "HostKeyForTarget")
			c.Assert(arg, gc.FitsTypeOf, params.SSHHostKeyRequestArg{})
			c.Assert(result, gc.FitsTypeOf, &params.SSHHostKeyResult{})

			*(result.(*params.SSHHostKeyResult)) = params.SSHHostKeyResult{
				HostKey: []byte("key"),
			}
			return nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	key, err := client.HostKeyForTarget(params.SSHHostKeyRequestArg{Hostname: "host"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, gc.DeepEquals, []byte("key"))
}
