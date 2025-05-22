// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/testhelpers"
)

type statusHistorySuite struct {
	testhelpers.IsolationSuite

	historyReader *MockStatusHistoryReader
	now           time.Time
}

func TestStatusHistorySuite(t *testing.T) {
	tc.Run(t, &statusHistorySuite{})
}

func (s *statusHistorySuite) TestGetStatusHistoryNoData(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectResults([]statushistory.HistoryRecord{})

	service := s.newService()
	results, err := service.GetStatusHistory(c.Context(), StatusHistoryRequest{
		Kind: status.KindUnit,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.HasLen, 0)
}

func (s *statusHistorySuite) TestGetStatusHistoryContextCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectResults([]statushistory.HistoryRecord{{}})

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	service := s.newService()
	_, err := service.GetStatusHistory(ctx, StatusHistoryRequest{
		Kind: status.KindUnit,
	})
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *statusHistorySuite) TestGetStatusHistoryError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.historyReader.EXPECT().Walk(gomock.Any()).Return(fmt.Errorf("foo"))

	service := s.newService()
	_, err := service.GetStatusHistory(c.Context(), StatusHistoryRequest{
		Kind: status.KindUnit,
	})
	c.Assert(err, tc.ErrorMatches, ".*foo")
}

func (s *statusHistorySuite) TestGetStatusHistoryErrorWalk(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.historyReader.EXPECT().Walk(gomock.Any()).DoAndReturn(
		func(fn func(statushistory.HistoryRecord) (bool, error)) error {
			_, err := fn(statushistory.HistoryRecord{})
			return err
		},
	).Return(fmt.Errorf("foo"))
	service := s.newService()
	_, err := service.GetStatusHistory(c.Context(), StatusHistoryRequest{
		Kind: status.KindUnit,
	})
	c.Assert(err, tc.ErrorMatches, ".*foo")
}

func (s *statusHistorySuite) TestGetStatusHistoryMatchesData(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectResults([]statushistory.HistoryRecord{{
		Kind: status.KindUnit,
		Status: status.DetailedStatus{
			Kind:   status.KindUnit,
			Status: status.Active,
			Info:   "foo",
			Data:   map[string]any{"bar": "baz"},
			Since:  ptr(s.now),
		},
	}})

	service := s.newService()
	results, err := service.GetStatusHistory(c.Context(), StatusHistoryRequest{
		Kind: status.KindUnit,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, []status.DetailedStatus{{
		Kind:   status.KindUnit,
		Status: status.Active,
		Info:   "foo",
		Data:   map[string]any{"bar": "baz"},
		Since:  ptr(s.now),
	}})
}

func (s *statusHistorySuite) TestGetStatusHistoryMatchesMultipleData(c *tc.C) {
	defer s.setupMocks(c).Finish()

	total := rand.IntN(100) + 10

	var records []statushistory.HistoryRecord
	var expected []status.DetailedStatus
	for i := range total {
		records = append(records, statushistory.HistoryRecord{
			Kind: status.KindUnit,
			Status: status.DetailedStatus{
				Kind:   status.KindUnit,
				Status: status.Active,
				Info:   fmt.Sprintf("foo-%d", i),
				Data:   map[string]any{"bar": fmt.Sprintf("baz-%d", i)},
				Since:  ptr(s.now.Add(time.Duration(total-i) * time.Minute)),
			},
		})

		expected = append(expected, status.DetailedStatus{
			Kind:   status.KindUnit,
			Status: status.Active,
			Info:   fmt.Sprintf("foo-%d", i),
			Data:   map[string]any{"bar": fmt.Sprintf("baz-%d", i)},
			Since:  ptr(s.now.Add(time.Duration(total-i) * time.Minute)),
		})
	}

	s.expectResults(records)

	service := s.newService()
	results, err := service.GetStatusHistory(c.Context(), StatusHistoryRequest{
		Kind: status.KindUnit,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, expected)
}

func (s *statusHistorySuite) TestGetStatusHistoryMatchesMultipleDataSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	total := rand.IntN(100) + 10

	var records []statushistory.HistoryRecord
	var expected []status.DetailedStatus
	for i := range total {
		records = append(records, statushistory.HistoryRecord{
			Kind: status.KindUnit,
			Status: status.DetailedStatus{
				Kind:   status.KindUnit,
				Status: status.Active,
				Info:   fmt.Sprintf("foo-%d", i),
				Data:   map[string]any{"bar": fmt.Sprintf("baz-%d", i)},
				Since:  ptr(s.now.Add(time.Duration(total-i) * time.Minute)),
			},
		})

		expected = append(expected, status.DetailedStatus{
			Kind:   status.KindUnit,
			Status: status.Active,
			Info:   fmt.Sprintf("foo-%d", i),
			Data:   map[string]any{"bar": fmt.Sprintf("baz-%d", i)},
			Since:  ptr(s.now.Add(time.Duration(total-i) * time.Minute)),
		})
	}

	s.expectResults(records)

	service := s.newService()
	results, err := service.GetStatusHistory(c.Context(), StatusHistoryRequest{
		Kind: status.KindUnit,
		Filter: StatusHistoryFilter{
			Size: total - 5,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, expected[:total-5])
}

func (s *statusHistorySuite) TestGetStatusHistoryMatchesKindData(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectResults([]statushistory.HistoryRecord{{
		Kind: status.KindUnit,
		Status: status.DetailedStatus{
			Kind:   status.KindUnit,
			Status: status.Active,
			Info:   "foo",
			Data:   map[string]any{"bar": "baz"},
			Since:  ptr(s.now),
		},
	}, {
		Kind: status.KindApplication,
		Status: status.DetailedStatus{
			Kind:   status.KindApplication,
			Status: status.Active,
			Info:   "foo",
			Data:   map[string]any{"bar": "baz"},
			Since:  ptr(s.now),
		},
	}})

	service := s.newService()
	results, err := service.GetStatusHistory(c.Context(), StatusHistoryRequest{
		Kind: status.KindUnit,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, []status.DetailedStatus{{
		Kind:   status.KindUnit,
		Status: status.Active,
		Info:   "foo",
		Data:   map[string]any{"bar": "baz"},
		Since:  ptr(s.now),
	}})
}

func (s *statusHistorySuite) TestMatches(c *tc.C) {
	tests := []struct {
		record   statushistory.HistoryRecord
		request  StatusHistoryRequest
		expected bool
		err      error
	}{
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindUnit,
			},
			request: StatusHistoryRequest{
				Kind: status.KindUnit,
			},
			expected: true,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindUnitAgent,
			},
			request: StatusHistoryRequest{
				Kind: status.KindUnit,
			},
			expected: true,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindWorkload,
			},
			request: StatusHistoryRequest{
				Kind: status.KindUnit,
			},
			expected: true,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindUnitAgent,
			},
			request: StatusHistoryRequest{
				Kind: status.KindUnitAgent,
			},
			expected: true,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindWorkload,
			},
			request: StatusHistoryRequest{
				Kind: status.KindUnitAgent,
			},
			expected: false,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindWorkload,
			},
			request: StatusHistoryRequest{
				Kind: status.KindWorkload,
			},
			expected: true,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindUnitAgent,
			},
			request: StatusHistoryRequest{
				Kind: status.KindWorkload,
			},
			expected: false,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindApplication,
			},
			request: StatusHistoryRequest{
				Kind: status.KindUnit,
			},
			expected: false,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindApplication,
			},
			request: StatusHistoryRequest{
				Kind: status.KindApplication,
			},
			expected: true,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindSAAS,
			},
			request: StatusHistoryRequest{
				Kind: status.KindApplication,
			},
			expected: false,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindApplication,
				Tag:  "foo",
			},
			request: StatusHistoryRequest{
				Kind: status.KindApplication,
				Tag:  "foo",
			},
			expected: true,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindApplication,
				Tag:  "foo",
			},
			request: StatusHistoryRequest{
				Kind: status.KindApplication,
				Tag:  "bar",
			},
			expected: false,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %v(%v) - %v(%v)", i, test.record.Kind, test.record.Tag, test.request.Kind, test.request.Tag)

		result, err := matches(test.record, test.request, s.now)
		if test.err != nil {
			c.Assert(err, tc.ErrorMatches, test.err.Error())
		} else {
			c.Assert(err, tc.ErrorIsNil)
		}

		c.Check(result, tc.Equals, test.expected)
	}
}

func (s *statusHistorySuite) TestMatchesDate(c *tc.C) {
	tests := []struct {
		record   statushistory.HistoryRecord
		request  StatusHistoryRequest
		expected bool
	}{
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindUnit,
				Status: status.DetailedStatus{
					Kind:  status.KindUnit,
					Since: ptr(s.now),
				},
			},
			request: StatusHistoryRequest{
				Kind: status.KindUnit,
				Filter: StatusHistoryFilter{
					Date: ptr(s.now),
				},
			},
			expected: false,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindUnit,
				Status: status.DetailedStatus{
					Kind:  status.KindUnit,
					Since: ptr(s.now),
				},
			},
			request: StatusHistoryRequest{
				Kind: status.KindUnit,
				Filter: StatusHistoryFilter{
					Date: ptr(s.now.Add(-time.Second)),
				},
			},
			expected: true,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %v(%v) - %v(%v)", i, test.record.Kind, test.record.Tag, test.request.Kind, test.request.Tag)

		result, err := matches(test.record, test.request, s.now)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.Equals, test.expected)
	}
}

func (s *statusHistorySuite) TestMatchesDelta(c *tc.C) {
	tests := []struct {
		record   statushistory.HistoryRecord
		request  StatusHistoryRequest
		expected bool
	}{
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindUnit,
				Status: status.DetailedStatus{
					Kind:  status.KindUnit,
					Since: ptr(s.now),
				},
			},
			request: StatusHistoryRequest{
				Kind: status.KindUnit,
				Filter: StatusHistoryFilter{
					Delta: ptr(time.Second),
				},
			},
			expected: true,
		},
		{
			record: statushistory.HistoryRecord{
				Kind: status.KindUnit,
				Status: status.DetailedStatus{
					Kind:  status.KindUnit,
					Since: ptr(s.now),
				},
			},
			request: StatusHistoryRequest{
				Kind: status.KindUnit,
				Filter: StatusHistoryFilter{
					Delta: ptr(-time.Second),
				},
			},
			expected: false,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %v(%v) - %v(%v)", i, test.record.Kind, test.record.Tag, test.request.Kind, test.request.Tag)

		result, err := matches(test.record, test.request, s.now)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.Equals, test.expected)
	}
}

func (s *statusHistorySuite) expectResults(records []statushistory.HistoryRecord) {
	s.historyReader.EXPECT().Walk(gomock.Any()).DoAndReturn(
		func(fn func(statushistory.HistoryRecord) (bool, error)) error {
			for _, record := range records {
				if stop, err := fn(record); err != nil {
					return err
				} else if stop {
					return nil
				}
			}
			return nil
		},
	)
}

func (s *statusHistorySuite) newService() *Service {
	return &Service{
		statusHistoryReaderFn: func() (StatusHistoryReader, error) {
			return s.historyReader, nil
		},
		clock: clock.WallClock,
	}
}

func (s *statusHistorySuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.historyReader = NewMockStatusHistoryReader(ctrl)
	s.historyReader.EXPECT().Close().Return(nil)

	s.now = time.Now()

	return ctrl
}
