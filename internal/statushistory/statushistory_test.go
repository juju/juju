// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/errors"
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

func (s *statusHistorySuite) TestNamespaceNoID(c *gc.C) {
	ns := Namespace{Name: "foo"}
	c.Assert(ns.String(), gc.Equals, "foo")
	c.Assert(ns.WithID("").String(), gc.Equals, "foo")
}

func (s *statusHistorySuite) TestRecordStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Name: "foo", ID: "123"}
	now := time.Now()

	s.recorder.EXPECT().Record(gomock.Any(), Record{
		Name:    "foo",
		ID:      "123",
		Status:  "active",
		Message: "foo",
		Time:    now.Format(time.RFC3339),
		Data: map[string]interface{}{
			"bar": "baz",
		},
	}).Return(nil)

	statusHistory := NewStatusHistory(s.recorder, clock.WallClock)
	err := statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]interface{}{
			"bar": "baz",
		},
		Since: &now,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusHistorySuite) TestRecordStatusWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Name: "foo", ID: "123"}
	now := time.Now()

	s.recorder.EXPECT().Record(gomock.Any(), Record{
		Name:    "foo",
		ID:      "123",
		Status:  "active",
		Message: "foo",
		Time:    now.Format(time.RFC3339),
		Data: map[string]interface{}{
			"bar": "baz",
		},
	}).Return(errors.Errorf("failed to record"))

	statusHistory := NewStatusHistory(s.recorder, clock.WallClock)
	err := statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]interface{}{
			"bar": "baz",
		},
		Since: &now,
	})
	c.Assert(err, gc.ErrorMatches, "failed to record")
}

func (s *statusHistorySuite) TestRecordStatusNoID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Name: "foo"}
	now := time.Now()

	s.recorder.EXPECT().Record(gomock.Any(), Record{
		Name:    "foo",
		Status:  "active",
		Message: "foo",
		Time:    now.Format(time.RFC3339),
		Data: map[string]interface{}{
			"bar": "baz",
		},
	}).Return(nil)

	statusHistory := NewStatusHistory(s.recorder, clock.WallClock)
	err := statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]interface{}{
			"bar": "baz",
		},
		Since: &now,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusHistorySuite) TestRecordStatusNoData(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Name: "foo"}.WithID("123")
	now := time.Now()

	s.recorder.EXPECT().Record(gomock.Any(), Record{
		Name:    "foo",
		ID:      "123",
		Status:  "active",
		Message: "foo",
		Time:    now.Format(time.RFC3339),
	}).Return(nil)

	statusHistory := NewStatusHistory(s.recorder, clock.WallClock)
	err := statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusHistorySuite) TestRecordStatusNoSince(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Name: "foo"}.WithID("123")

	var record Record
	s.recorder.EXPECT().Record(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, r Record) error {
		record = r
		return nil
	})

	statusHistory := NewStatusHistory(s.recorder, clock.WallClock)
	err := statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(record.Time, gc.Not(gc.Equals), "")
}

func (s *statusHistorySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.recorder = NewMockRecorder(ctrl)

	return ctrl
}
