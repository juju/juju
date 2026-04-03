// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package remoterelationconsumer -destination service_mock_test.go -source worker.go
//go:generate go run go.uber.org/mock/mockgen -typed -package remoterelationconsumer -destination worker_mock_test.go github.com/juju/juju/internal/worker/remoterelationconsumer RemoteRelationClientGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package remoterelationconsumer -destination remote_relation_caller_mock_test.go github.com/juju/juju/internal/worker/apiremoterelationcaller APIRemoteCallerGetter

type baseSuite struct {
	testhelpers.IsolationSuite

	crossModelService          *MockCrossModelService
	remoteModelRelationClient  *MockRemoteModelRelationsClient
	remoteRelationClientGetter *MockRemoteRelationClientGetter
	apiRemoteCallerGetter      *MockAPIRemoteCallerGetter

	modelUUID model.UUID

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.crossModelService = NewMockCrossModelService(ctrl)
	s.remoteModelRelationClient = NewMockRemoteModelRelationsClient(ctrl)
	s.remoteRelationClientGetter = NewMockRemoteRelationClientGetter(ctrl)
	s.apiRemoteCallerGetter = NewMockAPIRemoteCallerGetter(ctrl)

	s.modelUUID = tc.Must(c, model.NewUUID)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

type errWorker struct {
	reportableWorker
	tomb tomb.Tomb
}

func newErrWorker(err error) *errWorker {
	w := &errWorker{}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return err
	})
	return w
}

func (w *errWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *errWorker) Wait() error {
	return w.tomb.Wait()
}

// macaroonErrWorker extends errWorker with Macaroon() and RelationUUID()
// methods, so it can be used as an offerer unit relation worker in tests.
type macaroonErrWorker struct {
	errWorker
	macaroon     *macaroon.Macaroon
	relationUUID corerelation.UUID
}

func newMacaroonErrWorker(mac *macaroon.Macaroon, relationUUID corerelation.UUID) *macaroonErrWorker {
	w := &macaroonErrWorker{
		macaroon:     mac,
		relationUUID: relationUUID,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
	return w
}

func (w *macaroonErrWorker) Macaroon() *macaroon.Macaroon {
	return w.macaroon
}

func (w *macaroonErrWorker) RelationUUID() corerelation.UUID {
	return w.relationUUID
}

type reportableWorker struct {
	worker.Worker
}

func (w reportableWorker) Report(_ context.Context) map[string]any {
	return make(map[string]any)
}

func waitForEmptyRunner(c *tc.C, runner *worker.Runner) {
	for {
		select {
		case <-time.After(time.Millisecond * 50):
			if len(runner.WorkerNames()) == 0 {
				return
			}

		case <-c.Context().Done():
			c.Fatalf("timed out waiting for application to be stopped")
		}
	}
}

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}
