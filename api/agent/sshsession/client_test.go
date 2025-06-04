// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/sshsession"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
	jujutesting "github.com/juju/juju/testing"
)

type sshsessionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&sshsessionSuite{})

func (s *sshsessionSuite) TestWatchSSHConnRequest(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SSHSession")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchSSHConnRequest")
		c.Assert(arg, gc.FitsTypeOf, params.SSHConnRequestWatchArg{MachineId: "1"})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}

		return nil
	})

	client := sshsession.NewClient(apiCaller)
	watcher, err := client.WatchSSHConnRequest("1")
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *sshsessionSuite) TestGetSSHConnRequest(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SSHSession")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSSHConnRequest")
		c.Assert(arg, gc.FitsTypeOf, params.SSHConnRequestGetArg{RequestId: "1"})
		c.Assert(result, gc.FitsTypeOf, &params.SSHConnRequestResult{})
		*(result.(*params.SSHConnRequestResult)) = params.SSHConnRequestResult{
			SSHConnRequest: params.SSHConnRequest{},
			Error:          &params.Error{Message: "FAIL"},
		}
		return nil
	})

	client := sshsession.NewClient(apiCaller)
	connReq, err := client.GetSSHConnRequest("1")
	c.Assert(connReq, jc.DeepEquals, params.SSHConnRequest{})
	c.Assert(err, gc.ErrorMatches, "FAIL")

	apiCaller = basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SSHSession")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSSHConnRequest")
		c.Assert(arg, gc.FitsTypeOf, params.SSHConnRequestGetArg{RequestId: "1"})
		c.Assert(result, gc.FitsTypeOf, &params.SSHConnRequestResult{})
		*(result.(*params.SSHConnRequestResult)) = params.SSHConnRequestResult{
			SSHConnRequest: params.SSHConnRequest{
				Username: "alice",
			},
		}
		return nil
	})

	client = sshsession.NewClient(apiCaller)
	connReq, err = client.GetSSHConnRequest("1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(connReq, jc.DeepEquals, params.SSHConnRequest{Username: "alice"})
}

func (s *sshsessionSuite) TestControllerSSHPort(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SSHSession")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ControllerSSHPort")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringResult{})
		*(result.(*params.StringResult)) = params.StringResult{
			Result: "17022",
		}

		return nil
	})

	client := sshsession.NewClient(apiCaller)
	result, err := client.ControllerSSHPort()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "17022")
}

func (s *sshsessionSuite) TestControllerPublicKey(c *gc.C) {
	testKey := jujutesting.SSHServerHostKey
	signer, err := ssh.ParsePrivateKey([]byte(testKey))
	c.Assert(err, gc.IsNil)

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SSHSession")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ControllerPublicKey")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.ControllerSSHPublicKeyResult{})
		*(result.(*params.ControllerSSHPublicKeyResult)) = params.ControllerSSHPublicKeyResult{
			PublicKey: signer.PublicKey().Marshal(),
		}

		return nil
	})

	client := sshsession.NewClient(apiCaller)
	result, err := client.ControllerPublicKey()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, signer.PublicKey())
}

func (s *sshsessionSuite) TestControllerPublicKeyInvalidKey(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(request, gc.Equals, "ControllerPublicKey")
		c.Assert(result, gc.FitsTypeOf, &params.ControllerSSHPublicKeyResult{})
		*(result.(*params.ControllerSSHPublicKeyResult)) = params.ControllerSSHPublicKeyResult{
			PublicKey: []byte{0, 1, 2, 3}, // Invalid key
		}

		return nil
	})

	client := sshsession.NewClient(apiCaller)
	_, err := client.ControllerPublicKey()
	c.Assert(err, gc.NotNil)
}
