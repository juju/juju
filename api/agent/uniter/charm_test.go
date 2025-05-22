// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type charmSuite struct {
	coretesting.BaseSuite
}

func TestCharmSuite(t *stdtesting.T) {
	tc.Run(t, &charmSuite{})
}

func (s *charmSuite) TestCharmWithNilFails(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := client.Charm("")
	c.Assert(err, tc.ErrorMatches, "charm url cannot be empty")
}

func (s *charmSuite) TestCharm(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	curl := "ch:mysql"
	ch, err := client.Charm(curl)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.URL(), tc.DeepEquals, curl)
}

func (s *charmSuite) TestArchiveSha256(c *tc.C) {
	curl := "ch:mysql"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "CharmArchiveSha256")
		c.Assert(arg, tc.DeepEquals, params.CharmURLs{
			URLs: []params.CharmURL{{URL: curl}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "deadbeef",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	ch, err := client.Charm(curl)
	c.Assert(err, tc.ErrorIsNil)
	sha, err := ch.ArchiveSha256(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sha, tc.Equals, "deadbeef")
}

func (s *charmSuite) TestLXDProfileRequired(c *tc.C) {
	curl := "ch:mysql"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "LXDProfileRequired")
		c.Assert(arg, tc.DeepEquals, params.CharmURLs{
			URLs: []params.CharmURL{{URL: curl}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.BoolResults{})
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{
				Result: true,
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	ch, err := client.Charm(curl)
	c.Assert(err, tc.ErrorIsNil)
	required, err := ch.LXDProfileRequired(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(required, tc.IsTrue)
}
