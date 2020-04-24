// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type actionsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&actionsSuite{})

func (s *actionsSuite) TestTagToActionReceiverFn(c *gc.C) {
	stubActionReceiver := fakeActionReceiver{}
	stubEntity := fakeEntity{}
	tagToEntity := map[string]state.Entity{
		"unit-valid-0":   stubActionReceiver,
		"unit-invalid-0": stubEntity,
	}
	tagFn := common.TagToActionReceiverFn(makeFindEntity(tagToEntity))

	for i, test := range []struct {
		tag    string
		err    error
		result state.ActionReceiver
	}{{
		tag:    "unit-valid-0",
		result: stubActionReceiver,
	}, {
		tag: "unit-invalid-0",
		err: errors.NotImplementedf("action receiver interface on entity unit-invalid-0"),
	}, {
		tag: "unit-flustered-0",
		err: errors.NotFoundf("unit-flustered-0"),
	}, {
		tag: "notatag",
		err: errors.NotValidf("notatag"),
	}} {
		c.Logf("test %d", i)
		receiver, err := tagFn(test.tag)
		if test.err != nil {
			c.Check(err.Error(), gc.Equals, test.err.Error())
			c.Check(receiver, gc.IsNil)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(receiver, gc.Equals, test.result)
		}
	}
}

func (s *actionsSuite) TestAuthAndActionFromTagFn(c *gc.C) {
	notFoundActionTag := names.NewActionTag(utils.MustNewUUID().String())

	authorizedActionTag := names.NewActionTag(utils.MustNewUUID().String())
	authorizedMachineTag := names.NewMachineTag("1")
	authorizedAction := fakeAction{name: "action1", receiver: authorizedMachineTag.Id()}

	unauthorizedActionTag := names.NewActionTag(utils.MustNewUUID().String())
	unauthorizedMachineTag := names.NewMachineTag("10")
	unauthorizedAction := fakeAction{name: "action2", receiver: unauthorizedMachineTag.Id()}

	invalidReceiverActionTag := names.NewActionTag(utils.MustNewUUID().String())
	invalidReceiverAction := fakeAction{name: "action2", receiver: "masterexploder"}

	canAccess := makeCanAccess(map[names.Tag]bool{
		authorizedMachineTag: true,
	})
	getActionByTag := makeGetActionByTag(map[names.ActionTag]state.Action{
		authorizedActionTag:      authorizedAction,
		unauthorizedActionTag:    unauthorizedAction,
		invalidReceiverActionTag: invalidReceiverAction,
	})
	tagFn := common.AuthAndActionFromTagFn(canAccess, getActionByTag)

	for i, test := range []struct {
		tag            string
		errString      string
		err            error
		expectedAction state.Action
	}{{
		tag:       "invalid-action-tag",
		errString: `"invalid-action-tag" is not a valid tag`,
	}, {
		tag:       notFoundActionTag.String(),
		errString: "action not found",
	}, {
		tag:       invalidReceiverActionTag.String(),
		errString: `invalid actionreceiver name "masterexploder"`,
	}, {
		tag: unauthorizedActionTag.String(),
		err: common.ErrPerm,
	}, {
		tag:            authorizedActionTag.String(),
		expectedAction: authorizedAction,
	}} {
		c.Logf("test %d", i)
		action, err := tagFn(test.tag)
		if test.errString != "" {
			c.Check(err, gc.ErrorMatches, test.errString)
			c.Check(action, gc.IsNil)
		} else if test.err != nil {
			c.Check(err, gc.Equals, test.err)
			c.Check(action, gc.IsNil)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(action, gc.Equals, action)
		}
	}
}

func (s *actionsSuite) TestBeginActions(c *gc.C) {
	args := entities("success", "fail", "invalid")
	expectErr := errors.New("explosivo")
	actionFn := makeGetActionByTagString(map[string]state.Action{
		"success": fakeAction{},
		"fail":    fakeAction{beginErr: expectErr},
	})

	results := common.BeginActions(args, actionFn)

	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		[]params.ErrorResult{
			{},
			{common.ServerError(expectErr)},
			{common.ServerError(actionNotFoundErr)},
		},
	})
}

func (s *actionsSuite) TestGetActions(c *gc.C) {
	args := entities("success", "fail", "notPending")
	actionFn := makeGetActionByTagString(map[string]state.Action{
		"success":    fakeAction{name: "floosh", status: state.ActionPending},
		"notPending": fakeAction{status: state.ActionCancelled},
	})

	results := common.Actions(args, actionFn)

	c.Assert(results, jc.DeepEquals, params.ActionResults{
		[]params.ActionResult{
			{Action: &params.Action{Name: "floosh"}},
			{Error: common.ServerError(actionNotFoundErr)},
			{Error: common.ServerError(common.ErrActionNotAvailable)},
		},
	})
}

func (s *actionsSuite) TestFinishActions(c *gc.C) {
	args := params.ActionExecutionResults{
		[]params.ActionExecutionResult{
			{ActionTag: "success", Status: string(state.ActionCompleted)},
			{ActionTag: "notfound"},
			{ActionTag: "convertFail", Status: "failStatus"},
			{ActionTag: "finishFail", Status: string(state.ActionCancelled)},
		},
	}
	expectErr := errors.New("explosivo")
	actionFn := makeGetActionByTagString(map[string]state.Action{
		"success":     fakeAction{},
		"convertFail": fakeAction{},
		"finishFail":  fakeAction{finishErr: expectErr},
	})
	results := common.FinishActions(args, actionFn)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		[]params.ErrorResult{
			{},
			{common.ServerError(actionNotFoundErr)},
			{common.ServerError(errors.New("unrecognized action status 'failStatus'"))},
			{common.ServerError(expectErr)},
		},
	})
}

func (s *actionsSuite) TestWatchActionNotifications(c *gc.C) {
	args := entities("invalid-actionreceiver", "machine-1", "machine-2", "machine-3")
	canAccess := makeCanAccess(map[names.Tag]bool{
		names.NewMachineTag("2"): true,
		names.NewMachineTag("3"): true,
	})
	expectedStringsWatchResult := params.StringsWatchResult{
		StringsWatcherId: "orosu",
	}
	watchOne := makeWatchOne(map[names.Tag]params.StringsWatchResult{
		names.NewMachineTag("3"): expectedStringsWatchResult,
	})

	results := common.WatchActionNotifications(args, canAccess, watchOne)

	c.Assert(results, jc.DeepEquals, params.StringsWatchResults{
		[]params.StringsWatchResult{
			{Error: common.ServerError(errors.New(`invalid actionreceiver tag "invalid-actionreceiver"`))},
			{Error: common.ServerError(common.ErrPerm)},
			{Error: common.ServerError(errors.New("pax"))},
			{StringsWatcherId: "orosu"},
		},
	})
}

func (s *actionsSuite) TestWatchOneActionReceiverNotifications(c *gc.C) {
	expectErr := errors.New("zwoosh")
	registerFunc := func(facade.Resource) string { return "bambalam" }
	tagToActionReceiver := common.TagToActionReceiverFn(makeFindEntity(map[string]state.Entity{
		"machine-1": &fakeActionReceiver{watcher: &fakeWatcher{}},
		"machine-2": &fakeActionReceiver{watcher: &fakeWatcher{err: expectErr}},
	}))

	watchOneFn := common.WatchOneActionReceiverNotifications(tagToActionReceiver, registerFunc)

	for i, test := range []struct {
		tag       names.Tag
		err       string
		watcherId string
	}{{
		tag: names.NewMachineTag("0"),
		err: "machine-0 not found",
	}, {
		tag:       names.NewMachineTag("1"),
		watcherId: "bambalam",
	}, {
		tag: names.NewMachineTag("2"),
		err: "zwoosh",
	}} {
		c.Logf("test %d", i)
		c.Logf(test.tag.String())
		result, err := watchOneFn(test.tag)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
			c.Check(result, jc.DeepEquals, params.StringsWatchResult{})
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(result.StringsWatcherId, gc.Equals, test.watcherId)
		}
	}
}

func (s *actionsSuite) TestWatchPendingActionsForReceiver(c *gc.C) {
	expectErr := errors.New("zwoosh")
	registerFunc := func(facade.Resource) string { return "bambalam" }
	tagToActionReceiver := common.TagToActionReceiverFn(makeFindEntity(map[string]state.Entity{
		"machine-1": &fakeActionReceiver{watcher: &fakeWatcher{}},
		"machine-2": &fakeActionReceiver{watcher: &fakeWatcher{err: expectErr}},
	}))

	watchOneFn := common.WatchPendingActionsForReceiver(tagToActionReceiver, registerFunc)

	for i, test := range []struct {
		tag       names.Tag
		err       string
		watcherId string
	}{{
		tag: names.NewMachineTag("0"),
		err: "machine-0 not found",
	}, {
		tag:       names.NewMachineTag("1"),
		watcherId: "bambalam",
	}, {
		tag: names.NewMachineTag("2"),
		err: "zwoosh",
	}} {
		c.Logf("test %d", i)
		c.Logf(test.tag.String())
		result, err := watchOneFn(test.tag)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
			c.Check(result, jc.DeepEquals, params.StringsWatchResult{})
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(result.StringsWatcherId, gc.Equals, test.watcherId)
		}
	}
}

func makeWatchOne(mapping map[names.Tag]params.StringsWatchResult) func(names.Tag) (params.StringsWatchResult, error) {
	return func(tag names.Tag) (params.StringsWatchResult, error) {
		result, ok := mapping[tag]
		if !ok {
			return params.StringsWatchResult{}, errors.New("pax")
		}
		return result, nil
	}
}

func makeFindEntity(tagToEntity map[string]state.Entity) func(tag names.Tag) (state.Entity, error) {
	return func(tag names.Tag) (state.Entity, error) {
		receiver, ok := tagToEntity[tag.String()]
		if !ok {
			return nil, errors.New("splat")
		}
		return receiver, nil
	}
}

func makeCanAccess(allowed map[names.Tag]bool) common.AuthFunc {
	return func(tag names.Tag) bool {
		_, ok := allowed[tag]
		return ok
	}
}

var actionNotFoundErr = errors.New("action not found")

func makeGetActionByTag(tagToAction map[names.ActionTag]state.Action) func(names.ActionTag) (state.Action, error) {
	return func(tag names.ActionTag) (state.Action, error) {
		action, ok := tagToAction[tag]
		if !ok {
			return nil, actionNotFoundErr
		}
		return action, nil
	}
}

func makeGetActionByTagString(tagToAction map[string]state.Action) func(string) (state.Action, error) {
	return func(tag string) (state.Action, error) {
		action, ok := tagToAction[tag]
		if !ok {
			return nil, errors.New("action not found")
		}
		return action, nil
	}
}

type fakeActionReceiver struct {
	state.ActionReceiver
	watcher state.StringsWatcher
}

func (mock fakeActionReceiver) WatchActionNotifications() state.StringsWatcher {
	return mock.watcher
}

func (mock fakeActionReceiver) WatchPendingActionNotifications() state.StringsWatcher {
	return mock.watcher
}

type fakeWatcher struct {
	state.StringsWatcher
	err error
}

func (mock fakeWatcher) Changes() <-chan []string {
	ch := make(chan []string, 1)
	if mock.err != nil {
		close(ch)
	} else {
		ch <- []string{"pew", "pew", "pew"}
	}
	return ch
}

func (mock fakeWatcher) Err() error {
	return mock.err
}

type fakeEntity struct {
	state.Entity
}

type fakeAction struct {
	state.Action
	receiver  string
	name      string
	beginErr  error
	finishErr error
	status    state.ActionStatus
}

func (mock fakeAction) Status() state.ActionStatus {
	return mock.status
}

func (mock fakeAction) Begin() (state.Action, error) {
	return nil, mock.beginErr
}

func (mock fakeAction) Receiver() string {
	return mock.receiver
}

func (mock fakeAction) Name() string {
	return mock.name
}

func (mock fakeAction) Parameters() map[string]interface{} {
	return nil
}

func (mock fakeAction) Finish(state.ActionResults) (state.Action, error) {
	return nil, mock.finishErr
}

// entities is a convenience constructor for params.Entities.
func entities(tags ...string) params.Entities {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag
	}
	return entities
}
