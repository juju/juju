// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/externalcontrollerupdater"
	"github.com/juju/juju/core/crossmodel"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&ExternalControllerUpdaterSuite{})

type ExternalControllerUpdaterSuite struct {
	coretesting.BaseSuite
}

func (s *ExternalControllerUpdaterSuite) TestNewClient(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := externalcontrollerupdater.New(apiCaller)
	c.Assert(client, tc.NotNil)
}

func (s *ExternalControllerUpdaterSuite) TestExternalControllerInfo(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "ExternalControllerUpdater")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ExternalControllerInfo")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: coretesting.ControllerTag.String()}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ExternalControllerInfoResults{})
		*(result.(*params.ExternalControllerInfoResults)) = params.ExternalControllerInfoResults{
			Results: []params.ExternalControllerInfoResult{{
				Result: &params.ExternalControllerInfo{
					ControllerTag: coretesting.ControllerTag.String(),
					Alias:         "foo",
					Addrs:         []string{"bar"},
					CACert:        "baz",
				},
			}},
		}
		return nil
	})
	client := externalcontrollerupdater.New(apiCaller)
	info, err := client.ExternalControllerInfo(c.Context(), coretesting.ControllerTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &crossmodel.ControllerInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Alias:          "foo",
		Addrs:          []string{"bar"},
		CACert:         "baz",
	})
}

func (s *ExternalControllerUpdaterSuite) TestExternalControllerInfoError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ExternalControllerInfoResults)) = params.ExternalControllerInfoResults{
			Results: []params.ExternalControllerInfoResult{{
				Error: &params.Error{Code: params.CodeNotFound},
			}},
		}
		return nil
	})
	client := externalcontrollerupdater.New(apiCaller)
	info, err := client.ExternalControllerInfo(c.Context(), coretesting.ControllerTag.Id())
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(info, tc.IsNil)
}

func (s *ExternalControllerUpdaterSuite) TestSetExternalControllerInfo(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "ExternalControllerUpdater")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetExternalControllerInfo")
		c.Check(arg, tc.DeepEquals, params.SetExternalControllersInfoParams{
			Controllers: []params.SetExternalControllerInfoParams{{
				Info: params.ExternalControllerInfo{
					ControllerTag: coretesting.ControllerTag.String(),
					Alias:         "foo",
					Addrs:         []string{"bar"},
					CACert:        "baz",
				},
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := externalcontrollerupdater.New(apiCaller)
	err := client.SetExternalControllerInfo(c.Context(), crossmodel.ControllerInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Alias:          "foo",
		Addrs:          []string{"bar"},
		CACert:         "baz",
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllers(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "ExternalControllerUpdater")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchExternalControllers")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := externalcontrollerupdater.New(apiCaller)
	w, err := client.WatchExternalControllers(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
	c.Assert(w, tc.IsNil)
}
