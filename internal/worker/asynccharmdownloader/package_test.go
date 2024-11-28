// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	stdtesting "testing"
	time "time"

	"github.com/juju/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package asynccharmdownloader -destination service_mocks_test.go github.com/juju/juju/internal/worker/asynccharmdownloader ApplicationService,ModelConfigService
//go:generate go run go.uber.org/mock/mockgen -typed -package asynccharmdownloader -destination downloader_mocks_test.go github.com/juju/juju/internal/worker/asynccharmdownloader Downloader
//go:generate go run go.uber.org/mock/mockgen -typed -package asynccharmdownloader -destination clock_mocks_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package asynccharmdownloader -destination http_mocks_test.go github.com/juju/juju/core/http HTTPClientGetter,HTTPClient

func TestAll(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	testing.IsolationSuite

	applicationService *MockApplicationService
	modelConfigService *MockModelConfigService
	downloader         *MockDownloader
	clock              *MockClock
	httpClientGetter   *MockHTTPClientGetter
	httpClient         *MockHTTPClient
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
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
