// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/api/agent/migrationminion"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type ClientSuite struct {
	jujutesting.IsolationSuite
}

var _ = tc.Suite(&ClientSuite{})

func (s *ClientSuite) TestWatch(c *tc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		switch request {
		case "Watch":
			*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
				NotifyWatcherId: "abc",
			}
		case "Next":
			// The full success case is tested in api/watcher.
			return errors.New("boom")
		case "Stop":
		}
		return nil
	})

	client := migrationminion.NewClient(apiCaller)
	w, err := client.Watch(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Stop(w)

	errC := make(chan error)
	go func() {
		errC <- w.Wait()
	}()

	select {
	case err := <-errC:
		c.Assert(err, tc.ErrorMatches, "boom")
		expectedCalls := []jujutesting.StubCall{
			{FuncName: "Migrationminion.Watch", Args: []interface{}{"", nil}},
			{FuncName: "MigrationStatusWatcher.Next", Args: []interface{}{"abc", nil}},
			{FuncName: "MigrationStatusWatcher.Stop", Args: []interface{}{"abc", nil}},
		}
		// The Stop API call happens in a separate goroutine which
		// might execute after the worker has exited so wait for the
		// expected calls to arrive.
		for a := coretesting.LongAttempt.Start(); a.Next(); {
			if len(stub.Calls()) >= len(expectedCalls) {
				return
			}
		}
		stub.CheckCalls(c, expectedCalls)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for watcher to die")
	}
}

func (s *ClientSuite) TestWatchErr(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	client := migrationminion.NewClient(apiCaller)
	_, err := client.Watch(context.Background())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestReport(c *tc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, arg)
		return nil
	})

	client := migrationminion.NewClient(apiCaller)
	err := client.Report(context.Background(), "id", migration.IMPORT, true)
	c.Assert(err, jc.ErrorIsNil)

	stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "MigrationMinion.Report", Args: []interface{}{params.MinionReport{
			MigrationId: "id",
			Phase:       "IMPORT",
			Success:     true,
		}}},
	})
}

func (s *ClientSuite) TestReportError(c *tc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("boom")
	})

	client := migrationminion.NewClient(apiCaller)
	err := client.Report(context.Background(), "id", migration.IMPORT, true)
	c.Assert(err, tc.ErrorMatches, "boom")
}
