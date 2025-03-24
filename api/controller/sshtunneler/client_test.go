// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type sshTunnelerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&sshTunnelerSuite{})

func newClient(f basetesting.APICallerFunc) *Client {
	return NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: 1})
}

func (s *sshTunnelerSuite) TestControllerAddresses(c *gc.C) {
	entity := names.NewMachineTag("1")

	client := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SSHTunneler")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ControllerAddresses")
			c.Assert(arg, gc.DeepEquals, params.Entity{Tag: entity.String()})
			c.Assert(result, gc.FitsTypeOf, &params.StringsResult{})

			*(result.(*params.StringsResult)) = params.StringsResult{
				Result: []string{"1.2.3.4"},
			}
			return nil
		},
	)

	addresses, err := client.ControllerAddresses(entity)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		addresses,
		jc.DeepEquals,
		network.SpaceAddresses{network.NewSpaceAddress("1.2.3.4")},
	)
}

func (s *sshTunnelerSuite) TestControllerAddressesError(c *gc.C) {
	entity := names.NewMachineTag("1")

	client := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SSHTunneler")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ControllerAddresses")
			c.Assert(arg, gc.DeepEquals, params.Entity{Tag: entity.String()})
			c.Assert(result, gc.FitsTypeOf, &params.StringsResult{})

			*(result.(*params.StringsResult)) = params.StringsResult{
				Error: &params.Error{Message: "my-error"},
			}
			return nil
		},
	)

	_, err := client.ControllerAddresses(entity)
	c.Assert(err, gc.ErrorMatches, "my-error")
}

func (s *sshTunnelerSuite) TestInsertSSHConnRequest(c *gc.C) {
	client := newClient(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SSHTunneler")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "InsertSSHConnRequest")
			c.Assert(arg, gc.DeepEquals, params.SSHConnRequestArg{
				Username: "ubuntu",
				Password: "foo",
			})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResult{})

			*(result.(*params.ErrorResult)) = params.ErrorResult{
				Error: nil,
			}
			return nil
		},
	)

	req := state.SSHConnRequestArg{
		Username: "ubuntu",
		Password: "foo",
	}
	err := client.InsertSSHConnRequest(req)
	c.Assert(err, jc.ErrorIsNil)
}
