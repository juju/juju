// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/externalcontrollerupdater"
	"github.com/juju/juju/core/crossmodel"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&ExternalControllerUpdaterSuite{})

type ExternalControllerUpdaterSuite struct {
	coretesting.BaseSuite
}

func (s *ExternalControllerUpdaterSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := externalcontrollerupdater.New(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *ExternalControllerUpdaterSuite) TestExternalControllerInfo(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "ExternalControllerUpdater")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ExternalControllerInfo")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: coretesting.ControllerTag.String()}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ExternalControllerInfoResults{})
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
	info, err := client.ExternalControllerInfo(context.Background(), coretesting.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &crossmodel.ControllerInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Alias:          "foo",
		Addrs:          []string{"bar"},
		CACert:         "baz",
	})
}

func (s *ExternalControllerUpdaterSuite) TestExternalControllerInfoError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ExternalControllerInfoResults)) = params.ExternalControllerInfoResults{
			Results: []params.ExternalControllerInfoResult{{
				Error: &params.Error{Code: params.CodeNotFound},
			}},
		}
		return nil
	})
	client := externalcontrollerupdater.New(apiCaller)
	info, err := client.ExternalControllerInfo(context.Background(), coretesting.ControllerTag.Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(info, gc.IsNil)
}

func (s *ExternalControllerUpdaterSuite) TestSetExternalControllerInfo(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "ExternalControllerUpdater")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetExternalControllerInfo")
		c.Check(arg, jc.DeepEquals, params.SetExternalControllersInfoParams{
			Controllers: []params.SetExternalControllerInfoParams{{
				Info: params.ExternalControllerInfo{
					ControllerTag: coretesting.ControllerTag.String(),
					Alias:         "foo",
					Addrs:         []string{"bar"},
					CACert:        "baz",
				},
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := externalcontrollerupdater.New(apiCaller)
	err := client.SetExternalControllerInfo(context.Background(), crossmodel.ControllerInfo{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Alias:          "foo",
		Addrs:          []string{"bar"},
		CACert:         "baz",
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ExternalControllerUpdaterSuite) TestWatchExternalControllers(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "ExternalControllerUpdater")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchExternalControllers")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := externalcontrollerupdater.New(apiCaller)
	w, err := client.WatchExternalControllers(context.Background())
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(w, gc.IsNil)
}
