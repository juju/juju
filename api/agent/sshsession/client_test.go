// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/sshsession"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/internal/errors"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type clientSuite struct {
	coretesting.BaseSuite
}

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &clientSuite{})
}

func (s *clientSuite) TestGetSSHConnRequest(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Check(objType, tc.Equals, "SSHSession")
		c.Check(request, tc.Equals, "GetSSHConnRequest")
		c.Check(arg, tc.DeepEquals, params.SSHConnRequestArg{TunnelID: "tunnel-0"})
		c.Assert(result, tc.FitsTypeOf, &params.SSHConnRequestResult{})
		*(result.(*params.SSHConnRequestResult)) = params.SSHConnRequestResult{
			MachineName:         "0",
			ControllerAddresses: []string{"10.0.0.1"},
			Username:            "juju-reverse-tunnel",
			Password:            "jwt",
			UnitPort:            22,
			EphemeralPublicKey:  []byte("eph-pub"),
		}
		return nil
	})

	client := sshsession.NewClient(apiCaller)
	res, err := client.GetSSHConnRequest(c.Context(), "tunnel-0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.MachineName, tc.Equals, "0")
	c.Check(res.ControllerAddresses, tc.DeepEquals, []string{"10.0.0.1"})
	c.Check(res.EphemeralPublicKey, tc.DeepEquals, []byte("eph-pub"))
}

func (s *clientSuite) TestControllerSSHPort(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Check(objType, tc.Equals, "SSHSession")
		c.Check(request, tc.Equals, "ControllerSSHPort")
		c.Assert(result, tc.FitsTypeOf, &params.SSHControllerSSHPortResult{})
		*(result.(*params.SSHControllerSSHPortResult)) = params.SSHControllerSSHPortResult{Port: 2223}
		return nil
	})

	client := sshsession.NewClient(apiCaller)
	port, err := client.ControllerSSHPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(port, tc.Equals, 2223)
}

func (s *clientSuite) TestControllerPublicKey(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Check(objType, tc.Equals, "SSHSession")
		c.Check(request, tc.Equals, "ControllerPublicKey")
		c.Assert(result, tc.FitsTypeOf, &params.SSHControllerPublicKeyResult{})
		*(result.(*params.SSHControllerPublicKeyResult)) = params.SSHControllerPublicKeyResult{PublicKey: []byte("host-pub")}
		return nil
	})

	client := sshsession.NewClient(apiCaller)
	key, err := client.ControllerPublicKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.DeepEquals, []byte("host-pub"))
}

func (s *clientSuite) TestGetSSHConnRequestError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		*(result.(*params.SSHConnRequestResult)) = params.SSHConnRequestResult{
			Error: &params.Error{Message: "boom"},
		}
		return nil
	})

	client := sshsession.NewClient(apiCaller)
	_, err := client.GetSSHConnRequest(c.Context(), "tunnel-0")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *clientSuite) TestGetSSHConnRequestFacadeError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		return errors.New("transport boom")
	})

	client := sshsession.NewClient(apiCaller)
	_, err := client.GetSSHConnRequest(c.Context(), "tunnel-0")
	c.Assert(err, tc.ErrorMatches, "transport boom")
}
