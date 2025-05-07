// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/api/agent/machineactions"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

func mockHandleAction(stub *testhelpers.Stub) func(string, map[string]interface{}) (map[string]interface{}, error) {
	return func(name string, params map[string]interface{}) (map[string]interface{}, error) {
		stub.AddCall("HandleAction", name)
		return nil, stub.NextErr()
	}
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
	firstAction        = machineactions.NewAction("1", "foo", nil, true, "")
	secondAction       = machineactions.NewAction("2", "baz", nil, false, "")
	thirdAction        = machineactions.NewAction("3", "boo", nil, true, "")
	firstActionID      = "1"
	secondActionID     = "2"
	thirdActionID      = "3"
	thirdActionTag     = names.NewActionTag(thirdActionID)
	fakeRunningActions = []params.ActionResult{
		{Action: &params.Action{Tag: thirdActionTag.String()}},
	}
)
