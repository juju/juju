// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	"net/http"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/agent/engine"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/objectstores3caller"
)

type EngineStartupSuite struct {
	testing.BaseSuite
}

func (s *EngineStartupSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *EngineStartupSuite) newEngine(c *tc.C) *dependency.Engine {
	eng, err := dependency.NewEngine(dependency.EngineConfig{
		IsFatal:          func(err error) bool { return false },
		WorstError:       func(err1, err2 error) error { return err1 },
		Metrics:          dependency.DefaultMetrics(),
		Logger:           &stubEngineLogger{},
		ErrorDelay:       10 * time.Millisecond,
		BounceDelay:      10 * time.Millisecond,
		BackoffFactor:    1,
		BackoffResetTime: 1 * time.Minute,
		MaxDelay:         100 * time.Millisecond,
		Clock:            clock.WallClock,
	})
	c.Assert(err, tc.ErrorIsNil)
	return eng
}

func (s *EngineStartupSuite) TestS3CallerManifoldStartsBackendNotFound(c *tc.C) {
	httpClient := &stubHTTPClient{}
	httpClientGetter := &stubHTTPClientGetter{client: httpClient}
	objectStoreSvc := &stubObjStoreService{
		getActiveBackend: func(ctx context.Context) (objectstoreservice.BackendInfo, error) {
			return objectstoreservice.BackendInfo{}, objectstoreerrors.ErrBackendNotFound
		},
		watchBackend: func(ctx context.Context) (watcher.StringsWatcher, error) {
			ch := make(chan []string)
			return watchertest.NewMockStringsWatcher(ch), nil
		},
	}

	newClient := func(_ string, _ s3client.HTTPClient, _ s3client.Credentials, _ logger.Logger) (objectstore.Session, error) {
		c.Fatalf("NewClient should not be called when backend is not found")
		return nil, nil
	}

	httpWorker, err := engine.NewValueWorker(httpClientGetter)
	c.Assert(err, tc.ErrorIsNil)

	svcWorker, err := engine.NewValueWorker(objectStoreSvc)
	c.Assert(err, tc.ErrorIsNil)

	manifolds := dependency.Manifolds{
		"http-client": {
			Start:  func(ctx context.Context, _ dependency.Getter) (worker.Worker, error) { return httpWorker, nil },
			Output: engine.ValueWorkerOutput,
		},
		"object-store-service": {
			Start:  func(ctx context.Context, _ dependency.Getter) (worker.Worker, error) { return svcWorker, nil },
			Output: engine.ValueWorkerOutput,
		},
		"object-store-s3-caller": objectstores3caller.Manifold(objectstores3caller.ManifoldConfig{
			HTTPClientName:          "http-client",
			ObjectStoreServicesName: "object-store-service",
			NewClient:               newClient,
			Logger:                  loggertesting.WrapCheckLog(c),
			GetObjectStoreService:   getObjectStoreServiceDirect,
			NewWorker:               objectstores3caller.NewWorker,
		}),
	}

	eng := s.newEngine(c)
	defer workertest.DirtyKill(c, eng)

	err = dependency.Install(eng, manifolds)
	c.Assert(err, tc.ErrorIsNil)

	workertest.CheckAlive(c, eng)

	err = workertest.CheckKill(c, eng)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *EngineStartupSuite) TestS3CallerManifoldFailsNonFatalErrorDropped(c *tc.C) {
	// For non-ErrBackendNotFound errors, the manifold's Start function
	// returns the error, which the engine bounces. The engine does NOT
	// die because IsFatal returns false. This is correct behavior;
	// worker-level tests cover the precise error discrimination.
	// This test confirms the manifold integrates without panic.

	httpClient := &stubHTTPClient{}
	httpClientGetter := &stubHTTPClientGetter{client: httpClient}
	objectStoreSvc := &stubObjStoreService{
		getActiveBackend: func(ctx context.Context) (objectstoreservice.BackendInfo, error) {
			return objectstoreservice.BackendInfo{}, errors.Errorf("fatal service error")
		},
	}

	newClient := func(_ string, _ s3client.HTTPClient, _ s3client.Credentials, _ logger.Logger) (objectstore.Session, error) {
		c.Fatalf("NewClient should not be called on error")
		return nil, nil
	}

	httpWorker, err := engine.NewValueWorker(httpClientGetter)
	c.Assert(err, tc.ErrorIsNil)

	svcWorker, err := engine.NewValueWorker(objectStoreSvc)
	c.Assert(err, tc.ErrorIsNil)

	manifolds := dependency.Manifolds{
		"http-client": {
			Start:  func(ctx context.Context, _ dependency.Getter) (worker.Worker, error) { return httpWorker, nil },
			Output: engine.ValueWorkerOutput,
		},
		"object-store-service": {
			Start:  func(ctx context.Context, _ dependency.Getter) (worker.Worker, error) { return svcWorker, nil },
			Output: engine.ValueWorkerOutput,
		},
		"object-store-s3-caller": objectstores3caller.Manifold(objectstores3caller.ManifoldConfig{
			HTTPClientName:          "http-client",
			ObjectStoreServicesName: "object-store-service",
			NewClient:               newClient,
			Logger:                  loggertesting.WrapCheckLog(c),
			GetObjectStoreService:   getObjectStoreServiceDirect,
			NewWorker:               objectstores3caller.NewWorker,
		}),
	}

	eng := s.newEngine(c)
	defer workertest.DirtyKill(c, eng)

	err = dependency.Install(eng, manifolds)
	c.Assert(err, tc.ErrorIsNil)

	// Engine stays alive; the failing manifold bounces but doesn't
	// crash the engine.
	workertest.CheckAlive(c, eng)

	err = workertest.CheckKill(c, eng)
	c.Assert(err, tc.ErrorIsNil)
}

func getObjectStoreServiceDirect(getter dependency.Getter, name string) (objectstores3caller.ObjectStoreService, error) {
	var svc objectstores3caller.ObjectStoreService
	if err := getter.Get(name, &svc); err != nil {
		return nil, errors.Trace(err)
	}
	return svc, nil
}

type stubHTTPClientGetter struct {
	client corehttp.HTTPClient
}

func (s *stubHTTPClientGetter) GetHTTPClient(ctx context.Context, purpose corehttp.Purpose) (corehttp.HTTPClient, error) {
	return s.client, nil
}

type stubObjStoreService struct {
	getActiveBackend func(context.Context) (objectstoreservice.BackendInfo, error)
	watchBackend     func(context.Context) (watcher.StringsWatcher, error)
}

func (s *stubObjStoreService) GetActiveObjectStoreBackend(ctx context.Context) (objectstoreservice.BackendInfo, error) {
	if s.getActiveBackend != nil {
		return s.getActiveBackend(ctx)
	}
	return objectstoreservice.BackendInfo{}, nil
}

func (s *stubObjStoreService) WatchObjectStoreBackend(ctx context.Context) (watcher.StringsWatcher, error) {
	if s.watchBackend != nil {
		return s.watchBackend(ctx)
	}
	ch := make(chan []string)
	return watchertest.NewMockStringsWatcher(ch), nil
}

type stubHTTPClient struct{}

func (s *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

type stubEngineLogger struct{}

func (l *stubEngineLogger) Tracef(format string, args ...any) {}
func (l *stubEngineLogger) Debugf(format string, args ...any) {}
func (l *stubEngineLogger) Infof(format string, args ...any)  {}
func (l *stubEngineLogger) Errorf(format string, args ...any) {}

func TestEngineStartupSuite(t *stdtesting.T) {
	tc.Run(t, &EngineStartupSuite{})
}
