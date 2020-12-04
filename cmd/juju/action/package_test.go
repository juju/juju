// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2/exec"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

const (
	validActionTagString   = "action-1"
	validActionTagString2  = "action-2"
	validActionTagString3  = "action-3"
	invalidActionTagString = "action-invalid"
	validActionId          = "1"
	validUnitId            = "mysql/0"
	validUnitId2           = "mysql/1"
	invalidUnitId          = "something-strange-"
	invalidMachineId       = "fred"
	validApplicationId     = "mysql"
	invalidApplicationId   = "something-strange-"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type BaseActionSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	command cmd.Command

	modelFlags []string
	store      *jujuclient.MemStore
	clock      *testclock.Clock
}

func (s *BaseActionSuite) SetUpTest(c *gc.C) {
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
	return jujutesting.PatchValue(action.NewActionAPIClient,
		func(c *action.ActionCommandBase) (action.APIClient, error) {
			return client, nil
		},
	)
}

var someCharmActions = map[string]params.ActionSpec{
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
func setupValueFile(c *gc.C, dir, filename, value string) string {
	ctx := cmdtesting.ContextForDir(c, dir)
	path := ctx.AbsPath(filename)
	content := []byte(value)
	err := ioutil.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

type fakeAPIClient struct {
	block              bool
	delay              clock.Timer
	timeout            clock.Timer
	actionResults      []params.ActionResult
	operationResults   []params.OperationResult
	operationQueryArgs params.OperationQueryArgs
	enqueuedActions    params.Actions
	actionsByReceivers []params.ActionsByReceiver
	charmActions       map[string]params.ActionSpec
	machines           set.Strings
	execParams         *params.RunParams
	apiErr             error
	logMessageCh       chan []string
	waitForResults     chan bool
}

var _ action.APIClient = (*fakeAPIClient)(nil)

// EnqueuedActions is a testing method which shows what Actions got enqueued
// by our Enqueue stub.
func (c *fakeAPIClient) EnqueuedActions() params.Actions {
	return c.enqueuedActions
}

func (c *fakeAPIClient) Close() error {
	return nil
}

func (c *fakeAPIClient) EnqueueOperation(args params.Actions) (params.EnqueuedActions, error) {
	c.enqueuedActions = args
	return params.EnqueuedActions{
		OperationTag: "operation-1",
		Actions:      c.actionResults}, c.apiErr
}

func (c *fakeAPIClient) Cancel(args params.Entities) (params.ActionResults, error) {
	return c.getActionResults(args.Entities), c.apiErr
}

func (c *fakeAPIClient) ApplicationCharmActions(params.Entity) (map[string]params.ActionSpec, error) {
	return c.charmActions, c.apiErr
}

func (c *fakeAPIClient) getActionResults(entities []params.Entity) params.ActionResults {
	var result params.ActionResults
	for _, a := range c.actionResults {
		if a.Error != nil || a.Action == nil {
			result.Results = append(result.Results, a)
			continue
		}
		for _, e := range entities {
			if a.Action.Tag == e.Tag {
				result.Results = append(result.Results, a)
				break
			}
		}
	}
	return result
}

func (c *fakeAPIClient) Actions(args params.Entities) (params.ActionResults, error) {
	// If the test supplies a delay time too long, we'll return an error
	// to prevent the test hanging.  If the given wait is up, then return
	// the results; otherwise, return a pending status.

	if c.delay == nil && c.waitForResults == nil {
		// No delay requested, just return immediately.
		return c.getActionResults(args.Entities), c.apiErr
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
		return c.getActionResults(args.Entities), c.apiErr
	case _ = <-delayChan:
		// The API delay timer is up.
		return c.getActionResults(args.Entities), c.apiErr
	case _ = <-timeoutChan:
		// Timeout to prevent tests from hanging.
		return params.ActionResults{}, errors.New("test timed out before wait time")
	default:
		// Timeout should only be nonzero in case we want to test
		// pending behavior with a --wait flag on FetchCommand.
		return params.ActionResults{Results: []params.ActionResult{{
			Status:   params.ActionPending,
			Output:   map[string]interface{}{},
			Started:  time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Enqueued: time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		}}}, nil
	}
}

func (c *fakeAPIClient) WatchActionProgress(actionId string) (watcher.StringsWatcher, error) {
	return watchertest.NewMockStringsWatcher(c.logMessageCh), nil
}

func (c *fakeAPIClient) ListOperations(args params.OperationQueryArgs) (params.OperationResults, error) {
	c.operationQueryArgs = args
	return params.OperationResults{
		Results: c.operationResults,
	}, c.apiErr
}

func (c *fakeAPIClient) Operation(id string) (params.OperationResult, error) {
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
	case _ = <-delayChan:
		// The API delay timer is up.
		return c.getOperation(id)
	case _ = <-timeoutChan:
		// Timeout to prevent tests from hanging.
		return params.OperationResult{}, errors.New("test timed out before wait time")
	default:
		// Timeout should only be nonzero in case we want to test
		// pending behavior with a --wait flag on FetchCommand.
		return params.OperationResult{
			OperationTag: names.NewOperationTag("667").String(),
			Status:       params.ActionPending,
			Started:      time.Date(2015, time.February, 14, 8, 15, 0, 0, time.UTC),
			Enqueued:     time.Date(2015, time.February, 14, 8, 13, 0, 0, time.UTC),
		}, nil
	}
}

func (c *fakeAPIClient) getOperation(id string) (params.OperationResult, error) {
	if c.apiErr != nil {
		return params.OperationResult{}, c.apiErr
	}
	if len(c.operationResults) == 0 || c.operationResults[0].OperationTag != names.NewOperationTag(id).String() {
		return params.OperationResult{}, errors.NotFoundf("operation %q", id)
	}
	return c.operationResults[0], nil
}

func (c *fakeAPIClient) resultForMachine(machineId string) (params.ActionResult, bool) {
	for _, r := range c.actionResults {
		if r.Action != nil && r.Action.Receiver == "machine-"+machineId {
			return r, true
		}
	}
	return params.ActionResult{}, false
}

func (c *fakeAPIClient) resultForUnit(unitName string) (params.ActionResult, bool) {
	for _, r := range c.actionResults {
		if r.Action != nil && r.Action.Receiver == names.NewUnitTag(unitName).String() {
			return r, true
		}
	}
	return params.ActionResult{}, false
}

func (c *fakeAPIClient) RunOnAllMachines(commands string, wait time.Duration) (params.EnqueuedActions, error) {
	var result params.EnqueuedActions

	if c.block {
		return result, apiservererrors.OperationBlockedError("the operation has been blocked")
	}
	result.OperationTag = "operation-1"
	sortedMachineIds := c.machines.SortedValues()

	for _, machineId := range sortedMachineIds {
		response, found := c.resultForMachine(machineId)
		if !found {
			// Consider this a wait timeout.
			response = params.ActionResult{
				Action: &params.Action{
					Receiver: names.NewMachineTag(machineId).String(),
				},
				Message: exec.ErrCancelled.Error(),
			}
		}
		result.Actions = append(result.Actions, response)
	}
	result.OperationTag = "operation-1"

	return result, nil
}

func (c *fakeAPIClient) Run(runParams params.RunParams) (params.EnqueuedActions, error) {
	var result params.EnqueuedActions

	c.execParams = &runParams

	if c.block {
		return result, apiservererrors.OperationBlockedError("the operation has been blocked")
	}
	result.OperationTag = "operation-1"
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
