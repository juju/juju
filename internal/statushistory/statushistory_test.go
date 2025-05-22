// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type statusHistorySuite struct {
	testhelpers.IsolationSuite

	recorder *MockRecorder
}

func TestStatusHistorySuite(t *testing.T) {
	tc.Run(t, &statusHistorySuite{})
}

func (s *statusHistorySuite) TestNamespace(c *tc.C) {
	ns := Namespace{Kind: "foo", ID: "123"}
	c.Assert(ns.String(), tc.Equals, "foo (123)")
	c.Assert(ns.WithID("456").String(), tc.Equals, "foo (456)")
}

func (s *statusHistorySuite) TestNamespaceNoID(c *tc.C) {
	ns := Namespace{Kind: "foo"}
	c.Assert(ns.String(), tc.Equals, "foo")
	c.Assert(ns.WithID("").String(), tc.Equals, "foo")
}

func (s *statusHistorySuite) TestRecordStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Kind: "foo", ID: "123"}
	now := time.Now()

	s.recorder.EXPECT().Record(gomock.Any(), Record{
		Kind:    "foo",
		ID:      "123",
		Status:  "active",
		Message: "foo",
		Time:    now.Format(time.RFC3339),
		Data: map[string]any{
			"bar": "baz",
		},
	}).Return(nil)

	statusHistory := NewStatusHistory(s.recorder, clock.WallClock)
	err := statusHistory.RecordStatus(c.Context(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]any{
			"bar": "baz",
		},
		Since: &now,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *statusHistorySuite) TestRecordStatusWithError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Kind: "foo", ID: "123"}
	now := time.Now()

	s.recorder.EXPECT().Record(gomock.Any(), Record{
		Kind:    "foo",
		ID:      "123",
		Status:  "active",
		Message: "foo",
		Time:    now.Format(time.RFC3339),
		Data: map[string]any{
			"bar": "baz",
		},
	}).Return(errors.Errorf("failed to record"))

	statusHistory := NewStatusHistory(s.recorder, clock.WallClock)
	err := statusHistory.RecordStatus(c.Context(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]any{
			"bar": "baz",
		},
		Since: &now,
	})
	c.Assert(err, tc.ErrorMatches, "failed to record")
}

func (s *statusHistorySuite) TestRecordStatusNoID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Kind: "foo"}
	now := time.Now()

	s.recorder.EXPECT().Record(gomock.Any(), Record{
		Kind:    "foo",
		Status:  "active",
		Message: "foo",
		Time:    now.Format(time.RFC3339),
		Data: map[string]any{
			"bar": "baz",
		},
	}).Return(nil)

	statusHistory := NewStatusHistory(s.recorder, clock.WallClock)
	err := statusHistory.RecordStatus(c.Context(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]any{
			"bar": "baz",
		},
		Since: &now,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *statusHistorySuite) TestRecordStatusNoData(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Kind: "foo"}.WithID("123")
	now := time.Now()

	s.recorder.EXPECT().Record(gomock.Any(), Record{
		Kind:    "foo",
		ID:      "123",
		Status:  "active",
		Message: "foo",
		Time:    now.Format(time.RFC3339),
	}).Return(nil)

	statusHistory := NewStatusHistory(s.recorder, clock.WallClock)
	err := statusHistory.RecordStatus(c.Context(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *statusHistorySuite) TestRecordStatusNoSince(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Kind: "foo"}.WithID("123")

	var record Record
	s.recorder.EXPECT().Record(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, r Record) error {
		record = r
		return nil
	})

	statusHistory := NewStatusHistory(s.recorder, clock.WallClock)
	err := statusHistory.RecordStatus(c.Context(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(record.Time, tc.Not(tc.Equals), "")
}

func (s *statusHistorySuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.recorder = NewMockRecorder(ctrl)

	return ctrl
}

type statusHistoryReaderSuite struct {
	testhelpers.IsolationSuite
}

func TestStatusHistoryReaderSuite(t *testing.T) {
	tc.Run(t, &statusHistoryReaderSuite{})
}
func (s *statusHistoryReaderSuite) TestWalk(c *tc.C) {
	var expected []HistoryRecord

	modelUUID := "model-uuid"

	rnd := rand.IntN(50) + 100

	path := s.createFile(c, func(c *tc.C, w io.Writer) {
		now := time.Now().Truncate(time.Minute).UTC()

		encoder := json.NewEncoder(w)
		for i := range rnd {
			record := Record{
				Kind:    "application",
				ID:      strconv.Itoa(i),
				Status:  status.Active.String(),
				Message: "foo",
				Time:    now.Format(time.RFC3339),
			}
			data := `{"bar": "baz"}`

			labels := logger.Labels{
				categoryKey:    statusHistoryCategory,
				kindKey:        record.Kind.String(),
				namespaceIDKey: record.ID,
				statusKey:      record.Status,
				messageKey:     record.Message,
				sinceKey:       record.Time,
				dataKey:        data,
			}

			err := encoder.Encode(logger.LogRecord{
				ModelUUID: modelUUID,
				Time:      time.Now(),
				Labels:    labels,
			})
			c.Assert(err, tc.ErrorIsNil)

			expected = append(expected, HistoryRecord{
				ModelUUID: model.UUID(modelUUID),
				Kind:      status.KindApplication,
				Tag:       record.ID,
				Status: status.DetailedStatus{
					Kind:   status.KindApplication,
					Status: status.Active,
					Info:   "foo",
					Since:  &now,
					Data: map[string]any{
						"bar": "baz",
					},
				},
			})
		}

		sort.Slice(expected, func(i, j int) bool {
			tag1, _ := strconv.Atoi(expected[i].Tag)
			tag2, _ := strconv.Atoi(expected[j].Tag)
			return tag1 >= tag2
		})
	})

	history, err := ModelStatusHistoryReaderFromFile(model.UUID(modelUUID), path)
	c.Assert(err, tc.ErrorIsNil)

	var records []HistoryRecord
	err = history.Walk(func(rec HistoryRecord) (bool, error) {
		records = append(records, rec)
		return false, nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(records, tc.DeepEquals, expected)
}

func (s *statusHistoryReaderSuite) TestWalkWhilstAdding(c *tc.C) {
	var expected []HistoryRecord

	modelUUID := "model-uuid"

	rnd := rand.IntN(50) + 100

	path := s.createFile(c, func(c *tc.C, w io.Writer) {
		now := time.Now().Truncate(time.Minute).UTC()

		encoder := json.NewEncoder(w)
		for i := range rnd {
			record := Record{
				Kind:    "application",
				ID:      strconv.Itoa(i),
				Status:  status.Active.String(),
				Message: "foo",
				Time:    now.Format(time.RFC3339),
			}
			data := `{"bar": "baz"}`

			labels := logger.Labels{
				categoryKey:    statusHistoryCategory,
				kindKey:        record.Kind.String(),
				namespaceIDKey: record.ID,
				statusKey:      record.Status,
				messageKey:     record.Message,
				sinceKey:       record.Time,
				dataKey:        data,
			}

			err := encoder.Encode(logger.LogRecord{
				ModelUUID: modelUUID,
				Time:      time.Now(),
				Labels:    labels,
			})
			c.Assert(err, tc.ErrorIsNil)

			expected = append(expected, HistoryRecord{
				ModelUUID: model.UUID(modelUUID),
				Kind:      status.KindApplication,
				Tag:       record.ID,
				Status: status.DetailedStatus{
					Kind:   status.KindApplication,
					Status: status.Active,
					Info:   "foo",
					Since:  &now,
					Data: map[string]any{
						"bar": "baz",
					},
				},
			})
		}

		sort.Slice(expected, func(i, j int) bool {
			tag1, _ := strconv.Atoi(expected[i].Tag)
			tag2, _ := strconv.Atoi(expected[j].Tag)
			return tag1 >= tag2
		})
	})

	history, err := ModelStatusHistoryReaderFromFile(model.UUID(modelUUID), path)
	c.Assert(err, tc.ErrorIsNil)

	var records []HistoryRecord
	err = history.Walk(func(rec HistoryRecord) (bool, error) {
		s.appendToFile(c, path, func(c *tc.C, w io.Writer) {
			now := time.Now().Truncate(time.Minute).UTC()

			encoder := json.NewEncoder(w)
			for i := range rnd {
				record := Record{
					Kind:    "application",
					ID:      strconv.Itoa(i),
					Status:  status.Active.String(),
					Message: "foo",
					Time:    now.Format(time.RFC3339),
				}
				data := `{"bar": "baz"}`

				labels := logger.Labels{
					categoryKey:    statusHistoryCategory,
					kindKey:        record.Kind.String(),
					namespaceIDKey: record.ID,
					statusKey:      record.Status,
					messageKey:     record.Message,
					sinceKey:       record.Time,
					dataKey:        data,
				}

				err := encoder.Encode(logger.LogRecord{
					ModelUUID: "foo-bar",
					Time:      time.Now(),
					Labels:    labels,
				})
				c.Assert(err, tc.ErrorIsNil)
			}
		})
		if rec.ModelUUID != model.UUID(modelUUID) {
			return false, nil
		}

		records = append(records, rec)
		return false, nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(records, tc.DeepEquals, expected)
}

func (s *statusHistoryReaderSuite) TestWalkWithDifferentLabel(c *tc.C) {
	var expected []HistoryRecord

	modelUUID := "model-uuid"

	path := s.createFile(c, func(c *tc.C, w io.Writer) {
		now := time.Now().Truncate(time.Minute).UTC()

		encoder := json.NewEncoder(w)
		for i := range 10 {
			record := Record{
				Kind:    "application",
				ID:      strconv.Itoa(i),
				Status:  status.Active.String(),
				Message: "foo",
				Time:    now.Format(time.RFC3339),
			}
			data := `{"bar": "baz"}`

			labels := logger.Labels{
				categoryKey:    "foo",
				kindKey:        record.Kind.String(),
				namespaceIDKey: record.ID,
				statusKey:      record.Status,
				messageKey:     record.Message,
				sinceKey:       record.Time,
				dataKey:        data,
			}

			err := encoder.Encode(logger.LogRecord{
				ModelUUID: modelUUID,
				Time:      time.Now(),
				Labels:    labels,
			})
			c.Assert(err, tc.ErrorIsNil)
		}
	})

	history, err := ModelStatusHistoryReaderFromFile(model.UUID(modelUUID), path)
	c.Assert(err, tc.ErrorIsNil)

	var records []HistoryRecord
	err = history.Walk(func(rec HistoryRecord) (bool, error) {
		records = append(records, rec)
		return false, nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(records, tc.DeepEquals, expected)
}

func (s *statusHistoryReaderSuite) TestWalkNoDocuments(c *tc.C) {
	var expected []HistoryRecord

	modelUUID := "model-uuid"

	path := s.createFile(c, func(c *tc.C, w io.Writer) {})

	history, err := ModelStatusHistoryReaderFromFile(model.UUID(modelUUID), path)
	c.Assert(err, tc.ErrorIsNil)

	var records []HistoryRecord
	err = history.Walk(func(rec HistoryRecord) (bool, error) {
		records = append(records, rec)
		return false, nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(records, tc.DeepEquals, expected)
}

func (s *statusHistoryReaderSuite) TestWalkCorruptLine(c *tc.C) {
	var expected []HistoryRecord

	modelUUID := "model-uuid"

	rnd := rand.IntN(2) + 1

	path := s.createFile(c, func(c *tc.C, w io.Writer) {
		now := time.Now().Truncate(time.Minute).UTC()

		encoder := json.NewEncoder(w)
		for i := range 10 {
			record := Record{
				Kind:    "application",
				ID:      strconv.Itoa(i),
				Status:  status.Active.String(),
				Message: "foo",
				Time:    now.Format(time.RFC3339),
			}
			data := `{"bar": "baz"}`

			labels := logger.Labels{
				categoryKey:    statusHistoryCategory,
				kindKey:        record.Kind.String(),
				namespaceIDKey: record.ID,
				statusKey:      record.Status,
				messageKey:     record.Message,
				sinceKey:       record.Time,
				dataKey:        data,
			}

			err := encoder.Encode(logger.LogRecord{
				ModelUUID: modelUUID,
				Time:      time.Now(),
				Labels:    labels,
			})
			c.Assert(err, tc.ErrorIsNil)

			// Corrupt a line, by adding a non-JSON prefix to the line, after
			// the current has been written.
			if i == rnd-1 {
				fmt.Fprintf(w, "!!!")
			} else if i == rnd {
				continue
			}

			expected = append(expected, HistoryRecord{
				ModelUUID: model.UUID(modelUUID),
				Kind:      status.KindApplication,
				Tag:       record.ID,
				Status: status.DetailedStatus{
					Kind:   status.KindApplication,
					Status: status.Active,
					Info:   "foo",
					Since:  &now,
					Data: map[string]any{
						"bar": "baz",
					},
				},
			})
		}

		sort.Slice(expected, func(i, j int) bool {
			return expected[i].Tag > expected[j].Tag
		})
	})

	history, err := ModelStatusHistoryReaderFromFile(model.UUID(modelUUID), path)
	c.Assert(err, tc.ErrorIsNil)

	var records []HistoryRecord
	err = history.Walk(func(rec HistoryRecord) (bool, error) {
		records = append(records, rec)
		return false, nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(records, tc.DeepEquals, expected)
}

func (s *statusHistoryReaderSuite) createFile(c *tc.C, fn func(*tc.C, io.Writer)) string {
	path := c.MkDir()

	filePath := filepath.Join(path, "logsink.log")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0644)
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		_ = file.Close()
	}()

	fn(c, file)

	err = file.Sync()
	c.Assert(err, tc.ErrorIsNil)

	return filePath
}

func (s *statusHistoryReaderSuite) appendToFile(c *tc.C, path string, fn func(*tc.C, io.Writer)) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		_ = file.Close()
	}()

	fn(c, file)

	err = file.Sync()
	c.Assert(err, tc.ErrorIsNil)
}
