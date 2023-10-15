// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

const (
	validActionTagString  = "action-f47ac10b-58cc-4372-a567-0e02b2c3d479"
	validActionTagString2 = "action-f47ac10b-58cc-4372-a567-0e02b2c3d478"
	validActionId         = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	validActionId2        = "f47ac10b-58cc-4372-a567-0e02b2c3d478"
	validUnitId           = "mysql/0"
	validUnitId2          = "mysql/1"
	invalidUnitId         = "something-strange-"
	invalidMachineId      = "fred"
	validApplicationId    = "mysql"
	invalidApplicationId  = "something-strange-"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type BaseActionSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite

	modelFlags []string
	store      *jujuclient.MemStore
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
}

func (s *BaseActionSuite) patchAPIClient(client *fakeAPIClient) func() {
	return jujutesting.PatchValue(action.NewActionAPIClient,
		func(c *action.ActionCommandBase) (action.APIClient, error) {
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

// tagsForIdPrefix builds a params.FindTagResults for a given id prefix
// and 0..n given tags. This is useful for stubbing out the API and
// ensuring that the API returns expected tags for a given id prefix.
func tagsForIdPrefix(prefix string, tags ...string) params.FindTagsResults {
	entities := make([]params.Entity, len(tags))
	for i, t := range tags {
		entities[i] = params.Entity{Tag: t}
	}
	return params.FindTagsResults{Matches: map[string][]params.Entity{prefix: entities}}
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
	delay              *time.Timer
	timeout            *time.Timer
	actionResults      []actionapi.ActionResult
	operationResults   actionapi.Operations
	operationQueryArgs actionapi.OperationQueryArgs
	enqueuedActions    []actionapi.Action
	charmActions       map[string]actionapi.ActionSpec
	apiVersion         int
	apiErr             error
	logMessageCh       chan []string
	waitForResults     chan bool

	// These are to support legacy action UUIDs.
	actionTagMatches params.FindTagsResults
	actionsByNames   map[string][]actionapi.ActionResult
}

var _ action.APIClient = (*fakeAPIClient)(nil)

func (c *fakeAPIClient) Close() error {
	return nil
}

func (c *fakeAPIClient) BestAPIVersion() int {
	return c.apiVersion
}

func (c *fakeAPIClient) Enqueue(actions []actionapi.Action) ([]actionapi.ActionResult, error) {
	c.enqueuedActions = actions
	return c.actionResults, c.apiErr
}

func (c *fakeAPIClient) EnqueueOperation(args []actionapi.Action) (actionapi.EnqueuedActions, error) {
	c.enqueuedActions = args
	actions := make([]actionapi.ActionResult, len(c.actionResults))
	for i, a := range c.actionResults {
		actions[i] = actionapi.ActionResult{
			Error: a.Error,
		}
		if a.Action != nil {
			actions[i].Action = &actionapi.Action{
				ID: a.Action.ID,
			}
		}
	}
	return actionapi.EnqueuedActions{
		OperationID: "1",
		Actions:     actions}, c.apiErr
}

func (c *fakeAPIClient) Cancel(_ []string) ([]actionapi.ActionResult, error) {
	return c.actionResults, c.apiErr
}

func (c *fakeAPIClient) ApplicationCharmActions(_ string) (map[string]actionapi.ActionSpec, error) {
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

func (c *fakeAPIClient) Actions(actionIDs []string) ([]actionapi.ActionResult, error) {
	// If the test supplies a delay time too long, we'll return an error
	// to prevent the test hanging.  If the given wait is up, then return
	// the results; otherwise, return a pending status.

	if c.delay == nil && c.waitForResults == nil {
		// No delay requested, just return immediately.
		return c.getActionResults(actionIDs), c.apiErr
	}
	var delayChan, timeoutChan <-chan time.Time
	if c.delay != nil {
		delayChan = c.delay.C
	}
	if c.timeout != nil {
		timeoutChan = c.timeout.C
	}
	select {
	case <-c.waitForResults:
		return c.getActionResults(actionIDs), c.apiErr
	case _ = <-delayChan:
		// The API delay timer is up.
		return c.getActionResults(actionIDs), c.apiErr
	case _ = <-timeoutChan:
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

func (c *fakeAPIClient) FindActionTagsByPrefix(_ params.FindTags) (params.FindTagsResults, error) {
	return c.actionTagMatches, c.apiErr
}

func (c *fakeAPIClient) FindActionsByNames(_ params.FindActionsByNames) (map[string][]actionapi.ActionResult, error) {
	return c.actionsByNames, c.apiErr
}

func (c *fakeAPIClient) WatchActionProgress(_ string) (watcher.StringsWatcher, error) {
	return watchertest.NewMockStringsWatcher(c.logMessageCh), nil
}

func (c *fakeAPIClient) ListOperations(args actionapi.OperationQueryArgs) (actionapi.Operations, error) {
	c.operationQueryArgs = args
	return c.operationResults, c.apiErr
}

func (c *fakeAPIClient) Operation(id string) (actionapi.Operation, error) {
	// If the test supplies a delay time too long, we'll return an error
	// to prevent the test hanging.  If the given wait is up, then return
	// the results; otherwise, return a pending status.

	if c.delay == nil && c.waitForResults == nil {
		// No delay requested, just return immediately.
		return c.getOperation(id)
	}
	var delayChan, timeoutChan <-chan time.Time
	if c.delay != nil {
		delayChan = c.delay.C
	}
	if c.timeout != nil {
		timeoutChan = c.timeout.C
	}
	select {
	case <-c.waitForResults:
		return c.getOperation(id)
	case _ = <-delayChan:
		// The API delay timer is up.
		return c.getOperation(id)
	case _ = <-timeoutChan:
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
