// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	time "time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package asynccharmdownloader -destination package_mocks_test.go github.com/juju/juju/internal/worker/asynccharmdownloader ApplicationService,Downloader
//go:generate go run go.uber.org/mock/mockgen -typed -package asynccharmdownloader -destination clock_mocks_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package asynccharmdownloader -destination http_mocks_test.go github.com/juju/juju/core/http HTTPClientGetter,HTTPClient

type baseSuite struct {
	testhelpers.IsolationSuite

	applicationService *MockApplicationService
	downloader         *MockDownloader
	clock              *MockClock
	httpClientGetter   *MockHTTPClientGetter
	httpClient         *MockHTTPClient
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.downloader = NewMockDownloader(ctrl)
	s.clock = NewMockClock(ctrl)

	s.httpClientGetter = NewMockHTTPClientGetter(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)

	s.clock.EXPECT().Now().DoAndReturn(func() time.Time {
		return time.Now()
	}).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time)
		close(ch)
		return ch
	}).AnyTimes()

	return ctrl
}
