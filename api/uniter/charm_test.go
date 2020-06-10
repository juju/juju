// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type charmSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&charmSuite{})

func (s *charmSuite) TestCharmWithNilFails(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	_, err := client.Charm(nil)
	c.Assert(err, gc.ErrorMatches, "charm url cannot be nil")
}

func (s *charmSuite) TestCharm(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	curl := charm.MustParseURL("mysql")
	ch, err := client.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), jc.DeepEquals, curl)
	c.Assert(ch.String(), gc.Equals, curl.String())
}

func (s *charmSuite) TestArchiveSha256(c *gc.C) {
	curl := charm.MustParseURL("mysql")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "CharmArchiveSha256")
		c.Assert(arg, jc.DeepEquals, params.CharmURLs{
			URLs: []params.CharmURL{{URL: curl.String()}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "deadbeef",
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	ch, err := client.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	sha, err := ch.ArchiveSha256()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sha, gc.Equals, "deadbeef")
}

func (s *charmSuite) TestLXDProfileRequired(c *gc.C) {
	curl := charm.MustParseURL("mysql")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "LXDProfileRequired")
		c.Assert(arg, jc.DeepEquals, params.CharmURLs{
			URLs: []params.CharmURL{{URL: curl.String()}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.BoolResults{})
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{
				Result: true,
			}},
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	ch, err := client.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	required, err := ch.LXDProfileRequired()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(required, jc.IsTrue)
}
