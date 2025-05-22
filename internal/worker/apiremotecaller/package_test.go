// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	time "time"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package apiremotecaller -destination package_mocks_test.go github.com/juju/juju/internal/worker/apiremotecaller RemoteServer
//go:generate go run go.uber.org/mock/mockgen -typed -package apiremotecaller -destination clock_mocks_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package apiremotecaller -destination connection_mocks_test.go github.com/juju/juju/api Connection

type baseSuite struct {
	testhelpers.IsolationSuite

	clock      *MockClock
	remote     *MockRemoteServer
	connection *MockConnection

	states chan string
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.remote = NewMockRemoteServer(ctrl)
	s.connection = NewMockConnection(ctrl)

	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().DoAndReturn(func() time.Time {
		return time.Now()
	}).AnyTimes()
}

func (s *baseSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-time.After(testhelpers.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *baseSuite) ensureChanged(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateChanged)
	case <-time.After(testhelpers.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}
