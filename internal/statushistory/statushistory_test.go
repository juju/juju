// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"context"
	"time"

	logger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type statusHistorySuite struct {
	testing.IsolationSuite

	recorder *MockRecorder
}

var _ = gc.Suite(&statusHistorySuite{})

func (s *statusHistorySuite) TestNamespace(c *gc.C) {
	ns := Namespace{Name: "foo", ID: "123"}
	c.Assert(ns.String(), gc.Equals, "foo (123)")
	c.Assert(ns.WithID("456").String(), gc.Equals, "foo (456)")
}

func (s *statusHistorySuite) TestRecordStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Name: "foo", ID: "123"}
	now := time.Now()

	labels := logger.Labels{
		namespaceNameKey: "foo",
		namespaceIDKey:   "123",
		statusKey:        "active",
		messageKey:       "foo",
		sinceKey:         now.Format(time.RFC3339),
		dataKey:          `{"bar":"baz"}`,
	}

	s.recorder.EXPECT().Logf(gomock.Any(), logger.INFO, labels, "status-history (state: %q, status-message: %q)", status.Active, "foo")

	statusHistory := NewStatusHistory(s.recorder)
	statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]interface{}{
			"bar": "baz",
		},
		Since: &now,
	})
}

func (s *statusHistorySuite) TestRecordStatusNoID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Name: "foo"}
	now := time.Now()

	labels := logger.Labels{
		namespaceNameKey: "foo",
		statusKey:        "active",
		messageKey:       "foo",
		sinceKey:         now.Format(time.RFC3339),
		dataKey:          `{"bar":"baz"}`,
	}

	s.recorder.EXPECT().Logf(gomock.Any(), logger.INFO, labels, "status-history (state: %q, status-message: %q)", status.Active, "foo")

	statusHistory := NewStatusHistory(s.recorder)
	statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]interface{}{
			"bar": "baz",
		},
		Since: &now,
	})
}

func (s *statusHistorySuite) TestRecordStatusNoData(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Name: "foo"}
	now := time.Now()

	labels := logger.Labels{
		namespaceNameKey: "foo",
		statusKey:        "active",
		messageKey:       "foo",
		sinceKey:         now.Format(time.RFC3339),
	}

	s.recorder.EXPECT().Logf(gomock.Any(), logger.INFO, labels, "status-history (state: %q, status-message: %q)", status.Active, "foo")

	statusHistory := NewStatusHistory(s.recorder)
	statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Since:   &now,
	})
}

func (s *statusHistorySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.recorder = NewMockRecorder(ctrl)

	return ctrl
}
