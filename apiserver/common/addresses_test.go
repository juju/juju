// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
)

type apiAddresserSuite struct {
	apiAddressAccessor *MockAPIAddressAccessor
	watcherRegistry    *facademocks.MockWatcherRegistry
}

func TestApiAddresserSuite(t *testing.T) {
	tc.Run(t, &apiAddresserSuite{})
}

func (s *apiAddresserSuite) TestAPIAddresses(c *tc.C) {
	defer s.setupMock(c).Finish()
	// Arrange
	res := []string{"10.2.3.43:1", "10.4.7.178:2"}
	s.apiAddressAccessor.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(res, nil)
	addresser := s.getAddresser()

	// Act
	result, err := addresser.APIAddresses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Result, tc.SameContents, []string{"10.2.3.43:1", "10.4.7.178:2"})
}

func (s *apiAddresserSuite) TestAPIHostPorts(c *tc.C) {
	defer s.setupMock(c).Finish()
	// Arrange
	mhp := []network.MachineHostPorts{
		{
			{
				MachineAddress: network.NewMachineAddress("10.2.3.54"),
				NetPort:        1,
			},
		}, {
			{
				MachineAddress: network.NewMachineAddress("192.168.5.7"),
				NetPort:        2,
			},
		},
	}
	args := []network.HostPorts{
		mhp[0].HostPorts(),
		mhp[1].HostPorts(),
	}
	s.apiAddressAccessor.EXPECT().GetAPIHostPortsForAgents(gomock.Any()).Return(args, nil)
	expected := [][]params.HostPort{
		{{
			Address: params.Address{Value: "10.2.3.54"},
			Port:    1,
		}}, {{
			Address: params.Address{Value: "192.168.5.7"},
			Port:    2,
		}}}
	addresser := s.getAddresser()

	// Act
	obtained, err := addresser.APIHostPorts(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	// DeepEquals yield inconsistent results as a map is used in
	// APIHostPorts. SameContents doesn't like the slice of
	// slices.
	found := 0
	for _, server := range obtained.Servers {
		if server[0].Port == expected[0][0].Port &&
			server[0].Address.Value == expected[0][0].Address.Value {
			found++
			continue
		}
		if server[0].Port == expected[1][0].Port &&
			server[0].Address.Value == expected[1][0].Address.Value {
			found++
		}
	}
	c.Assert(found, tc.Equals, 2)
}

func (s *apiAddresserSuite) TestWatchAPIHostPorts(c *tc.C) {
	defer s.setupMock(c).Finish()
	// Arrange
	done := make(chan struct{})
	defer close(done)
	ch := make(chan struct{})
	w := watchertest.NewMockNotifyWatcher(ch)
	s.apiAddressAccessor.EXPECT().WatchControllerAPIAddresses(gomock.Any()).DoAndReturn(
		func(_ context.Context) (watcher.Watcher[struct{}], error) {
			time.AfterFunc(coretesting.ShortWait, func() {
				// Send initial event.
				select {
				case ch <- struct{}{}:
				case <-done:
					c.Error("watcher did not fire")
				}
			})
			return w, nil
		})

	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("42", nil)
	addresser := s.getAddresser()

	// Act
	obtained, err := addresser.WatchAPIHostPorts(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained.NotifyWatcherId, tc.Equals, "42")
}

func (s *apiAddresserSuite) getAddresser() *APIAddresser {
	return NewAPIAddresser(s.apiAddressAccessor, s.watcherRegistry)
}

func (s *apiAddresserSuite) setupMock(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.apiAddressAccessor = NewMockAPIAddressAccessor(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	c.Cleanup(func() {
		s.apiAddressAccessor = nil
		s.watcherRegistry = nil
	})
	return ctrl
}
