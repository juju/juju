// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
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

	client := migrationmaster.NewClient(apiCaller)
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
			{"MigrationMaster.Watch", []interface{}{"", nil}},
			{"NotifyWatcher.Next", []interface{}{"abc", nil}},
			{"NotifyWatcher.Stop", []interface{}{"abc", nil}},
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
	client := migrationmaster.NewClient(apiCaller)
	_, err := client.Watch()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestGetMigrationStatus(c *gc.C) {
	modelUUID := utils.MustNewUUID().String()
	controllerUUID := utils.MustNewUUID().String()
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _, _ string, _, result interface{}) error {
		out := result.(*params.FullMigrationStatus)
		*out = params.FullMigrationStatus{
			Spec: params.ModelMigrationSpec{
				ModelTag: names.NewModelTag(modelUUID).String(),
				TargetInfo: params.ModelMigrationTargetInfo{
					ControllerTag: names.NewModelTag(controllerUUID).String(),
					Addrs:         []string{"2.2.2.2:2"},
					CACert:        "cert",
					AuthTag:       names.NewUserTag("admin").String(),
					Password:      "secret",
				},
			},
			Attempt: 3,
			Phase:   "READONLY",
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller)

	status, err := client.GetMigrationStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.DeepEquals, migrationmaster.MigrationStatus{
		ModelUUID: modelUUID,
		Attempt:   3,
		Phase:     migration.READONLY,
		TargetInfo: migration.TargetInfo{
			ControllerTag: names.NewModelTag(controllerUUID),
			Addrs:         []string{"2.2.2.2:2"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("admin"),
			Password:      "secret",
		},
	})
}

func (s *ClientSuite) TestSetPhase(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationmaster.NewClient(apiCaller)
	err := client.SetPhase(migration.QUIESCE)
	c.Assert(err, jc.ErrorIsNil)
	expectedArg := params.SetMigrationPhaseArgs{Phase: "QUIESCE"}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.SetPhase", []interface{}{"", expectedArg}},
	})
}

func (s *ClientSuite) TestSetPhaseError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller)
	err := client.SetPhase(migration.QUIESCE)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestExport(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		out := result.(*params.SerializedModel)
		*out = params.SerializedModel{Bytes: []byte("foo")}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller)
	bytes, err := client.Export()
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.Export", []interface{}{"", nil}},
	})
	c.Assert(string(bytes), gc.Equals, "foo")
}

func (s *ClientSuite) TestExportError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller)
	_, err := client.Export()
	c.Assert(err, gc.ErrorMatches, "blam")
}

func (s *ClientSuite) TestReap(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationmaster.NewClient(apiCaller)
	err := client.Reap()
	c.Check(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.Reap", []interface{}{"", nil}},
	})
}

func (s *ClientSuite) TestReapError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller)
	err := client.Reap()
	c.Assert(err, gc.ErrorMatches, "blam")
}
