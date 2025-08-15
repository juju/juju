// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"context"
	"errors"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

func TestCrossControllerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &CrossControllerSuite{})
}

type CrossControllerSuite struct {
	testhelpers.IsolationSuite

	watcherRegistry          *facademocks.MockWatcherRegistry
	watcher                  *mockNotifyWatcher
	localControllerInfo      func() ([]string, string, error)
	watchLocalControllerInfo watchLocalControllerInfoFunc
	api                      *CrossControllerAPI

	publicDnsAddress string
}

func (s *CrossControllerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	c.Cleanup(func() {
		s.watcherRegistry = nil
	})

	return ctrl
}

func (s *CrossControllerSuite) newAPI(c *tc.C) {
	s.localControllerInfo = func() ([]string, string, error) {
		return []string{"addr1", "addr2"}, "ca-cert", nil
	}
	s.watchLocalControllerInfo = func(ctx context.Context) (watcher.NotifyWatcher, error) {
		return s.watcher, nil
	}
	api, err := NewCrossControllerAPI(
		s.watcherRegistry,
		func(context.Context) ([]string, string, error) { return s.localControllerInfo() },
		func(context.Context) (string, error) { return s.publicDnsAddress, nil },
		func(ctx context.Context) (watcher.NotifyWatcher, error) {
			return s.watchLocalControllerInfo(c.Context())
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.api = api
	s.watcher = newMockNotifyWatcher()
	s.AddCleanup(func(*tc.C) { _ = s.watcher.Stop() })

	c.Cleanup(func() {
		s.api = nil
		s.localControllerInfo = nil
		s.watchLocalControllerInfo = nil
		s.watcher = nil
	})
}

func (s *CrossControllerSuite) TestControllerInfo(c *tc.C) {
	s.newAPI(c)
	results, err := s.api.ControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ControllerAPIInfoResults{
		Results: []params.ControllerAPIInfoResult{{
			Addresses: []string{"addr1", "addr2"},
			CACert:    "ca-cert",
		}},
	})
}

func (s *CrossControllerSuite) TestControllerInfoWithDNSAddress(c *tc.C) {
	s.newAPI(c)
	s.publicDnsAddress = "publicDNSaddr"
	results, err := s.api.ControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ControllerAPIInfoResults{
		Results: []params.ControllerAPIInfoResult{{
			Addresses: []string{"publicDNSaddr", "addr1", "addr2"},
			CACert:    "ca-cert",
		}},
	})
}

func (s *CrossControllerSuite) TestControllerInfoError(c *tc.C) {
	s.newAPI(c)
	s.localControllerInfo = func() ([]string, string, error) {
		return nil, "", errors.New("nope")
	}
	results, err := s.api.ControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ControllerAPIInfoResults{
		Results: []params.ControllerAPIInfoResult{{
			Error: &params.Error{Message: "nope"},
		}},
	})
}

func (s *CrossControllerSuite) TestWatchControllerInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.newAPI(c)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("42", nil)
	s.watcher.changes <- struct{}{} // initial value
	results, err := s.api.WatchControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "42",
		}},
	})
}

func (s *CrossControllerSuite) TestWatchControllerInfoError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.newAPI(c)
	s.watcher.tomb.Kill(errors.New("nope"))
	close(s.watcher.changes)

	results, err := s.api.WatchControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: &params.Error{Message: "nope"},
		}},
	})
}

type stubControllerInfoGetter struct{}

func (stubControllerInfoGetter) GetAllAPIAddressesForClients(ctx context.Context) ([]string, error) {
	return []string{"host-name:50000", "10.1.2.3:50000"}, nil

}

func (s *CrossControllerSuite) TestGetControllerInfo(c *tc.C) {
	addrs, cert, err := controllerInfo(c.Context(), stubControllerInfoGetter{}, controller.Config{
		"ca-cert": "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Public address is sorted first.
	c.Check(addrs, tc.DeepEquals, []string{"host-name:50000", "10.1.2.3:50000"})
	c.Check(cert, tc.Equals, "ca-cert")
}
