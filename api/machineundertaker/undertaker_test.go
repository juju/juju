// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/machineundertaker"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

type undertakerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&undertakerSuite{})

func (s *undertakerSuite) TestRequiresModelConnection(c *gc.C) {
	api, err := machineundertaker.NewAPI(&fakeAPICaller{hasModelTag: false}, nil)
	c.Assert(err, gc.ErrorMatches, "machine undertaker client requires a model API connection")
	c.Assert(api, gc.IsNil)
	api, err = machineundertaker.NewAPI(&fakeAPICaller{hasModelTag: true}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api, gc.NotNil)
}

func (s *undertakerSuite) TestAllMachineRemovals(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, gc.Equals, "MachineUndertaker")
		c.Check(request, gc.Equals, "AllMachineRemovals")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(arg, gc.DeepEquals, wrapEntities(coretesting.ModelTag.String()))
		c.Assert(result, gc.FitsTypeOf, &params.EntitiesResults{})
		*result.(*params.EntitiesResults) = *wrapEntitiesResults("machine-23", "machine-42")
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, []names.MachineTag{
		names.NewMachineTag("23"),
		names.NewMachineTag("42"),
	})
}

func (s *undertakerSuite) TestAllMachineRemovals_Error(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		return errors.New("restless year")
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals()
	c.Assert(err, gc.ErrorMatches, "restless year")
	c.Assert(results, gc.IsNil)
}

func (s *undertakerSuite) TestAllMachineRemovals_BadTag(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.EntitiesResults{})
		*result.(*params.EntitiesResults) = *wrapEntitiesResults("machine-23", "application-burp")
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals()
	c.Assert(err, gc.ErrorMatches, `"application-burp" is not a valid machine tag`)
	c.Assert(results, gc.IsNil)
}

func (s *undertakerSuite) TestAllMachineRemovals_ErrorResult(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.EntitiesResults{})
		*result.(*params.EntitiesResults) = params.EntitiesResults{
			Results: []params.EntitiesResult{{
				Error: common.ServerError(errors.New("everythingisterrible")),
			}},
		}
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals()
	c.Assert(err, gc.ErrorMatches, "everythingisterrible")
	c.Assert(results, gc.IsNil)
}

func (s *undertakerSuite) TestAllMachineRemovals_TooManyResults(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.EntitiesResults{})
		*result.(*params.EntitiesResults) = params.EntitiesResults{
			Results: []params.EntitiesResult{{
				Entities: []params.Entity{{Tag: "machine-1"}},
			}, {
				Entities: []params.Entity{{Tag: "machine-2"}},
			}},
		}
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals()
	c.Assert(err, gc.ErrorMatches, "expected one result, got 2")
	c.Assert(results, gc.IsNil)
}

func (s *undertakerSuite) TestAllMachineRemovals_TooFewResults(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.EntitiesResults{})
		*result.(*params.EntitiesResults) = params.EntitiesResults{}
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals()
	c.Assert(err, gc.ErrorMatches, "expected one result, got 0")
	c.Assert(results, gc.IsNil)
}

func (*undertakerSuite) TestGetInfo(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, gc.Equals, "MachineUndertaker")
		c.Check(request, gc.Equals, "GetMachineProviderInterfaceInfo")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(arg, gc.DeepEquals, wrapEntities("machine-100"))
		c.Assert(result, gc.FitsTypeOf, &params.ProviderInterfaceInfoResults{})
		*result.(*params.ProviderInterfaceInfoResults) = params.ProviderInterfaceInfoResults{
			Results: []params.ProviderInterfaceInfoResult{{
				MachineTag: "machine-100",
				Interfaces: []params.ProviderInterfaceInfo{{
					InterfaceName: "hamster huey",
					MACAddress:    "calvin",
					ProviderId:    "1234",
				}, {
					InterfaceName: "happy hamster hop",
					MACAddress:    "hobbes",
					ProviderId:    "1235",
				}},
			}},
		}
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.GetProviderInterfaceInfo(names.NewMachineTag("100"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, []network.ProviderInterfaceInfo{{
		InterfaceName: "hamster huey",
		MACAddress:    "calvin",
		ProviderId:    "1234",
	}, {
		InterfaceName: "happy hamster hop",
		MACAddress:    "hobbes",
		ProviderId:    "1235",
	}})
}

func (*undertakerSuite) TestGetInfo_GenericError(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		return errors.New("gooey kablooey")
	}
	api := makeAPI(c, caller)
	results, err := api.GetProviderInterfaceInfo(names.NewMachineTag("100"))
	c.Assert(err, gc.ErrorMatches, "gooey kablooey")
	c.Assert(results, gc.IsNil)
}

func (*undertakerSuite) TestGetInfo_TooMany(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.ProviderInterfaceInfoResults{})
		*result.(*params.ProviderInterfaceInfoResults) = params.ProviderInterfaceInfoResults{
			Results: []params.ProviderInterfaceInfoResult{{
				MachineTag: "machine-100",
				Interfaces: []params.ProviderInterfaceInfo{{
					InterfaceName: "hamster huey",
					MACAddress:    "calvin",
					ProviderId:    "1234",
				}},
			}, {
				MachineTag: "machine-101",
				Interfaces: []params.ProviderInterfaceInfo{{
					InterfaceName: "hamster huey",
					MACAddress:    "calvin",
					ProviderId:    "1234",
				}},
			}},
		}
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.GetProviderInterfaceInfo(names.NewMachineTag("100"))
	c.Assert(err, gc.ErrorMatches, "expected one result, got 2")
	c.Assert(results, gc.IsNil)
}

func (*undertakerSuite) TestGetInfo_BadMachine(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.ProviderInterfaceInfoResults{})
		*result.(*params.ProviderInterfaceInfoResults) = params.ProviderInterfaceInfoResults{
			Results: []params.ProviderInterfaceInfoResult{{
				MachineTag: "machine-101",
				Interfaces: []params.ProviderInterfaceInfo{{
					InterfaceName: "hamster huey",
					MACAddress:    "calvin",
					ProviderId:    "1234",
				}},
			}},
		}
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.GetProviderInterfaceInfo(names.NewMachineTag("100"))
	c.Assert(err, gc.ErrorMatches, "expected interface info for machine-100 but got machine-101")
	c.Assert(results, gc.IsNil)
}

func (*undertakerSuite) TestCompleteRemoval(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, gc.Equals, "MachineUndertaker")
		c.Check(request, gc.Equals, "CompleteMachineRemovals")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(arg, gc.DeepEquals, wrapEntities("machine-100"))
		c.Check(result, gc.DeepEquals, nil)
		return errors.New("gooey kablooey")
	}
	api := makeAPI(c, caller)
	err := api.CompleteRemoval(names.NewMachineTag("100"))
	c.Assert(err, gc.ErrorMatches, "gooey kablooey")
}

func (*undertakerSuite) TestWatchMachineRemovals_CallFailed(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, gc.Equals, "MachineUndertaker")
		c.Check(request, gc.Equals, "WatchMachineRemovals")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(arg, gc.DeepEquals, wrapEntities(coretesting.ModelTag.String()))
		return errors.New("oopsy")
	}
	api := makeAPI(c, caller)
	w, err := api.WatchMachineRemovals()
	c.Check(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "oopsy")
}

func (*undertakerSuite) TestWatchMachineRemovals_ErrorInWatcher(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*result.(*params.NotifyWatchResults) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "blammo"},
			}},
		}
		return nil
	}
	api := makeAPI(c, caller)
	w, err := api.WatchMachineRemovals()
	c.Check(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "blammo")
}

func (*undertakerSuite) TestWatchMachineRemovals_TooMany(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*result.(*params.NotifyWatchResults) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "2",
			}, {
				NotifyWatcherId: "3",
			}},
		}
		return nil
	}
	api := makeAPI(c, caller)
	w, err := api.WatchMachineRemovals()
	c.Check(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "expected one result, got 2")
}

func (*undertakerSuite) TestWatchMachineRemovals_Success(c *gc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*result.(*params.NotifyWatchResults) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "2",
			}},
		}
		return nil
	}
	expectWatcher := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(wcaller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(wcaller, gc.NotNil) // not comparable
		c.Check(result, gc.DeepEquals, params.NotifyWatchResult{
			NotifyWatcherId: "2",
		})
		return expectWatcher
	}

	api, err := machineundertaker.NewAPI(testing.APICallerFunc(caller), newWatcher)
	c.Check(err, jc.ErrorIsNil)
	w, err := api.WatchMachineRemovals()
	c.Check(err, jc.ErrorIsNil)
	c.Check(w, gc.Equals, expectWatcher)
}

func makeAPI(c *gc.C, caller testing.APICallerFunc) *machineundertaker.API {
	api, err := machineundertaker.NewAPI(caller, nil)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func wrapEntities(tags ...string) *params.Entities {
	return &params.Entities{Entities: makeEntitySlice(tags...)}
}

func makeEntitySlice(tags ...string) []params.Entity {
	results := make([]params.Entity, len(tags))
	for i := range tags {
		results[i].Tag = tags[i]
	}
	return results
}

func wrapEntitiesResults(tags ...string) *params.EntitiesResults {
	return &params.EntitiesResults{
		Results: []params.EntitiesResult{{
			Entities: makeEntitySlice(tags...),
		}},
	}
}

type fakeAPICaller struct {
	base.APICaller
	hasModelTag bool
}

func (c *fakeAPICaller) ModelTag() (names.ModelTag, bool) {
	return names.ModelTag{}, c.hasModelTag
}

func (c *fakeAPICaller) BestFacadeVersion(string) int {
	return 0
}
