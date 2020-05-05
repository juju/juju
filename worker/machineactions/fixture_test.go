// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"errors"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"

	"github.com/juju/juju/api/machineactions"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

var actionNotFoundErr = errors.New("action not found")

func mockHandleAction(stub *testing.Stub) func(string, map[string]interface{}) (map[string]interface{}, error) {
	return func(name string, params map[string]interface{}) (map[string]interface{}, error) {
		stub.AddCall("HandleAction", name)
		return nil, stub.NextErr()
	}
}

// mockFacade implements machineactions.Facade for use in the tests.
type mockFacade struct {
	stub                     *testing.Stub
	runningActions           []params.ActionResult
	watcherSendInvalidValues bool
}

// RunningActions is part of the machineactions.Facade interface.
func (mock *mockFacade) RunningActions(agent names.MachineTag) ([]params.ActionResult, error) {
	mock.stub.AddCall("RunningActions", agent)
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	return mock.runningActions, nil
}

// RunningActions is part of the machineactions.Facade interface.
func (mock *mockFacade) Action(tag names.ActionTag) (*machineactions.Action, error) {
	mock.stub.AddCall("Action", tag)
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	return tagToActionMap[tag], nil
}

// ActionBegin is part of the machineactions.Facade interface.
func (mock *mockFacade) ActionBegin(tag names.ActionTag) error {
	mock.stub.AddCall("ActionBegin", tag)
	return mock.stub.NextErr()
}

// ActionFinish is part of the machineactions.Facade interface.
func (mock *mockFacade) ActionFinish(tag names.ActionTag, status string, results map[string]interface{}, message string) error {
	mock.stub.AddCall("ActionFinish", tag, status, message)
	return mock.stub.NextErr()
}

// Watch is part of the machineactions.Facade interface.
func (mock *mockFacade) WatchActionNotifications(agent names.MachineTag) (watcher.StringsWatcher, error) {
	mock.stub.AddCall("WatchActionNotifications", agent)
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	return newStubWatcher(mock.watcherSendInvalidValues), nil
}

// stubWatcher implements watcher.StringsWatcher and supplied canned
// data over the Changes() channel.
type stubWatcher struct {
	worker.Worker
	changes chan []string
}

func newStubWatcher(watcherSendInvalidValues bool) *stubWatcher {
	changes := make(chan []string, 3)
	changes <- []string{firstActionID, secondActionID}
	changes <- []string{thirdActionID}
	if watcherSendInvalidValues {
		changes <- []string{"invalid-action-id"}
	}
	return &stubWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: changes,
	}
}

// Changes is part of the watcher.StringsWatcher interface.
func (stubWatcher *stubWatcher) Changes() watcher.StringsChannel {
	return stubWatcher.changes
}

var (
	firstAction     = machineactions.NewAction("foo", nil)
	secondAction    = machineactions.NewAction("baz", nil)
	thirdAction     = machineactions.NewAction("boo", nil)
	firstActionID   = "11234567-89ab-cdef-0123-456789abcdef"
	secondActionID  = "21234567-89ab-cdef-0123-456789abcdef"
	thirdActionID   = "31234567-89ab-cdef-0123-456789abcdef"
	firstActionTag  = names.NewActionTag(firstActionID)
	secondActionTag = names.NewActionTag(secondActionID)
	thirdActionTag  = names.NewActionTag(thirdActionID)
	tagToActionMap  = map[names.ActionTag]*machineactions.Action{
		firstActionTag:  firstAction,
		secondActionTag: secondAction,
		thirdActionTag:  thirdAction,
	}
	fakeRunningActions = []params.ActionResult{
		{Action: &params.Action{Tag: thirdActionTag.String()}},
	}
)
