// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"reflect"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/remoterelations"
)

var _ = gc.Suite(&remoteRelationsSuite{})

type remoteRelationsSuite struct {
	coretesting.BaseSuite

	remoteControllerInfo  *api.Info
	resources             *common.Resources
	authorizer            *apiservertesting.FakeAuthorizer
	relationsFacade       *mockRelationsFacade
	remoteRelationsFacade *mockRemoteRelationsFacade
	config                remoterelations.Config
	stub                  *jujutesting.Stub
}

func (s *remoteRelationsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.remoteControllerInfo = &api.Info{
		Addrs:  []string{"1.2.3.4:1234"},
		CACert: coretesting.CACert,
	}

	s.stub = new(jujutesting.Stub)
	s.relationsFacade = newMockRelationsFacade(s.stub)
	s.remoteRelationsFacade = newMockRemoteRelationsFacade(s.stub)

	s.config = remoterelations.Config{
		ModelUUID:       "local-model-uuid",
		RelationsFacade: s.relationsFacade,
		NewRemoteModelFacadeFunc: func(*api.Info) (remoterelations.RemoteModelRelationsFacadeCloser, error) {
			return s.remoteRelationsFacade, nil
		},
		Clock:  testclock.NewClock(time.Time{}),
		Logger: loggo.GetLogger("test"),
	}
}

func (s *remoteRelationsSuite) waitForWorkerStubCalls(c *gc.C, expected []jujutesting.StubCall) {
	waitForStubCalls(c, s.stub, expected)
}

func waitForStubCalls(c *gc.C, stub *jujutesting.Stub, expected []jujutesting.StubCall) {
	var calls []jujutesting.StubCall
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		calls = stub.Calls()
		if reflect.DeepEqual(calls, expected) {
			return
		}
	}
	c.Fatalf("failed to see expected calls.\nexpected: %v\nobserved: %v", expected, calls)
}

func (s *remoteRelationsSuite) assertRemoteApplicationWorkers(c *gc.C) worker.Worker {
	// Checks that the main worker loop responds to remote application events
	// by starting relevant relation watchers.
	s.relationsFacade.remoteApplications["db2"] = newMockRemoteApplication("db2", "db2url")
	s.relationsFacade.remoteApplications["mysql"] = newMockRemoteApplication("mysql", "mysqlurl")
	s.relationsFacade.controllerInfo["remote-model-uuid"] = s.remoteControllerInfo

	w, err := remoterelations.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	expected := []jujutesting.StubCall{
		{"WatchRemoteApplications", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"db2"}
	expected = []jujutesting.StubCall{
		{"RemoteApplications", []interface{}{[]string{"db2"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		{"WatchOfferStatus", []interface{}{"offer-db2-uuid", macaroon.Slice{mac}}},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}
	expected = []jujutesting.StubCall{
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
		c.Check(ok, jc.IsTrue)
		waitForStubCalls(c, &w.Stub, []jujutesting.StubCall{
			{"Changes", nil},
		})
	}
	return w
}

func (s *remoteRelationsSuite) TestRemoteApplicationWorkers(c *gc.C) {
	w := s.assertRemoteApplicationWorkers(c)
	workertest.CleanKill(c, w)

	// Check that relation watchers are stopped with the worker.
	applicationNames := []string{"db2", "mysql"}
	for _, app := range applicationNames {
		w, ok := s.relationsFacade.remoteApplicationRelationsWatcher(app)
		c.Check(ok, jc.IsTrue)
		c.Check(w.killed(), jc.IsTrue)
	}
}

func (s *remoteRelationsSuite) TestRemoteApplicationWorkersRedirect(c *gc.C) {
	newControllerTag := names.NewControllerTag(utils.MustNewUUID().String())

	s.config.NewRemoteModelFacadeFunc = func(info *api.Info) (remoterelations.RemoteModelRelationsFacadeCloser, error) {
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
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)

	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}
	expected := []jujutesting.StubCall{
		{"WatchRemoteApplications", nil},
		{"RemoteApplications", []interface{}{[]string{"mysql"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"mysql"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		// We expect a redirect error will cause the new details to be saved.
		{"UpdateControllerForModel", []interface{}{
			crossmodel.ControllerInfo{
				ControllerTag: newControllerTag,
				Alias:         "",
				Addrs:         []string{"2.3.4.5:2345"},
				CACert:        "new-controller-cert",
			},
			"remote-model-uuid"},
		},
		{"WatchOfferStatus", []interface{}{"offer-mysql-uuid", macaroon.Slice{mac}}},
	}
	s.waitForWorkerStubCalls(c, expected)

}

func (s *remoteRelationsSuite) TestRemoteApplicationRemoved(c *gc.C) {
	// Checks that when a remote application is removed, the relation
	// worker is killed.
	w := s.assertRemoteApplicationWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	relWatcher, _ := s.relationsFacade.removeApplication("mysql")
	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, ok := s.relationsFacade.remoteApplicationRelationsWatcher("mysql")
		if !ok {
			break
		}
	}
	c.Check(relWatcher.killed(), jc.IsTrue)
	expected := []jujutesting.StubCall{
		{"RemoteApplications", []interface{}{[]string{"mysql"}}},
		{"Close", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteNotFoundTerminatesOnWatching(c *gc.C) {
	s.relationsFacade.remoteApplications["db2"] = newMockRemoteApplication("db2", "db2url")
	s.relationsFacade.remoteApplications["mysql"] = newMockRemoteApplication("mysql", "mysqlurl")
	s.relationsFacade.controllerInfo["remote-model-uuid"] = s.remoteControllerInfo

	w, err := remoterelations.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	expected := []jujutesting.StubCall{
		{"WatchRemoteApplications", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	s.stub.SetErrors(nil, nil, nil, params.Error{Code: params.CodeNotFound})

	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"db2"}
	expected = []jujutesting.StubCall{
		{"RemoteApplications", []interface{}{[]string{"db2"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		{"WatchOfferStatus", []interface{}{"offer-db2-uuid", macaroon.Slice{mac}}},
		{"SetRemoteApplicationStatus", []interface{}{"db2", "terminated", "offer has been removed"}},
		{"Close", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestOfferStatusChange(c *gc.C) {
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

	expected := []jujutesting.StubCall{
		{"SetRemoteApplicationStatus", []interface{}{"mysql", "active", "started"}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteNotFoundTerminatesOnChange(c *gc.C) {
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

	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	relTag := names.NewRelationTag("db2:db django:db")
	expected := []jujutesting.StubCall{
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
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SetRemoteApplicationStatus", []interface{}{"db2", "terminated", "offer has been removed"}},
		{"Close", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) assertRemoteRelationsWorkers(c *gc.C) worker.Worker {
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

	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	apiMac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	relTag := names.NewRelationTag("db2:db django:db")
	expected := []jujutesting.StubCall{
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
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SaveMacaroon", []interface{}{relTag, apiMac}},
		{"ImportRemoteEntity", []interface{}{names.NewApplicationTag("db2"), "token-offer-db2-uuid"}},
		{"WatchRelationSuspendedStatus", []interface{}{"token-db2:db django:db", macaroon.Slice{apiMac}}},
		{"WatchLocalRelationChanges", []interface{}{"db2:db django:db"}},
		{"WatchRelationChanges", []interface{}{"token-db2:db django:db", "token-offer-db2-uuid", macaroon.Slice{apiMac}}},
	}
	s.waitForWorkerStubCalls(c, expected)

	changeWatcher, ok := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
	c.Check(ok, jc.IsTrue)
	waitForStubCalls(c, &changeWatcher.Stub, []jujutesting.StubCall{
		{"Changes", nil},
	})
	changeWatcher, ok = s.remoteRelationsFacade.remoteRelationWatcher("token-db2:db django:db")
	c.Check(ok, jc.IsTrue)
	waitForStubCalls(c, &changeWatcher.Stub, []jujutesting.StubCall{
		{"Changes", nil},
	})
	relationStatusWatcher, ok := s.remoteRelationsFacade.relationsStatusWatcher("token-db2:db django:db")
	c.Check(ok, jc.IsTrue)
	waitForStubCalls(c, &relationStatusWatcher.Stub, []jujutesting.StubCall{
		{"Changes", nil},
	})
	return w
}

func (s *remoteRelationsSuite) TestRemoteRelationsWorkers(c *gc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	workertest.CleanKill(c, w)

	// Check that relation unit watchers are stopped with the worker.
	relWatcher, ok := s.relationsFacade.remoteRelationWatchers["db2:db django:db"]
	c.Check(ok, jc.IsTrue)
	c.Check(relWatcher.killed(), jc.IsTrue)

	relWatcher, ok = s.remoteRelationsFacade.remoteRelationWatchers["token-db2:db django:db"]
	c.Check(ok, jc.IsTrue)
	c.Check(relWatcher.killed(), jc.IsTrue)
}

func (s *remoteRelationsSuite) TestRemoteRelationsRevoked(c *gc.C) {
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

	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	relTag := names.NewRelationTag("db2:db django:db")
	expected := []jujutesting.StubCall{
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
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SetRemoteApplicationStatus", []interface{}{"db2", "error", "message"}},
		{"Close", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationsDying(c *gc.C) {
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
	c.Assert(unitsWatcher.killed(), jc.IsFalse)
	apiMac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	relTag := names.NewRelationTag("db2:db django:db")
	expected := []jujutesting.StubCall{
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
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SaveMacaroon", []interface{}{relTag, apiMac}},
		{"ImportRemoteEntity", []interface{}{names.NewApplicationTag("db2"), "token-offer-db2-uuid"}},
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				Life:             life.Dying,
				ApplicationToken: "token-django",
				RelationToken:    "token-db2:db django:db",
				Macaroons:        macaroon.Slice{apiMac},
				BakeryVersion:    bakery.LatestVersion,
			},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestLocalRelationsRemoved(c *gc.C) {
	// Checks that when a remote relation goes away, the relation units
	// worker is killed.
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.relationsFacade.removeRelation("db2:db django:db")
	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, ok := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
		if !ok {
			break
		}
	}
	c.Assert(unitsWatcher.killed(), jc.IsTrue)
	expected := []jujutesting.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestLocalRelationsChangedNotifies(c *gc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		RelationToken:    "token-db2:db django:db",
		ApplicationToken: "token-django",
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: map[string]interface{}{"foo": "bar"},
		}},
		DepartedUnits: []int{2},
	}

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	expected := []jujutesting.StubCall{
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationToken: "token-django",
				RelationToken:    "token-db2:db django:db",
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

func (s *remoteRelationsSuite) TestRemoteNotFoundTerminatesOnPublish(c *gc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	s.stub.SetErrors(params.Error{Code: params.CodeNotFound})

	unitsWatcher, _ := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		ApplicationToken: "token-django",
		RelationToken:    "token-db2:db django:db",
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: map[string]interface{}{"foo": "bar"},
		}},
		DepartedUnits: []int{2},
	}

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	expected := []jujutesting.StubCall{
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationToken: "token-django",
				RelationToken:    "token-db2:db django:db",
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   1,
					Settings: map[string]interface{}{"foo": "bar"},
				}},
				DepartedUnits: []int{2},
				Macaroons:     macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			},
		}},
		{"SetRemoteApplicationStatus", []interface{}{"db2", "terminated", "offer has been removed"}},
		{"Close", nil},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationsChangedConsumes(c *gc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.remoteRelationsFacade.remoteRelationWatcher("token-db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		ApplicationToken: "token-offer-db2-uuid",
		RelationToken:    "token-db2:db django:db",
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: map[string]interface{}{"foo": "bar"},
		}},
		DepartedUnits: []int{2},
	}

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	expected := []jujutesting.StubCall{
		{"ConsumeRemoteRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationToken: "token-offer-db2-uuid",
				RelationToken:    "token-db2:db django:db",
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

func (s *remoteRelationsSuite) TestRemoteRelationsDyingConsumes(c *gc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	statusWatcher, _ := s.remoteRelationsFacade.relationsStatusWatcher("token-db2:db django:db")
	statusWatcher.changes <- []watcher.RelationStatusChange{{
		Life: life.Dying,
	}}

	suspended := false
	expected := []jujutesting.StubCall{
		{"ConsumeRemoteRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				Life:             life.Dying,
				ApplicationToken: "token-offer-db2-uuid",
				RelationToken:    "token-db2:db django:db",
				Suspended:        &suspended,
			},
		}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationsChangedError(c *gc.C) {
	s.assertRemoteRelationsChangedError(c, false)
}

func (s *remoteRelationsSuite) TestRemoteDyingRelationsChangedError(c *gc.C) {
	s.assertRemoteRelationsChangedError(c, true)
}

func (s *remoteRelationsSuite) assertRemoteRelationsChangedError(c *gc.C, dying bool) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	s.stub.SetErrors(errors.New("failed"))
	unitsWatcher, _ := s.relationsFacade.remoteRelationWatcher("db2:db django:db")
	unitsWatcher.changes <- params.RemoteRelationChangeEvent{
		ApplicationToken: "token-django",
		RelationToken:    "token-db2:db django:db",
		DepartedUnits:    []int{1},
	}

	// The error causes relation change publication to fail.
	apiMac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	expected := []jujutesting.StubCall{
		{"PublishRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				ApplicationToken: "token-django",
				RelationToken:    "token-db2:db django:db",
				DepartedUnits:    []int{1},
				Macaroons:        macaroon.Slice{apiMac},
				BakeryVersion:    bakery.LatestVersion,
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
	c.Assert(s.stub.Calls(), gc.HasLen, 0)
	s.config.Clock.(*testclock.Clock).WaitAdvance(10*time.Second, coretesting.LongWait, 1)

	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	expected = []jujutesting.StubCall{
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"ControllerAPIInfoForModel", []interface{}{"remote-model-uuid"}},
		{"WatchOfferStatus", []interface{}{"offer-db2-uuid", macaroon.Slice{mac}}},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	relTag := names.NewRelationTag("db2:db django:db")
	expected = []jujutesting.StubCall{
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
			LocalEndpointName: "data",
			Macaroons:         macaroon.Slice{mac},
			BakeryVersion:     bakery.LatestVersion,
		}}}},
		{"SaveMacaroon", []interface{}{relTag, apiMac}},
		{"ImportRemoteEntity", []interface{}{names.NewApplicationTag("db2"), "token-offer-db2-uuid"}},
		{"WatchRelationSuspendedStatus", []interface{}{"token-db2:db django:db", macaroon.Slice{apiMac}}},
		{"WatchLocalRelationChanges", []interface{}{"db2:db django:db"}},
		{"WatchRelationChanges", []interface{}{"token-db2:db django:db", "token-offer-db2-uuid", macaroon.Slice{apiMac}}},
	}

	// If a relation is dying and there's been an error, when processing resumes
	// a cleanup is forced on the remote side.
	if dying {
		s.relationsFacade.updateRelationLife("db2:db django:db", life.Dying)
		forceCleanup := true
		expected = append(expected, jujutesting.StubCall{
			FuncName: "PublishRelationChange",
			Args: []interface{}{
				params.RemoteRelationChangeEvent{
					ApplicationToken: "token-django",
					RelationToken:    "token-db2:db django:db",
					Life:             life.Dying,
					Macaroons:        macaroon.Slice{apiMac},
					BakeryVersion:    bakery.LatestVersion,
					ForceCleanup:     &forceCleanup,
				},
			}},
		)
	}

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	// After the worker resumes, normal processing happens.
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRegisteredApplicationNotRegistered(c *gc.C) {
	s.relationsFacade.relations["db2:db django:db"] = newMockRelation(123)
	db2app := newMockRemoteApplication("db2", "db2url")
	db2app.registered = true
	s.relationsFacade.remoteApplications["db2"] = db2app
	applicationNames := []string{"db2"}
	s.relationsFacade.remoteApplicationsWatcher.changes <- applicationNames

	w, err := remoterelations.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	expected := []jujutesting.StubCall{
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

	expected = []jujutesting.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
	}
	s.waitForWorkerStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationSuspended(c *gc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	// First suspend the relation.
	s.relationsFacade.relations["db2:db django:db"].SetSuspended(true)
	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	expected := []jujutesting.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
	}
	s.waitForWorkerStubCalls(c, expected)
	s.stub.ResetCalls()

	// Now resume the relation.
	s.relationsFacade.relations["db2:db django:db"].SetSuspended(false)
	relWatcher.changes <- []string{"db2:db django:db"}

	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	apiMac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)

	relTag := names.NewRelationTag("db2:db django:db")
	// When resuming, it's similar to setting things up for a new relation
	// except that the call to create te life/status listener is missing.
	expected = []jujutesting.StubCall{
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
