// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"time"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/migrationminion"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/testing"
)

type ClientSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) TestWatch(c *gc.C) {
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
	w, err := client.Watch()
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Stop(w)

	errC := make(chan error)
	go func() {
		errC <- w.Wait()
	}()

	select {
	case err := <-errC:
		c.Assert(err, gc.ErrorMatches, "boom")
		expectedCalls := []jujutesting.StubCall{
			{"Migrationminion.Watch", []interface{}{"", nil}},
			{"MigrationStatusWatcher.Next", []interface{}{"abc", nil}},
			{"MigrationStatusWatcher.Stop", []interface{}{"abc", nil}},
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

func (s *ClientSuite) TestWatchErr(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	client := migrationminion.NewClient(apiCaller)
	_, err := client.Watch()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestReport(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, arg)
		return nil
	})

	client := migrationminion.NewClient(apiCaller)
	err := client.Report("id", migration.IMPORT, true)
	c.Assert(err, jc.ErrorIsNil)

	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMinion.Report", []interface{}{params.MinionReport{
			MigrationId: "id",
			Phase:       "IMPORT",
			Success:     true,
		}}},
	})
}

func (s *ClientSuite) TestReportError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("boom")
	})

	client := migrationminion.NewClient(apiCaller)
	err := client.Report("id", migration.IMPORT, true)
	c.Assert(err, gc.ErrorMatches, "boom")
}
