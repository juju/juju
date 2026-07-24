// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	"context"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	watcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run github.com/canonical/gomock/mockgen -package objectstores3caller -destination package_mock_test.go github.com/juju/juju/core/objectstore Client,Session
//go:generate go run github.com/canonical/gomock/mockgen -package objectstores3caller -destination services_mocks_test.go github.com/juju/juju/internal/worker/objectstores3caller ObjectStoreService
//go:generate go run github.com/canonical/gomock/mockgen -package objectstores3caller -destination http_mocks_test.go github.com/juju/juju/internal/s3client HTTPClient
//go:generate go run github.com/canonical/gomock/mockgen -package objectstores3caller -destination httpclient_mock_test.go github.com/juju/juju/core/http HTTPClientGetter

type baseSuite struct {
	states chan string

	session            *MockSession
	objectStoreService *MockObjectStoreService

	httpClientGetter *MockHTTPClientGetter
	httpClient       *MockHTTPClient

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := gomock.NewController(c)

	s.session = NewMockSession(ctrl)
	s.objectStoreService = NewMockObjectStoreService(ctrl)

	s.httpClientGetter = NewMockHTTPClientGetter(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	c.Cleanup(func() {
		s.session = nil
		s.objectStoreService = nil
		s.httpClientGetter = nil
		s.httpClient = nil
		s.logger = nil
	})

	return ctrl
}

func (s *baseSuite) expectHTTPClient(c *tc.C) {
	s.httpClientGetter.EXPECT().GetHTTPClient(gomock.Any(), corehttp.S3Purpose).Return(s.httpClient, nil)
}

func (s *baseSuite) expectGetActiveBackendS3(c *tc.C) {
	endpoint := "https://s3.example.com"
	region := "us-east-1"
	accessKey := "access-key"
	secretKey := "secret-key"
	s.objectStoreService.EXPECT().GetActiveObjectStoreBackend(gomock.Any()).Return(objectstoreservice.BackendInfo{
		Type:      "s3",
		Region:    &region,
		Endpoint:  &endpoint,
		AccessKey: &accessKey,
		SecretKey: &secretKey,
	}, nil)
}

func (s *baseSuite) expectGetActiveBackendFile(c *tc.C) {
	s.objectStoreService.EXPECT().GetActiveObjectStoreBackend(gomock.Any()).Return(objectstoreservice.BackendInfo{
		Type: "file",
	}, nil)
}

func (s *baseSuite) expectWatchObjectStoreBackend(c *tc.C) {
	s.objectStoreService.EXPECT().WatchObjectStoreBackend(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
		ch := make(chan []string)
		return watchertest.NewMockStringsWatcher(ch), nil
	})
}

func (s *baseSuite) watchObjectStoreBackendWithChanges(changes <-chan []string) func(context.Context) (watcher.Watcher[[]string], error) {
	return func(context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(changes), nil
	}
}

func (s *baseSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for startup")
	}
}
