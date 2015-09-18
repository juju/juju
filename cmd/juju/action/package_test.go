// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"errors"
	"io/ioutil"
	"regexp"
	"testing"
	"time"

	"github.com/juju/cmd"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/action"
	coretesting "github.com/juju/juju/testing"
)

const (
	validActionTagString   = "action-f47ac10b-58cc-4372-a567-0e02b2c3d479"
	invalidActionTagString = "action-f47ac10b-58cc-4372-a567-0e02b2c3d47"
	validActionId          = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	invalidActionId        = "f47ac10b-58cc-4372-a567-0e02b2c3d47"
	validUnitId            = "mysql/0"
	invalidUnitId          = "something-strange-"
	validServiceId         = "mysql"
	invalidServiceId       = "something-strange-"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type BaseActionSuite struct {
	jujutesting.IsolationSuite
	command cmd.Command
}

var _ = gc.Suite(&FetchSuite{})

func (s *BaseActionSuite) SetUpTest(c *gc.C) {
	s.command = action.NewSuperCommand()
}

func (s *BaseActionSuite) patchAPIClient(client *fakeAPIClient) func() {
	return jujutesting.PatchValue(action.NewActionAPIClient,
		func(c *action.ActionCommandBase) (action.APIClient, error) {
			return client, nil
		},
	)
}

func (s *BaseActionSuite) checkHelp(c *gc.C, subcmd envcmd.EnvironCommand) {
	ctx, err := coretesting.RunCommand(c, s.command, subcmd.Info().Name, "--help")
	c.Assert(err, gc.IsNil)

	expected := "(?sm).*^usage: juju action " +
		regexp.QuoteMeta(subcmd.Info().Name) +
		` \[options\] ` + regexp.QuoteMeta(subcmd.Info().Args) + ".+"
	c.Check(coretesting.Stdout(ctx), gc.Matches, expected)

	expected = "(?sm).*^purpose: " + regexp.QuoteMeta(subcmd.Info().Purpose) + "$.*"
	c.Check(coretesting.Stdout(ctx), gc.Matches, expected)

	expected = "(?sm).*^" + regexp.QuoteMeta(subcmd.Info().Doc) + "$.*"
	c.Check(coretesting.Stdout(ctx), gc.Matches, expected)
}

var someCharmActions = &charm.Actions{
	ActionSpecs: map[string]charm.ActionSpec{
		"snapshot": {
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "baz",
				},
				"baz": "bar",
			},
		},
		"kill": {
			Description: "Kill the database.",
			Params: map[string]interface{}{
				"bar": map[string]interface{}{
					"baz": "foo",
				},
				"foo": "baz",
			},
		},
		"no-description": {
			Params: map[string]interface{}{
				"bar": map[string]interface{}{
					"baz": "foo",
				},
				"foo": "baz",
			},
		},
		"no-params": {
			Description: "An action with no parameters.",
		},
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
	ctx := coretesting.ContextForDir(c, dir)
	path := ctx.AbsPath(filename)
	content := []byte(value)
	err := ioutil.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

type fakeAPIClient struct {
	delay              *time.Timer
	timeout            *time.Timer
	actionResults      []params.ActionResult
	enqueuedActions    params.Actions
	actionsByReceivers []params.ActionsByReceiver
	actionTagMatches   params.FindTagsResults
	charmActions       *charm.Actions
	apiErr             error
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

func (c *fakeAPIClient) Enqueue(args params.Actions) (params.ActionResults, error) {
	c.enqueuedActions = args
	return params.ActionResults{Results: c.actionResults}, c.apiErr
}

func (c *fakeAPIClient) ListAll(args params.Entities) (params.ActionsByReceivers, error) {
	return params.ActionsByReceivers{
		Actions: c.actionsByReceivers,
	}, c.apiErr
}

func (c *fakeAPIClient) ListPending(args params.Entities) (params.ActionsByReceivers, error) {
	return params.ActionsByReceivers{
		Actions: c.actionsByReceivers,
	}, c.apiErr
}

func (c *fakeAPIClient) ListCompleted(args params.Entities) (params.ActionsByReceivers, error) {
	return params.ActionsByReceivers{
		Actions: c.actionsByReceivers,
	}, c.apiErr
}

func (c *fakeAPIClient) Cancel(args params.Actions) (params.ActionResults, error) {
	return params.ActionResults{
		Results: c.actionResults,
	}, c.apiErr
}

func (c *fakeAPIClient) ServiceCharmActions(params.Entity) (*charm.Actions, error) {
	return c.charmActions, c.apiErr
}

func (c *fakeAPIClient) Actions(args params.Entities) (params.ActionResults, error) {
	// If the test supplies a delay time too long, we'll return an error
	// to prevent the test hanging.  If the given wait is up, then return
	// the results; otherwise, return a pending status.

	// First, sync.
	_ = <-time.NewTimer(0 * time.Second).C

	select {
	case _ = <-c.delay.C:
		// The API delay timer is up.  Pass pre-canned results back.
		return params.ActionResults{Results: c.actionResults}, c.apiErr
	case _ = <-c.timeout.C:
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

func (c *fakeAPIClient) FindActionTagsByPrefix(arg params.FindTags) (params.FindTagsResults, error) {
	return c.actionTagMatches, c.apiErr
}
