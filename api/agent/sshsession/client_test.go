// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/sshsession"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
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
		c.Assert(arg, jc.DeepEquals, "1")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
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
		c.Assert(arg, jc.DeepEquals, "1")
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
		c.Assert(arg, jc.DeepEquals, "1")
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
