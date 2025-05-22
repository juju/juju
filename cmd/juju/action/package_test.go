// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"context"
	"os"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/exec"

	actionapi "github.com/juju/juju/api/client/action"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

const (
	validActionId        = "1"
	validActionId2       = "2"
	validActionId3       = "3"
	validUnitId          = "mysql/0"
	validUnitId2         = "mysql/1"
	invalidUnitId        = "something-strange-"
	invalidMachineId     = "fred"
	validApplicationId   = "mysql"
	invalidApplicationId = "something-strange-"
)


type BaseActionSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite

	modelFlags []string
	store      *jujuclient.MemStore
	clock      testclock.AdvanceableClock
}

func (s *BaseActionSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.modelFlags = []string{"-m", "--model"}

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "ctrl"
	s.store.Controllers["ctrl"] = jujuclient.ControllerDetails{}
	s.store.Models["ctrl"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{"admin/admin": {ModelType: "iaas"}}}
	s.store.Accounts["ctrl"] = jujuclient.AccountDetails{
		User: "admin",
	}

	s.clock = testClock()
}

func (s *BaseActionSuite) patchAPIClient(client *fakeAPIClient) func() {
	return testhelpers.PatchValue(action.NewActionAPIClient,
		func(ctx context.Context, c *action.ActionCommandBase) (action.APIClient, error) {
			return client, nil
		},
	)
}

var someCharmActions = map[string]actionapi.ActionSpec{
	"snapshot": {
		Description: "Take a snapshot of the database.",
		Params: map[string]interface{}{
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "snapshot name",
				},
				"full": map[string]interface{}{
					"type":        "boolean",
					"description": "take a full backup",
					"default":     true,
				},
				"prefix": map[string]interface{}{
					"type":        "string",
					"description": "prefix to snapshot name",
					"default":     "",
				},
			},
			"baz": "bar",
		},
	},
	"kill": {
		Description: "Kill the database.",
		Params: map[string]interface{}{
			"properties": map[string]interface{}{
				"baz": map[string]interface{}{
					"type": "string",
				},
			},
			"foo": "baz",
		},
	},
	"no-description": {
		Params: map[string]interface{}{
			"properties": map[string]interface{}{
				"baz": map[string]interface{}{
					"type": "string",
				},
			},
			"foo": "baz",
		},
	},
	"no-params": {
		Description: "An action with no parameters.\n",
	},
}

// setupValueFile creates a file containing one value for testing.
// cf. cmd/juju/set_test.go
func setupValueFile(c *tc.C, dir, filename, value string) string {
	ctx := cmdtesting.ContextForDir(c, dir)
	path := ctx.AbsPath(filename)
	content := []byte(value)
	err := os.WriteFile(path, content, 0666)
	c.Assert(err, tc.ErrorIsNil)
	return path
}

type fakeAPIClient struct {
	block              bool
	delay              clock.Timer
	timeout            clock.Timer
	actionResults      []actionapi.ActionResult
	operationResults   actionapi.Operations
	operationQueryArgs actionapi.OperationQueryArgs
	enqueuedActions    []actionapi.Action
	charmActions       map[string]actionapi.ActionSpec
	machines           set.Strings
	execParams         *actionapi.RunParams
	apiErr             error
	logMessageCh       chan []string
	waitForResults     chan bool
}

var _ action.APIClient = (*fakeAPIClient)(nil)

func (c *fakeAPIClient) Close() error {
	return nil
}

func (c *fakeAPIClient) EnqueueOperation(ctx context.Context, args []actionapi.Action) (actionapi.EnqueuedActions, error) {
	c.enqueuedActions = args
	actions := make([]actionapi.ActionResult, len(c.actionResults))
	for i, a := range c.actionResults {
		actions[i] = actionapi.ActionResult{
			Action: a.Action,
			Error:  a.Error,
		}
	}
	return actionapi.EnqueuedActions{
		OperationID: "1",
		Actions:     actions}, c.apiErr
}

func (c *fakeAPIClient) Cancel(ctx context.Context, _ []string) ([]actionapi.ActionResult, error) {
	return c.actionResults, c.apiErr
}

func (c *fakeAPIClient) ApplicationCharmActions(ctx context.Context, _ string) (map[string]actionapi.ActionSpec, error) {
	return c.charmActions, c.apiErr
}

func (c *fakeAPIClient) getActionResults(actionIDs []string) []actionapi.ActionResult {
	var result []actionapi.ActionResult
	for _, a := range c.actionResults {
		if a.Error != nil || a.Action == nil {
			result = append(result, a)
			continue
		}
		for _, ID := range actionIDs {
			if a.Action.ID == ID {
				result = append(result, a)
				break
			}
		}
	}
	return result
}

func (c *fakeAPIClient) Actions(ctx context.Context, actionIDs []string) ([]actionapi.ActionResult, error) {
	// If the test supplies a delay time too long, we'll return an error
	// to prevent the test hanging.  If the given wait is up, then return
	// the results; otherwise, return a pending status.

	if c.delay == nil && c.waitForResults == nil {
		// No delay requested, just return immediately.
		return c.getActionResults(actionIDs), c.apiErr
	}
	var delayChan, timeoutChan <-chan time.Time
	if c.delay != nil {
		delayChan = c.delay.Chan()
	}
	if c.timeout != nil {
		timeoutChan = c.timeout.Chan()
	}
	select {
	case <-c.waitForResults:
		return c.getActionResults(actionIDs), c.apiErr
	case <-delayChan:
		// The API delay timer is up.
		return c.getActionResults(actionIDs), c.apiErr
	case <-timeoutChan:
		// Timeout to prevent tests from hanging.
		return nil, errors.New("test timed out before wait time")
	default:
		// Timeout should only be nonzero in case we want to test
		// pending behavior with a --wait flag on FetchCommand.
		return []actionapi.ActionResult{{
			Status:   "pending",
			Output:   map[string]interface{}{},
			Started:  time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Enqueued: time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		}}, nil
	}
}

func (c *fakeAPIClient) WatchActionProgress(ctx context.Context, _ string) (watcher.StringsWatcher, error) {
	return watchertest.NewMockStringsWatcher(c.logMessageCh), nil
}

func (c *fakeAPIClient) ListOperations(ctx context.Context, args actionapi.OperationQueryArgs) (actionapi.Operations, error) {
	c.operationQueryArgs = args
	return c.operationResults, c.apiErr
}

func (c *fakeAPIClient) Operation(ctx context.Context, id string) (actionapi.Operation, error) {
	// If the test supplies a delay time too long, we'll return an error
	// to prevent the test hanging.  If the given wait is up, then return
	// the results; otherwise, return a pending status.

	if c.delay == nil && c.waitForResults == nil {
		// No delay requested, just return immediately.
		return c.getOperation(id)
	}
	var delayChan, timeoutChan <-chan time.Time
	if c.delay != nil {
		delayChan = c.delay.Chan()
	}
	if c.timeout != nil {
		timeoutChan = c.timeout.Chan()
	}
	select {
	case <-c.waitForResults:
		return c.getOperation(id)
	case <-delayChan:
		// The API delay timer is up.
		return c.getOperation(id)
	case <-timeoutChan:
		// Timeout to prevent tests from hanging.
		return actionapi.Operation{}, errors.New("test timed out before wait time")
	default:
		// Timeout should only be nonzero in case we want to test
		// pending behavior with a --wait flag on FetchCommand.
		return actionapi.Operation{
			ID:       "667",
			Status:   "pending",
			Started:  time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Enqueued: time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		}, nil
	}
}

func (c *fakeAPIClient) getOperation(id string) (actionapi.Operation, error) {
	if c.apiErr != nil {
		return actionapi.Operation{}, c.apiErr
	}
	if len(c.operationResults.Operations) == 0 || c.operationResults.Operations[0].ID != id {
		return actionapi.Operation{}, errors.NotFoundf("operation %q", id)
	}
	return c.operationResults.Operations[0], nil
}

func (c *fakeAPIClient) resultForMachine(machineId string) (actionapi.ActionResult, bool) {
	for _, r := range c.actionResults {
		if r.Action != nil && r.Action.Receiver == "machine-"+machineId {
			return r, true
		}
	}
	return actionapi.ActionResult{}, false
}

func (c *fakeAPIClient) resultForUnit(unitName string) (actionapi.ActionResult, bool) {
	for _, r := range c.actionResults {
		if r.Action != nil && r.Action.Receiver == names.NewUnitTag(unitName).String() {
			return r, true
		}
	}
	return actionapi.ActionResult{}, false
}

func (c *fakeAPIClient) RunOnAllMachines(ctx context.Context, _ string, _ time.Duration) (actionapi.EnqueuedActions, error) {
	var result actionapi.EnqueuedActions

	if c.block {
		return result, apiservererrors.OperationBlockedError("the operation has been blocked")
	}
	result.OperationID = "1"
	sortedMachineIds := c.machines.SortedValues()

	for _, machineId := range sortedMachineIds {
		response, found := c.resultForMachine(machineId)
		if !found {
			// Consider this a wait timeout.
			response = actionapi.ActionResult{
				Action: &actionapi.Action{
					Receiver: names.NewMachineTag(machineId).String(),
				},
				Message: exec.ErrCancelled.Error(),
			}
		}
		result.Actions = append(result.Actions, response)
	}
	result.OperationID = "1"

	return result, nil
}

func (c *fakeAPIClient) Run(ctx context.Context, runParams actionapi.RunParams) (actionapi.EnqueuedActions, error) {
	var result actionapi.EnqueuedActions

	c.execParams = &runParams

	if c.block {
		return result, apiservererrors.OperationBlockedError("the operation has been blocked")
	}
	result.OperationID = "1"
	// Just add in ids that match in order.
	for _, id := range runParams.Machines {
		response, found := c.resultForMachine(id)
		if found {
			result.Actions = append(result.Actions, response)
		}
	}
	// mock ignores applications
	for _, id := range runParams.Units {
		response, found := c.resultForUnit(id)
		if found {
			result.Actions = append(result.Actions, response)
		}
	}

	return result, nil
}
