// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"time"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/migrationmaster"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/watcher"
)

type ClientSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) TestWatch(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			NotifyWatcherId: "123",
		}
		return nil
	})
	expectWatch := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(caller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(caller, gc.NotNil)
		c.Check(result, jc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "123"})
		return expectWatch
	}
	client := migrationmaster.NewClient(apiCaller, newWatcher)

	w, err := client.Watch()
	c.Check(err, jc.ErrorIsNil)
	c.Check(w, gc.Equals, expectWatch)
	stub.CheckCalls(c, []jujutesting.StubCall{{"MigrationMaster.Watch", []interface{}{"", nil}}})
}

func (s *ClientSuite) TestWatchCallError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.Watch()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestGetMigrationStatus(c *gc.C) {
	modelUUID := utils.MustNewUUID().String()
	controllerUUID := utils.MustNewUUID().String()
	timestamp := time.Date(2016, 6, 22, 16, 42, 44, 0, time.UTC)
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _, _ string, _, result interface{}) error {
		out := result.(*params.MasterMigrationStatus)
		*out = params.MasterMigrationStatus{
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
			MigrationId:      "id",
			Phase:            "PRECHECK",
			PhaseChangedTime: timestamp,
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)

	status, err := client.GetMigrationStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.DeepEquals, migration.MigrationStatus{
		MigrationId:      "id",
		ModelUUID:        modelUUID,
		Phase:            migration.PRECHECK,
		PhaseChangedTime: timestamp,
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
	client := migrationmaster.NewClient(apiCaller, nil)
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
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetPhase(migration.QUIESCE)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestSetStatusMessage(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetStatusMessage("foo")
	c.Assert(err, jc.ErrorIsNil)
	expectedArg := params.SetMigrationStatusMessageArgs{Message: "foo"}
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.SetStatusMessage", []interface{}{"", expectedArg}},
	})
}

func (s *ClientSuite) TestSetStatusMessageError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.SetStatusMessage("foo")
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestExport(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		out := result.(*params.SerializedModel)
		*out = params.SerializedModel{
			Bytes:  []byte("foo"),
			Charms: []string{"cs:foo-1"},
			Tools: []params.SerializedModelTools{{
				Version: "2.0.0-trusty-amd64",
				URI:     "/tools/0",
			}},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	out, err := client.Export()
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.Export", []interface{}{"", nil}},
	})
	c.Assert(out, gc.DeepEquals, migration.SerializedModel{
		Bytes:  []byte("foo"),
		Charms: []string{"cs:foo-1"},
		Tools: map[version.Binary]string{
			version.MustParseBinary("2.0.0-trusty-amd64"): "/tools/0",
		},
	})
}

func (s *ClientSuite) TestExportError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.Export()
	c.Assert(err, gc.ErrorMatches, "blam")
}

func (s *ClientSuite) TestReap(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
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
	client := migrationmaster.NewClient(apiCaller, nil)
	err := client.Reap()
	c.Assert(err, gc.ErrorMatches, "blam")
}

func (s *ClientSuite) TestWatchMinionReports(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			NotifyWatcherId: "123",
		}
		return nil
	})

	expectWatch := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(caller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(caller, gc.NotNil)
		c.Check(result, jc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "123"})
		return expectWatch
	}
	client := migrationmaster.NewClient(apiCaller, newWatcher)

	w, err := client.WatchMinionReports()
	c.Check(err, jc.ErrorIsNil)
	c.Check(w, gc.Equals, expectWatch)
	stub.CheckCalls(c, []jujutesting.StubCall{{"MigrationMaster.WatchMinionReports", []interface{}{"", nil}}})
}

func (s *ClientSuite) TestWatchMinionReportsError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.WatchMinionReports()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ClientSuite) TestGetMinionReports(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		stub.AddCall(objType+"."+request, id, arg)
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			MigrationId:  "id",
			Phase:        "PRECHECK",
			SuccessCount: 4,
			UnknownCount: 3,
			UnknownSample: []string{
				names.NewMachineTag("3").String(),
				names.NewMachineTag("4").String(),
				names.NewUnitTag("foo/0").String(),
			},
			Failed: []string{
				names.NewMachineTag("5").String(),
				names.NewUnitTag("foo/1").String(),
				names.NewUnitTag("foo/2").String(),
			},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	out, err := client.GetMinionReports()
	c.Assert(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"MigrationMaster.GetMinionReports", []interface{}{"", nil}},
	})
	c.Assert(out, gc.DeepEquals, migration.MinionReports{
		MigrationId:         "id",
		Phase:               migration.PRECHECK,
		SuccessCount:        4,
		UnknownCount:        3,
		SomeUnknownMachines: []string{"3", "4"},
		SomeUnknownUnits:    []string{"foo/0"},
		FailedMachines:      []string{"5"},
		FailedUnits:         []string{"foo/1", "foo/2"},
	})
}

func (s *ClientSuite) TestGetMinionReportsFailedCall(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("blam")
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.GetMinionReports()
	c.Assert(err, gc.ErrorMatches, "blam")
}

func (s *ClientSuite) TestGetMinionReportsInvalidPhase(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _ string, _ string, _ interface{}, result interface{}) error {
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			Phase: "BLARGH",
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.GetMinionReports()
	c.Assert(err, gc.ErrorMatches, `invalid phase: "BLARGH"`)
}

func (s *ClientSuite) TestGetMinionReportsBadUnknownTag(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _ string, _ string, _ interface{}, result interface{}) error {
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			Phase:         "PRECHECK",
			UnknownSample: []string{"carl"},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.GetMinionReports()
	c.Assert(err, gc.ErrorMatches, `processing unknown agents: "carl" is not a valid tag`)
}

func (s *ClientSuite) TestGetMinionReportsBadFailedTag(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(_ string, _ int, _ string, _ string, _ interface{}, result interface{}) error {
		out := result.(*params.MinionReports)
		*out = params.MinionReports{
			Phase:  "PRECHECK",
			Failed: []string{"dave"},
		}
		return nil
	})
	client := migrationmaster.NewClient(apiCaller, nil)
	_, err := client.GetMinionReports()
	c.Assert(err, gc.ErrorMatches, `processing failed agents: "dave" is not a valid tag`)
}
