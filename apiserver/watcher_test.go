// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	stdtesting "testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testhelpers"
)

type offerStatusWatcherSuite struct {
	testhelpers.IsolationSuite

	statusService *MockStatusService
	watcher       *MockOfferWatcher

	api *srvOfferStatusWatcher
}

func TestOfferStatusWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &offerStatusWatcherSuite{})
}

func (s *offerStatusWatcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.statusService = NewMockStatusService(ctrl)
	s.watcher = NewMockOfferWatcher(ctrl)

	s.api = &srvOfferStatusWatcher{
		statusService: s.statusService,
		watcher:       s.watcher,
	}

	return ctrl
}

func (s *offerStatusWatcherSuite) TestNext(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	offerUUID := tc.Must(c, offer.NewUUID)

	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	s.watcher.EXPECT().Changes().Return(changes)
	s.watcher.EXPECT().OfferUUID().Return(offerUUID)
	s.statusService.EXPECT().GetOfferStatus(gomock.Any(), offerUUID).Return(status.StatusInfo{
		Status:  status.Active,
		Message: "message",
	}, nil)

	res, err := s.api.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(res.Error, tc.IsNil)
	c.Assert(res.Changes, tc.HasLen, 1)
	c.Check(res.Changes[0].Status.Status, tc.Equals, status.Active)
	c.Check(res.Changes[0].Status.Info, tc.Equals, "message")
}
