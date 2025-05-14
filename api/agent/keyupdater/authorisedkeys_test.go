// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/keyupdater"
	"github.com/juju/juju/api/base/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type keyupdaterSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&keyupdaterSuite{})

func (s *keyupdaterSuite) TestAuthorisedKeys(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "KeyUpdater")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "AuthorisedKeys")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsResults{})
		*(result.(*params.StringsResults)) = params.StringsResults{
			Results: []params.StringsResult{{
				Result: []string{"key1", "key2"},
			}},
		}
		return nil
	})
	tag := names.NewMachineTag("666")
	client := keyupdater.NewClient(apiCaller)
	keys, err := client.AuthorisedKeys(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys, tc.DeepEquals, []string{"key1", "key2"})
}

func (s *keyupdaterSuite) TestWatchAuthorisedKeys(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "KeyUpdater")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchAuthorisedKeys")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}}}
		return nil
	})
	tag := names.NewMachineTag("666")
	client := keyupdater.NewClient(apiCaller)
	_, err := client.WatchAuthorisedKeys(c.Context(), tag)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}
