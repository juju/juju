// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"context"
	"reflect"
	stdtesting "testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/remoterelations"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

func TestRemoteRelationsSuite(t *stdtesting.T) {
	tc.Run(t, &remoteRelationsSuite{})
}

type remoteRelationsSuite struct {
	testhelpers.IsolationSuite

	remoteControllerInfo  *api.Info
	relationsFacade       *mockRelationsFacade
	remoteRelationsFacade *mockRemoteRelationsFacade
	config                remoterelations.Config
	stub                  *testhelpers.Stub
}

func (s *remoteRelationsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.remoteControllerInfo = &api.Info{
		Addrs:  []string{"1.2.3.4:1234"},
		CACert: coretesting.CACert,
	}

	s.stub = new(testhelpers.Stub)
	s.relationsFacade = newMockRelationsFacade(s.stub)
	s.remoteRelationsFacade = newMockRemoteRelationsFacade(s.stub)

	clk := testclock.NewClock(time.Time{})
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:         "remote-relations",
		Clock:        clk,
		IsFatal:      func(error) bool { return false },
		RestartDelay: time.Second,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.config = remoterelations.Config{
		ModelUUID:       "local-model-uuid",
		RelationsFacade: s.relationsFacade,
		NewRemoteModelFacadeFunc: func(context.Context, *api.Info) (remoterelations.RemoteModelRelationsFacadeCloser, error) {
			return s.remoteRelationsFacade, nil
		},
		Clock:  clk,
		Logger: loggertesting.WrapCheckLog(c),
		Runner: runner,
	}
}

func (s *remoteRelationsSuite) waitForWorkerStubCalls(c *tc.C, expected []testhelpers.StubCall) {
	waitForStubCalls(c, s.stub, expected)
}

func waitForStubCalls(c *tc.C, stub *testhelpers.Stub, expected []testhelpers.StubCall) {
	var calls []testhelpers.StubCall
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		calls = stub.Calls()
		if reflect.DeepEqual(calls, expected) {
			return
		}
	}
	c.Assert(expected, tc.DeepEquals, calls)
}

func (s *remoteRelationsSuite) assertRemoteApplicationWorkers(c *tc.C) worker.Worker {
	// Checks that the main worker loop responds to remote application events
	// by starting relevant relation watchers.
	s.relationsFacade.remoteApplications["db2"] = newMockRemoteApplication("db2", "db2url")
	s.relationsFacade.remoteApplications["mysql"] = newMockRemoteApplication("mysql", "mysqlurl")
	s.relationsFacade.controllerInfo["remote-model-uuid"] = s.remoteControllerInfo

	w, err := remoterelations.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	expected := []testhelpers.StubCall{
		{"WatchRemoteApplications", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"db2"}
	expected = []testhelpers.StubCall{
		{"RemoteApplications", []interface{}{[]string{"db2"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		{"WatchOfferStatus", []interface{}{"offer-db2-uuid", macaroon.Slice{mac}}},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}
	expected = []testhelpers.StubCall{
		{"RemoteApplications", []interface{}{[]string{"mysql"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"mysql"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		{"WatchOfferStatus", []interface{}{"offer-mysql-uuid", macaroon.Slice{mac}}},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	applicationNames := []string{"db2", "mysql"}
	for _, app := range applicationNames {
		w, ok := s.relationsFacade.remoteApplicationRelationsWatcher(app)
		c.Check(ok, tc.IsTrue)
		waitForStubCalls(c, &w.Stub, []testhelpers.StubCall{
			{"Changes", nil},
		})
	}
	return w
}

func (s *remoteRelationsSuite) TestRemoteApplicationWorkers(c *tc.C) {
	w := s.assertRemoteApplicationWorkers(c)
	workertest.CleanKill(c, w)

	// Check that relation watchers are stopped with the worker.
	applicationNames := []string{"db2", "mysql"}
	for _, app := range applicationNames {
		w, ok := s.relationsFacade.remoteApplicationRelationsWatcher(app)
		c.Check(ok, tc.IsTrue)
		c.Check(w.killed(), tc.IsTrue)
	}
}

func (s *remoteRelationsSuite) TestExternalControllerError(c *tc.C) {
	s.config.NewRemoteModelFacadeFunc = func(ctx context.Context, info *api.Info) (remoterelations.RemoteModelRelationsFacadeCloser, error) {
		return nil, errors.New("boom")
	}

	s.relationsFacade.remoteApplications["mysql"] = newMockRemoteApplication("mysql", "mysqlurl")
	s.relationsFacade.controllerInfo["remote-model-uuid"] = s.remoteControllerInfo

	w, err := remoterelations.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}
	expected := []testhelpers.StubCall{
		{"WatchRemoteApplications", nil},
		{"RemoteApplications", []interface{}{[]string{"mysql"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"mysql"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		{"SetRemoteApplicationStatus", []interface{}{
			"mysql", "error", "cannot connect to external controller: opening facade to remote model: boom",
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteApplicationWorkersRedirect(c *tc.C) {
	newControllerTag := names.NewControllerTag(uuid.MustNewUUID().String())

	s.config.NewRemoteModelFacadeFunc = func(ctx context.Context, info *api.Info) (remoterelations.RemoteModelRelationsFacadeCloser, error) {
		// If attempting to connect to the remote controller as defined in
		// SetUpTest, return a redirect error with a different address.
		if info.Addrs[0] == "1.2.3.4:1234" {
			return nil, &api.RedirectError{
				Servers:         []network.MachineHostPorts{network.NewMachineHostPorts(2345, "2.3.4.5")},
				CACert:          "new-controller-cert",
				FollowRedirect:  false,
				ControllerTag:   newControllerTag,
				ControllerAlias: "",
			}
		}

		// The address we asked to connect has changed;
		// represent a successful connection.
		return s.remoteRelationsFacade, nil
	}

	s.relationsFacade.remoteApplications["mysql"] = newMockRemoteApplication("mysql", "mysqlurl")
	s.relationsFacade.controllerInfo["remote-model-uuid"] = s.remoteControllerInfo

	w, err := remoterelations.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)

	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}
	expected := []testhelpers.StubCall{
		{"WatchRemoteApplications", nil},
		{"RemoteApplications", []interface{}{[]string{"mysql"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"mysql"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		// We expect a redirect error will cause the new details to be saved.
		{"UpdateControllerForModel", []interface{}{
			crossmodel.ControllerInfo{
				ControllerUUID: newControllerTag.Id(),
				Alias:          "",
				Addrs:          []string{"2.3.4.5:2345"},
				CACert:         "new-controller-cert",
			},
			"remote-model-uuid"},
		},
		{"WatchOfferStatus", []interface{}{"offer-mysql-uuid", macaroon.Slice{mac}}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteApplicationWorkersRedirectControllerUpdateError(c *tc.C) {
	s.stub.SetErrors(nil, nil, nil, nil, errors.New("busted"))

	newControllerTag := names.NewControllerTag(uuid.MustNewUUID().String())

	s.config.NewRemoteModelFacadeFunc = func(ctx context.Context, info *api.Info) (remoterelations.RemoteModelRelationsFacadeCloser, error) {
		// If attempting to connect to the remote controller as defined in
		// SetUpTest, return a redirect error with a different address.
		if info.Addrs[0] == "1.2.3.4:1234" {
			return nil, &api.RedirectError{
				Servers:         []network.MachineHostPorts{network.NewMachineHostPorts(2345, "2.3.4.5")},
				CACert:          "new-controller-cert",
				FollowRedirect:  false,
				ControllerTag:   newControllerTag,
				ControllerAlias: "",
			}
		}

		// The address we asked to connect has changed;
		// represent a successful connection.
		return s.remoteRelationsFacade, nil
	}

	s.relationsFacade.remoteApplications["mysql"] = newMockRemoteApplication("mysql", "mysqlurl")
	s.relationsFacade.controllerInfo["remote-model-uuid"] = s.remoteControllerInfo

	w, err := remoterelations.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}
	expected := []testhelpers.StubCall{
		{"WatchRemoteApplications", nil},
		{"RemoteApplications", []interface{}{[]string{"mysql"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"mysql"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		// We expect a redirect error will cause the new details to be saved,
		// But this call returns an error.
		{"UpdateControllerForModel", []interface{}{
			crossmodel.ControllerInfo{
				ControllerUUID: newControllerTag.Id(),
				Alias:          "",
				Addrs:          []string{"2.3.4.5:2345"},
				CACert:         "new-controller-cert",
			},
			"remote-model-uuid"},
		},
		{"Close", nil},
		{"SetRemoteApplicationStatus", []interface{}{"mysql", "error",
			"cannot connect to external controller: opening facade to remote model: updating external controller info: busted"}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteApplicationRemoved(c *tc.C) {
	// Checks that when a remote application is removed, the application worker
	// and relation worker are killed.
	w := s.assertRemoteApplicationWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	appWorker, err := s.config.Runner.Worker("mysql", nil)
	c.Assert(err, tc.ErrorIsNil)

	relWatcher, _ := s.relationsFacade.removeApplication("mysql")
	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, ok := s.relationsFacade.remoteApplicationRelationsWatcher("mysql")
		if !ok {
			break
		}
	}
	c.Check(relWatcher.killed(), tc.IsTrue)
	expected := []testhelpers.StubCall{
		{"RemoteApplications", []interface{}{[]string{"mysql"}}},
		{"Close", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
	err = workertest.CheckKilled(c, appWorker)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteRelationsSuite) TestRemoteApplicationTerminated(c *tc.C) {
	// Checks that when a remote offer is terminated, the application worker
	// and relation worker are killed.
	w := s.assertRemoteApplicationWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	appWorker, err := s.config.Runner.Worker("mysql", nil)
	c.Assert(err, tc.ErrorIsNil)

	relWatcher, ok := s.relationsFacade.remoteApplicationRelationsWatchers["mysql"]
	c.Assert(ok, tc.IsTrue)
	s.relationsFacade.remoteApplications["mysql"].status = status.Terminated
	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}

	expected := []testhelpers.StubCall{
		{"RemoteApplications", []interface{}{[]string{"mysql"}}},
		{"Close", nil},
	}
	s.waitForWorkerStubCalls(c, expected)

	c.Check(relWatcher.killed(), tc.IsTrue)
	err = workertest.CheckKilled(c, appWorker)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteRelationsSuite) TestRemoteApplicationOfferChanged(c *tc.C) {
	// Checks that when a remote offer now different to the previous one being used,
	// the application worker and relation worker are killed.
	w := s.assertRemoteApplicationWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	oldAppWorker, err := s.config.Runner.Worker("mysql", nil)
	c.Assert(err, tc.ErrorIsNil)

	relWatcher, ok := s.relationsFacade.remoteApplicationRelationsWatchers["mysql"]
	c.Assert(ok, tc.IsTrue)
	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)

	s.relationsFacade.remoteApplications["mysql"].offeruuid = "different-uuid"
	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}

	expected := []testhelpers.StubCall{
		{"RemoteApplications", []interface{}{[]string{"mysql"}}},
		{"Close", nil},
		{"WatchRemoteApplicationRelations", []interface{}{"mysql"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		{"WatchOfferStatus", []interface{}{"different-uuid", macaroon.Slice{mac}}},
	}
	s.waitForWorkerStubCalls(c, expected)

	c.Check(relWatcher.killed(), tc.IsTrue)
	err = workertest.CheckKilled(c, oldAppWorker)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteRelationsSuite) TestRemoteNotFoundTerminatesOnWatching(c *tc.C) {
	s.relationsFacade.remoteApplications["db2"] = newMockRemoteApplication("db2", "db2url")
	s.relationsFacade.remoteApplications["mysql"] = newMockRemoteApplication("mysql", "mysqlurl")
	s.relationsFacade.controllerInfo["remote-model-uuid"] = s.remoteControllerInfo

	w, err := remoterelations.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	expected := []testhelpers.StubCall{
		{"WatchRemoteApplications", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	s.stub.SetErrors(nil, nil, nil, params.Error{Code: params.CodeNotFound})

	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"db2"}
	expected = []testhelpers.StubCall{
		{"RemoteApplications", []interface{}{[]string{"db2"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		{"WatchOfferStatus", []interface{}{"offer-db2-uuid", macaroon.Slice{mac}}},
		{"SetRemoteApplicationStatus", []interface{}{"db2", "terminated", "offer has been removed"}},
		{"Close", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestOfferStatusChange(c *tc.C) {
	w := s.assertRemoteApplicationWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	statusWatcher := s.remoteRelationsFacade.offersStatusWatchers["offer-mysql-uuid"]
	statusWatcher.changes <- []watcher.OfferStatusChange{{
		Name: "mysql",
		Status: status.StatusInfo{
			Status:  status.Active,
			Message: "started",
		},
	}}

	expected := []testhelpers.StubCall{
		{"SetRemoteApplicationStatus", []interface{}{"mysql", "active", "started"}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestOfferStatusTerminatedStopsWatcher(c *tc.C) {
	w := s.assertRemoteApplicationWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	statusWatcher := s.remoteRelationsFacade.offersStatusWatchers["offer-mysql-uuid"]
	statusWatcher.changes <- []watcher.OfferStatusChange{{
		Name: "mysql",
		Status: status.StatusInfo{
			Status: status.Terminated,
		},
	}}

	expected := []testhelpers.StubCall{
		{"SetRemoteApplicationStatus", []interface{}{"mysql", "terminated", ""}},
	}
	s.waitForWorkerStubCalls(c, expected)
	c.Check(statusWatcher.killed(), tc.IsTrue)
}

func (s *remoteRelationsSuite) TestRemoteNotFoundTerminatesOnChange(c *tc.C) {
	s.relationsFacade.relations["db2:db django:db"] = newMockRelation(123)
	w := s.assertRemoteApplicationWorkers(c)
	defer workertest.CleanKill(c, w)

	s.stub.ResetCalls()
	s.stub.SetErrors(nil, nil, params.Error{Code: params.CodeNotFound})

	s.relationsFacade.relationsEndpoints["db2:db django:db"] = &relationEndpointInfo{
		localApplicationName: "django",
		localEndpoint: params.RemoteEndpoint{
			Name:      "db2",
			Role:      "requires",
			Interface: "db2",
		},
		remoteEndpointName: "data",
	}

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	relTag := names.NewRelationTag("db2:db django:db")
	expected := []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
		{"ExportEntities", []interface{}{
			[]names.Tag{names.NewApplicationTag("django"), relTag}}},
		{"RegisterRemoteRelations", []interface{}{[]params.RegisterRemoteRelationArg{{
			ApplicationToken: "token-django",
			SourceModelTag:   "model-local-model-uuid",
			RelationToken:    "token-db2:db django:db",
			RemoteEndpoint: params.RemoteEndpoint{
				Name:      "db2",
				Role:      "requires",
				Interface: "db2",
			},
			OfferUUID:         "offer-db2-uuid",
			ConsumeVersion:    666,
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteWatcherNotFoundError(c *tc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)

	s.stub.ResetCalls()

	s.relationsFacade.relationsEndpoints["db2:db django:db"] = &relationEndpointInfo{
		localApplicationName: "django",
		localEndpoint: params.RemoteEndpoint{
			Name:      "db2",
			Role:      "requires",
			Interface: "db2",
		},
		remoteEndpointName: "data",
	}

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	expected := []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
	}
	changeWatcher, ok := s.remoteRelationsFacade.remoteRelationWatcher("token-db2:db django:db")
	c.Check(ok, tc.IsTrue)
	changeWatcher.kill(params.Error{Code: params.CodeNotFound})
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) assertRemoteRelationsWorkers(c *tc.C) worker.Worker {
	s.relationsFacade.relations["db2:db django:db"] = newMockRelation(123)
	w := s.assertRemoteApplicationWorkers(c)
	s.stub.ResetCalls()

	s.relationsFacade.relationsEndpoints["db2:db django:db"] = &relationEndpointInfo{
		localApplicationName: "django",
		localEndpoint: params.RemoteEndpoint{
			Name:      "db2",
			Role:      "requires",
			Interface: "db2",
		},
		remoteEndpointName: "data",
	}

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	apiMac, err := testing.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
	relTag := names.NewRelationTag("db2:db django:db")
	expected := []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
		{"ExportEntities", []interface{}{
			[]names.Tag{names.NewApplicationTag("django"), relTag}}},
		{"RegisterRemoteRelations", []interface{}{[]params.RegisterRemoteRelationArg{{
			ApplicationToken: "token-django",
			SourceModelTag:   "model-local-model-uuid",
			RelationToken:    "token-db2:db django:db",
			RemoteEndpoint: params.RemoteEndpoint{
				Name:      "db2",
				Role:      "requires",
				Interface: "db2",
			},
			OfferUUID:         "offer-db2-uuid",
			ConsumeVersion:    666,
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SaveMacaroon", []interface{}{relTag, apiMac}},
		{"ImportRemoteEntity", []interface{}{names.NewApplicationTag("db2"), "token-offer-db2-uuid"}},
		{"WatchRelationSuspendedStatus", []interface{}{"token-db2:db django:db", macaroon.Slice{apiMac}}},
		{"WatchLocalRelationChanges", []interface{}{"db2:db django:db"}},
		{"WatchRelationChanges", []interface{}{"token-db2:db django:db", "token-offer-db2-uuid", macaroon.Slice{apiMac}}},
		{"WatchConsumedSecretsChanges", []interface{}{"token-django", "token-db2:db django:db", mac}},
	}
	s.waitForWorkerStubCalls(c, expected)

	changeWatcher, ok := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
	c.Check(ok, tc.IsTrue)
	waitForStubCalls(c, &changeWatcher.Stub, []testhelpers.StubCall{
		{"Changes", nil},
	})
	changeWatcher, ok = s.remoteRelationsFacade.remoteRelationWatcher("token-db2:db django:db")
	c.Check(ok, tc.IsTrue)
	waitForStubCalls(c, &changeWatcher.Stub, []testhelpers.StubCall{
		{"Changes", nil},
	})
	relationStatusWatcher, ok := s.remoteRelationsFacade.relationsStatusWatcher("token-db2:db django:db")
	c.Check(ok, tc.IsTrue)
	waitForStubCalls(c, &relationStatusWatcher.Stub, []testhelpers.StubCall{
		{"Changes", nil},
	})
	return w
}

func (s *remoteRelationsSuite) TestRemoteRelationsWorkers(c *tc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	workertest.CleanKill(c, w)

	// Check that relation unit watchers are stopped with the worker.
	relWatcher, ok := s.relationsFacade.remoteRelationWatchers["db2:db django:db"]
	c.Check(ok, tc.IsTrue)
	c.Check(relWatcher.killed(), tc.IsTrue)

	relWatcher, ok = s.remoteRelationsFacade.remoteRelationWatchers["token-db2:db django:db"]
	c.Check(ok, tc.IsTrue)
	c.Check(relWatcher.killed(), tc.IsTrue)
}

func (s *remoteRelationsSuite) TestRemoteRelationsRevoked(c *tc.C) {
	// The consume permission is revoked after an offer is consumed.
	// Subsequent api calls against that offer will fail and record an
	// error in the local model.
	s.relationsFacade.relations["db2:db django:db"] = newMockRelation(123)
	w := s.assertRemoteApplicationWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()
	s.stub.SetErrors(nil, nil, &params.Error{
		Code:    params.CodeDischargeRequired,
		Message: "message",
	})

	s.relationsFacade.relationsEndpoints["db2:db django:db"] = &relationEndpointInfo{
		localApplicationName: "django",
		localEndpoint: params.RemoteEndpoint{
			Name:      "db2",
			Role:      "requires",
			Interface: "db2",
		},
		remoteEndpointName: "data",
	}

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	relTag := names.NewRelationTag("db2:db django:db")
	expected := []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
		{"ExportEntities", []interface{}{
			[]names.Tag{names.NewApplicationTag("django"), relTag}}},
		{"RegisterRemoteRelations", []interface{}{[]params.RegisterRemoteRelationArg{{
			ApplicationToken: "token-django",
			SourceModelTag:   "model-local-model-uuid",
			RelationToken:    "token-db2:db django:db",
			RemoteEndpoint: params.RemoteEndpoint{
				Name:      "db2",
				Role:      "requires",
				Interface: "db2",
			},
			OfferUUID:         "offer-db2-uuid",
			ConsumeVersion:    666,
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SetRemoteApplicationStatus", []interface{}{"db2", "error", "message"}},
		{"Close", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationsDying(c *tc.C) {
	// Checks that when a remote relation dies, the relation units
	// workers are killed.
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.relationsFacade.updateRelationLife("db2:db django:db", life.Dying)
	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, ok := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
		if ok {
			continue
		}
		_, ok = s.remoteRelationsFacade.remoteRelationWatcher("token-db2:db django:db")
		if !ok {
			break
		}
		_, ok = s.remoteRelationsFacade.relationsStatusWatcher("token-db2:db django:db")
		if !ok {
			break
		}
	}
	// We keep the relation units watcher alive when the relation
	// goes to Dying; they're only stopped when the relation is
	// finally removed.
	c.Assert(unitsWatcher.killed(), tc.IsFalse)
	apiMac, err := testing.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	relTag := names.NewRelationTag("db2:db django:db")
	expected := []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
		{"ExportEntities", []interface{}{
			[]names.Tag{names.NewApplicationTag("django"), relTag}}},
		{"RegisterRemoteRelations", []interface{}{[]params.RegisterRemoteRelationArg{{
			ApplicationToken: "token-django",
			SourceModelTag:   "model-local-model-uuid",
			RelationToken:    "token-db2:db django:db",
			RemoteEndpoint: params.RemoteEndpoint{
				Name:      "db2",
				Role:      "requires",
				Interface: "db2",
			},
			OfferUUID:         "offer-db2-uuid",
			ConsumeVersion:    666,
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SaveMacaroon", []interface{}{relTag, apiMac}},
		{"ImportRemoteEntity", []interface{}{names.NewApplicationTag("db2"), "token-offer-db2-uuid"}},
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				Life:                    life.Dying,
				ApplicationOrOfferToken: "token-django",
				RelationToken:           "token-db2:db django:db",
				Macaroons:               macaroon.Slice{apiMac},
				BakeryVersion:           bakery.LatestVersion,
			},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func intPtr(i int) *int {
	return &i
}

func (s *remoteRelationsSuite) TestLocalRelationsRemoved(c *tc.C) {
	// Checks that when a remote relation goes away the relation units worker is killed.
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		RelationToken:           "token-db2:db django:db",
		ApplicationOrOfferToken: "token-django",
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: map[string]interface{}{"foo": "bar"},
		}, {
			UnitId:   2,
			Settings: map[string]interface{}{"foo": "baz"},
		}},
		UnitCount: intPtr(2),
	}

	mac, err := testing.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
	expected := []testhelpers.StubCall{
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationOrOfferToken: "token-django",
				RelationToken:           "token-db2:db django:db",
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   1,
					Settings: map[string]interface{}{"foo": "bar"},
				}, {
					UnitId:   2,
					Settings: map[string]interface{}{"foo": "baz"},
				}},
				UnitCount:     intPtr(2),
				Macaroons:     macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	unitsWatcher, _ = s.relationsFacade.removeRelation("db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		RelationToken:           "token-db2:db django:db",
		ApplicationOrOfferToken: "token-django",
		DepartedUnits:           []int{1},
		UnitCount:               intPtr(1),
	}

	expected = []testhelpers.StubCall{
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationOrOfferToken: "token-django",
				RelationToken:           "token-db2:db django:db",
				DepartedUnits:           []int{1},
				UnitCount:               intPtr(1),
				Macaroons:               macaroon.Slice{mac},
				BakeryVersion:           bakery.LatestVersion,
			},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	// Remove relation before we receive the final unit change event.
	unitsWatcher, _ = s.relationsFacade.removeRelation("db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		RelationToken:           "token-db2:db django:db",
		ApplicationOrOfferToken: "token-django",
		DepartedUnits:           []int{2},
		UnitCount:               intPtr(0),
	}

	expected = []testhelpers.StubCall{
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationOrOfferToken: "token-django",
				RelationToken:           "token-db2:db django:db",
				DepartedUnits:           []int{2},
				UnitCount:               intPtr(0),
				Macaroons:               macaroon.Slice{mac},
				BakeryVersion:           bakery.LatestVersion,
			},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, ok := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
		if !ok {
			break
		}
	}
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		RelationToken:           "token-db2:db django:db",
		ApplicationOrOfferToken: "token-django",
		DepartedUnits:           []int{2},
	}
	c.Assert(unitsWatcher.killed(), tc.IsTrue)
	expected = []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestLocalRelationsChangedNotifies(c *tc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		RelationToken:           "token-db2:db django:db",
		ApplicationOrOfferToken: "token-django",
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: map[string]interface{}{"foo": "bar"},
		}},
		DepartedUnits: []int{2},
	}

	mac, err := testing.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
	expected := []testhelpers.StubCall{
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationOrOfferToken: "token-django",
				RelationToken:           "token-db2:db django:db",
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   1,
					Settings: map[string]interface{}{"foo": "bar"},
				}},
				DepartedUnits: []int{2},
				Macaroons:     macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteNotFoundTerminatesOnPublish(c *tc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	s.stub.SetErrors(params.Error{Code: params.CodeNotFound})

	unitsWatcher, _ := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		ApplicationOrOfferToken: "token-django",
		RelationToken:           "token-db2:db django:db",
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: map[string]interface{}{"foo": "bar"},
		}},
		DepartedUnits: []int{2},
	}

	mac, err := testing.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
	expected := []testhelpers.StubCall{
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationOrOfferToken: "token-django",
				RelationToken:           "token-db2:db django:db",
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   1,
					Settings: map[string]interface{}{"foo": "bar"},
				}},
				DepartedUnits: []int{2},
				Macaroons:     macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationsChangedConsumes(c *tc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.remoteRelationsFacade.remoteRelationWatcher("token-db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		ApplicationOrOfferToken: "token-offer-db2-uuid",
		RelationToken:           "token-db2:db django:db",
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: map[string]interface{}{"foo": "bar"},
		}},
		DepartedUnits: []int{2},
	}

	mac, err := testing.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
	expected := []testhelpers.StubCall{
		{"ConsumeRemoteRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationOrOfferToken: "token-offer-db2-uuid",
				RelationToken:           "token-db2:db django:db",
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   1,
					Settings: map[string]interface{}{"foo": "bar"},
				}},
				DepartedUnits: []int{2},
				Macaroons:     macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationsDyingConsumes(c *tc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	statusWatcher, _ := s.remoteRelationsFacade.relationsStatusWatcher("token-db2:db django:db")
	statusWatcher.changes <- []watcher.RelationStatusChange{{
		Life: life.Dying,
	}}

	suspended := false
	expected := []testhelpers.StubCall{
		{"ConsumeRemoteRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				Life:                    life.Dying,
				ApplicationOrOfferToken: "token-offer-db2-uuid",
				RelationToken:           "token-db2:db django:db",
				Suspended:               &suspended,
			},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationsChangedError(c *tc.C) {
	s.assertRemoteRelationsChangedError(c, false)
}

func (s *remoteRelationsSuite) TestRemoteDyingRelationsChangedError(c *tc.C) {
	s.assertRemoteRelationsChangedError(c, true)
}

func (s *remoteRelationsSuite) assertRemoteRelationsChangedError(c *tc.C, dying bool) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	s.stub.SetErrors(errors.New("failed"))
	unitsWatcher, _ := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		ApplicationOrOfferToken: "token-django",
		RelationToken:           "token-db2:db django:db",
		DepartedUnits:           []int{1},
	}

	// The error causes relation change publication to fail.
	apiMac, err := testing.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
	expected := []testhelpers.StubCall{
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationOrOfferToken: "token-django",
				RelationToken:           "token-db2:db django:db",
				DepartedUnits:           []int{1},
				Macaroons:               macaroon.Slice{apiMac},
				BakeryVersion:           bakery.LatestVersion,
			},
		}},
		{"Close", nil},
	}

	s.waitForWorkerStubCalls(c, expected)
	// An error in one of the units watchers does not kill the parent worker.
	workertest.CheckAlive(c, w)

	// Allow the worker to resume.
	s.stub.SetErrors(nil)
	s.stub.ResetCalls()
	s.config.Clock.(*testclock.Clock).WaitAdvance(50*time.Second, coretesting.LongWait, 1)
	// Not resumed yet.
	c.Assert(s.stub.Calls(), tc.HasLen, 0)
	s.config.Clock.(*testclock.Clock).WaitAdvance(10*time.Second, coretesting.LongWait, 1)

	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	expected = []testhelpers.StubCall{
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		{"WatchOfferStatus", []interface{}{"offer-db2-uuid", macaroon.Slice{mac}}},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	relTag := names.NewRelationTag("db2:db django:db")
	expected = []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
		{"ExportEntities", []interface{}{
			[]names.Tag{names.NewApplicationTag("django"), relTag}}},
		{"RegisterRemoteRelations", []interface{}{[]params.RegisterRemoteRelationArg{{
			ApplicationToken: "token-django",
			SourceModelTag:   "model-local-model-uuid",
			RelationToken:    "token-db2:db django:db",
			RemoteEndpoint: params.RemoteEndpoint{
				Name:      "db2",
				Role:      "requires",
				Interface: "db2",
			},
			OfferUUID:         "offer-db2-uuid",
			ConsumeVersion:    666,
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SaveMacaroon", []interface{}{relTag, apiMac}},
		{"ImportRemoteEntity", []interface{}{names.NewApplicationTag("db2"), "token-offer-db2-uuid"}},
		{"WatchRelationSuspendedStatus", []interface{}{"token-db2:db django:db", macaroon.Slice{apiMac}}},
		{"WatchLocalRelationChanges", []interface{}{"db2:db django:db"}},
		{"WatchRelationChanges", []interface{}{"token-db2:db django:db", "token-offer-db2-uuid", macaroon.Slice{apiMac}}},
		{"WatchConsumedSecretsChanges", []interface{}{"token-django", "token-db2:db django:db", mac}},
	}

	// If a relation is dying and there's been an error, when processing resumes
	// a cleanup is forced on the remote side.
	if dying {
		s.relationsFacade.updateRelationLife("db2:db django:db", life.Dying)
		forceCleanup := true
		expected = append(expected, testhelpers.StubCall{
			FuncName: "PublishRelationChange",
			Args: []interface{}{
				params.RemoteRelationChangeEvent{
					ApplicationOrOfferToken: "token-django",
					RelationToken:           "token-db2:db django:db",
					Life:                    life.Dying,
					Macaroons:               macaroon.Slice{apiMac},
					BakeryVersion:           bakery.LatestVersion,
					ForceCleanup:            &forceCleanup,
				},
			}},
		)
	}

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	// After the worker resumes, normal processing happens.
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteSecretChangedConsumes(c *tc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	change := watcher.SecretRevisionChange{
		URI:      secrets.NewURI(),
		Revision: 666,
	}
	secretsWatcher, _ := s.remoteRelationsFacade.secretsRevisionWatcher("token-django")
	secretsWatcher.changes <- []watcher.SecretRevisionChange{change}

	expected := []testhelpers.StubCall{
		{"ConsumeRemoteSecretChanges", []interface{}{
			[]watcher.SecretRevisionChange{change},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteSecretNotImplemented(c *tc.C) {
	s.relationsFacade.relations["db2:db django:db"] = newMockRelation(123)
	w := s.assertRemoteApplicationWorkers(c)
	s.stub.ResetCalls()

	s.stub.SetErrors(nil, nil, nil, nil, nil, nil, nil, nil, errors.NotImplemented)

	s.relationsFacade.relationsEndpoints["db2:db django:db"] = &relationEndpointInfo{
		localApplicationName: "django",
		localEndpoint: params.RemoteEndpoint{
			Name:      "db2",
			Role:      "requires",
			Interface: "db2",
		},
		remoteEndpointName: "data",
	}

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	apiMac, err := testing.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
	relTag := names.NewRelationTag("db2:db django:db")
	expected := []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
		{"ExportEntities", []interface{}{
			[]names.Tag{names.NewApplicationTag("django"), relTag}}},
		{"RegisterRemoteRelations", []interface{}{[]params.RegisterRemoteRelationArg{{
			ApplicationToken: "token-django",
			SourceModelTag:   "model-local-model-uuid",
			RelationToken:    "token-db2:db django:db",
			RemoteEndpoint: params.RemoteEndpoint{
				Name:      "db2",
				Role:      "requires",
				Interface: "db2",
			},
			OfferUUID:         "offer-db2-uuid",
			ConsumeVersion:    666,
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SaveMacaroon", []interface{}{relTag, apiMac}},
		{"ImportRemoteEntity", []interface{}{names.NewApplicationTag("db2"), "token-offer-db2-uuid"}},
		{"WatchRelationSuspendedStatus", []interface{}{"token-db2:db django:db", macaroon.Slice{apiMac}}},
		{"WatchLocalRelationChanges", []interface{}{"db2:db django:db"}},
		{"WatchRelationChanges", []interface{}{"token-db2:db django:db", "token-offer-db2-uuid", macaroon.Slice{apiMac}}},
		{"WatchConsumedSecretsChanges", []interface{}{"token-django", "token-db2:db django:db", mac}},
	}
	s.waitForWorkerStubCalls(c, expected)

	changeWatcher, ok := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
	c.Check(ok, tc.IsTrue)
	waitForStubCalls(c, &changeWatcher.Stub, []testhelpers.StubCall{
		{"Changes", nil},
	})
	changeWatcher, ok = s.remoteRelationsFacade.remoteRelationWatcher("token-db2:db django:db")
	c.Check(ok, tc.IsTrue)
	waitForStubCalls(c, &changeWatcher.Stub, []testhelpers.StubCall{
		{"Changes", nil},
	})
	relationStatusWatcher, ok := s.remoteRelationsFacade.relationsStatusWatcher("token-db2:db django:db")
	c.Check(ok, tc.IsTrue)
	waitForStubCalls(c, &relationStatusWatcher.Stub, []testhelpers.StubCall{
		{"Changes", nil},
	})
	workertest.CleanKill(c, w)
}

func (s *remoteRelationsSuite) TestRegisteredApplicationNotRegistered(c *tc.C) {
	s.relationsFacade.relations["db2:db django:db"] = newMockRelation(123)
	db2app := newMockRemoteApplication("db2", "db2url")
	db2app.registered = true
	s.relationsFacade.remoteApplications["db2"] = db2app
	applicationNames := []string{"db2"}
	s.relationsFacade.remoteApplicationsWatcher.changes <- applicationNames

	w, err := remoterelations.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	expected := []testhelpers.StubCall{
		{"WatchRemoteApplications", nil},
		{"RemoteApplications", []interface{}{[]string{"db2"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	s.relationsFacade.relationsEndpoints["db2:db django:db"] = &relationEndpointInfo{
		localApplicationName: "django",
		localEndpoint: params.RemoteEndpoint{
			Name:      "db2",
			Role:      "requires",
			Interface: "db2",
		},
		remoteEndpointName: "data",
	}

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	expected = []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationSuspended(c *tc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	// First suspend the relation.
	s.relationsFacade.relations["db2:db django:db"].SetSuspended(true)
	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	expected := []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
	}
	s.waitForWorkerStubCalls(c, expected)
	unitsWatcher, ok := s.relationsFacade.remoteRelationWatchers["db2:db django:db"]
	c.Assert(ok, tc.IsTrue)
	c.Assert(unitsWatcher.killed(), tc.IsTrue)
	s.stub.ResetCalls()

	// Now resume the relation.
	s.relationsFacade.relations["db2:db django:db"].SetSuspended(false)
	relWatcher.changes <- []string{"db2:db django:db"}

	mac, err := testing.NewMacaroon("test")
	c.Assert(err, tc.ErrorIsNil)
	apiMac, err := testing.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)

	relTag := names.NewRelationTag("db2:db django:db")
	// When resuming, it's similar to setting things up for a new relation
	// except that the call to create te life/status listener is missing.
	expected = []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
		{"ExportEntities", []interface{}{
			[]names.Tag{names.NewApplicationTag("django"), relTag}}},
		{"RegisterRemoteRelations", []interface{}{[]params.RegisterRemoteRelationArg{{
			ApplicationToken: "token-django",
			SourceModelTag:   "model-local-model-uuid",
			RelationToken:    "token-db2:db django:db",
			RemoteEndpoint: params.RemoteEndpoint{
				Name:      "db2",
				Role:      "requires",
				Interface: "db2",
			},
			OfferUUID:         "offer-db2-uuid",
			ConsumeVersion:    666,
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SaveMacaroon", []interface{}{relTag, apiMac}},
		{"ImportRemoteEntity", []interface{}{names.NewApplicationTag("db2"), "token-offer-db2-uuid"}},
		{"WatchLocalRelationChanges", []interface{}{"db2:db django:db"}},
		{"WatchRelationChanges", []interface{}{"token-db2:db django:db", "token-offer-db2-uuid", macaroon.Slice{apiMac}}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestDyingRelationSuspended(c *tc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	s.relationsFacade.relations["db2:db django:db"].life = life.Dying
	s.relationsFacade.relations["db2:db django:db"].SetSuspended(true)
	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	expected := []testhelpers.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
	}
	s.waitForWorkerStubCalls(c, expected)
	// Suspending a dying relation does not stop workers.
	unitsWatcher, ok := s.relationsFacade.remoteRelationWatchers["db2:db django:db"]
	c.Assert(ok, tc.IsTrue)
	c.Assert(unitsWatcher.killed(), tc.IsFalse)
}
