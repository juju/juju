// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/machineundertaker"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type undertakerSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&undertakerSuite{})

func (s *undertakerSuite) TestRequiresModelConnection(c *tc.C) {
	api, err := machineundertaker.NewAPI(&fakeAPICaller{hasModelTag: false}, nil)
	c.Assert(err, tc.ErrorMatches, "machine undertaker client requires a model API connection")
	c.Assert(api, tc.IsNil)
	api, err = machineundertaker.NewAPI(&fakeAPICaller{hasModelTag: true}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api, tc.NotNil)
}

func (s *undertakerSuite) TestAllMachineRemovals(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, tc.Equals, "MachineUndertaker")
		c.Check(request, tc.Equals, "AllMachineRemovals")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(arg, tc.DeepEquals, wrapEntities(coretesting.ModelTag.String()))
		c.Assert(result, tc.FitsTypeOf, &params.EntitiesResults{})
		*result.(*params.EntitiesResults) = *wrapEntitiesResults("machine-23", "machine-42")
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []names.MachineTag{
		names.NewMachineTag("23"),
		names.NewMachineTag("42"),
	})
}

func (s *undertakerSuite) TestAllMachineRemovals_Error(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		return errors.New("restless year")
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals(context.Background())
	c.Assert(err, tc.ErrorMatches, "restless year")
	c.Assert(results, tc.IsNil)
}

func (s *undertakerSuite) TestAllMachineRemovals_BadTag(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.EntitiesResults{})
		*result.(*params.EntitiesResults) = *wrapEntitiesResults("machine-23", "application-burp")
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals(context.Background())
	c.Assert(err, tc.ErrorMatches, `"application-burp" is not a valid machine tag`)
	c.Assert(results, tc.IsNil)
}

func (s *undertakerSuite) TestAllMachineRemovals_ErrorResult(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.EntitiesResults{})
		*result.(*params.EntitiesResults) = params.EntitiesResults{
			Results: []params.EntitiesResult{{
				Error: apiservererrors.ServerError(errors.New("everythingisterrible")),
			}},
		}
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals(context.Background())
	c.Assert(err, tc.ErrorMatches, "everythingisterrible")
	c.Assert(results, tc.IsNil)
}

func (s *undertakerSuite) TestAllMachineRemovals_TooManyResults(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.EntitiesResults{})
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
	results, err := api.AllMachineRemovals(context.Background())
	c.Assert(err, tc.ErrorMatches, "expected one result, got 2")
	c.Assert(results, tc.IsNil)
}

func (s *undertakerSuite) TestAllMachineRemovals_TooFewResults(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.EntitiesResults{})
		*result.(*params.EntitiesResults) = params.EntitiesResults{}
		return nil
	}
	api := makeAPI(c, caller)
	results, err := api.AllMachineRemovals(context.Background())
	c.Assert(err, tc.ErrorMatches, "expected one result, got 0")
	c.Assert(results, tc.IsNil)
}

func (*undertakerSuite) TestGetInfo(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, tc.Equals, "MachineUndertaker")
		c.Check(request, tc.Equals, "GetMachineProviderInterfaceInfo")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(arg, tc.DeepEquals, wrapEntities("machine-100"))
		c.Assert(result, tc.FitsTypeOf, &params.ProviderInterfaceInfoResults{})
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
	results, err := api.GetProviderInterfaceInfo(context.Background(), names.NewMachineTag("100"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []network.ProviderInterfaceInfo{{
		InterfaceName:   "hamster huey",
		HardwareAddress: "calvin",
		ProviderId:      "1234",
	}, {
		InterfaceName:   "happy hamster hop",
		HardwareAddress: "hobbes",
		ProviderId:      "1235",
	}})
}

func (*undertakerSuite) TestGetInfo_GenericError(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		return errors.New("gooey kablooey")
	}
	api := makeAPI(c, caller)
	results, err := api.GetProviderInterfaceInfo(context.Background(), names.NewMachineTag("100"))
	c.Assert(err, tc.ErrorMatches, "gooey kablooey")
	c.Assert(results, tc.IsNil)
}

func (*undertakerSuite) TestGetInfo_TooMany(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.ProviderInterfaceInfoResults{})
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
	results, err := api.GetProviderInterfaceInfo(context.Background(), names.NewMachineTag("100"))
	c.Assert(err, tc.ErrorMatches, "expected one result, got 2")
	c.Assert(results, tc.IsNil)
}

func (*undertakerSuite) TestGetInfo_BadMachine(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.ProviderInterfaceInfoResults{})
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
	results, err := api.GetProviderInterfaceInfo(context.Background(), names.NewMachineTag("100"))
	c.Assert(err, tc.ErrorMatches, "expected interface info for machine-100 but got machine-101")
	c.Assert(results, tc.IsNil)
}

func (*undertakerSuite) TestCompleteRemoval(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, tc.Equals, "MachineUndertaker")
		c.Check(request, tc.Equals, "CompleteMachineRemovals")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(arg, tc.DeepEquals, wrapEntities("machine-100"))
		c.Check(result, tc.DeepEquals, nil)
		return errors.New("gooey kablooey")
	}
	api := makeAPI(c, caller)
	err := api.CompleteRemoval(context.Background(), names.NewMachineTag("100"))
	c.Assert(err, tc.ErrorMatches, "gooey kablooey")
}

func (*undertakerSuite) TestWatchMachineRemovals_CallFailed(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, tc.Equals, "MachineUndertaker")
		c.Check(request, tc.Equals, "WatchMachineRemovals")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(arg, tc.DeepEquals, wrapEntities(coretesting.ModelTag.String()))
		return errors.New("oopsy")
	}
	api := makeAPI(c, caller)
	w, err := api.WatchMachineRemovals(context.Background())
	c.Check(w, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "oopsy")
}

func (*undertakerSuite) TestWatchMachineRemovals_ErrorInWatcher(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*result.(*params.NotifyWatchResults) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "blammo"},
			}},
		}
		return nil
	}
	api := makeAPI(c, caller)
	w, err := api.WatchMachineRemovals(context.Background())
	c.Check(w, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "blammo")
}

func (*undertakerSuite) TestWatchMachineRemovals_TooMany(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
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
	w, err := api.WatchMachineRemovals(context.Background())
	c.Check(w, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "expected one result, got 2")
}

func (*undertakerSuite) TestWatchMachineRemovals_Success(c *tc.C) {
	caller := func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*result.(*params.NotifyWatchResults) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "2",
			}},
		}
		return nil
	}
	expectWatcher := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(wcaller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(wcaller, tc.NotNil) // not comparable
		c.Check(result, tc.DeepEquals, params.NotifyWatchResult{
			NotifyWatcherId: "2",
		})
		return expectWatcher
	}

	api, err := machineundertaker.NewAPI(testing.APICallerFunc(caller), newWatcher)
	c.Check(err, jc.ErrorIsNil)
	w, err := api.WatchMachineRemovals(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(w, tc.Equals, expectWatcher)
}

func makeAPI(c *tc.C, caller testing.APICallerFunc) *machineundertaker.API {
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
