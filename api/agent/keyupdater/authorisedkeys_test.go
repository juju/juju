// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/keyupdater"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type keyupdaterSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&keyupdaterSuite{})

func (s *keyupdaterSuite) TestAuthorisedKeys(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "KeyUpdater")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "AuthorisedKeys")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsResults{})
		*(result.(*params.StringsResults)) = params.StringsResults{
			Results: []params.StringsResult{{
				Result: []string{"key1", "key2"},
			}},
		}
		return nil
	})
	tag := names.NewMachineTag("666")
	client := keyupdater.NewClient(apiCaller)
	keys, err := client.AuthorisedKeys(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.DeepEquals, []string{"key1", "key2"})
}

func (s *keyupdaterSuite) TestWatchAuthorisedKeys(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "KeyUpdater")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchAuthorisedKeys")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}}}
		return nil
	})
	tag := names.NewMachineTag("666")
	client := keyupdater.NewClient(apiCaller)
	_, err := client.WatchAuthorisedKeys(tag)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}
