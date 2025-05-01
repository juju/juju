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
	"time"

	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/errors"
)

type statusHistorySuite struct {
	testing.IsolationSuite

	recorder *MockRecorder
}

var _ = gc.Suite(&statusHistorySuite{})

func (s *statusHistorySuite) TestNamespace(c *gc.C) {
	ns := Namespace{Kind: "foo", ID: "123"}
	c.Assert(ns.String(), gc.Equals, "foo (123)")
	c.Assert(ns.WithID("456").String(), gc.Equals, "foo (456)")
}

func (s *statusHistorySuite) TestNamespaceNoID(c *gc.C) {
	ns := Namespace{Kind: "foo"}
	c.Assert(ns.String(), gc.Equals, "foo")
	c.Assert(ns.WithID("").String(), gc.Equals, "foo")
}

func (s *statusHistorySuite) TestRecordStatus(c *gc.C) {
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
	err := statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]any{
			"bar": "baz",
		},
		Since: &now,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusHistorySuite) TestRecordStatusWithError(c *gc.C) {
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
	err := statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]any{
			"bar": "baz",
		},
		Since: &now,
	})
	c.Assert(err, gc.ErrorMatches, "failed to record")
}

func (s *statusHistorySuite) TestRecordStatusNoID(c *gc.C) {
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
	err := statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Data: map[string]any{
			"bar": "baz",
		},
		Since: &now,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusHistorySuite) TestRecordStatusNoData(c *gc.C) {
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
	err := statusHistory.RecordStatus(context.Background(), ns, status.StatusInfo{
		Status:  status.Active,
		Message: "foo",
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusHistorySuite) TestRecordStatusNoSince(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ns := Namespace{Kind: "foo"}.WithID("123")

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

type statusHistoryReaderSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&statusHistoryReaderSuite{})

func (s *statusHistoryReaderSuite) TestWalk(c *gc.C) {
	var expected []HistoryRecord

	modelUUID := "model-uuid"

	rnd := rand.IntN(50) + 100

	path := s.createFile(c, func(c *gc.C, w io.Writer) {
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
			c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

	var records []HistoryRecord
	err = history.Walk(func(rec HistoryRecord) (bool, error) {
		records = append(records, rec)
		return false, nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(records, gc.DeepEquals, expected)
}

func (s *statusHistoryReaderSuite) TestWalkWhilstAdding(c *gc.C) {
	var expected []HistoryRecord

	modelUUID := "model-uuid"

	rnd := rand.IntN(50) + 100

	path := s.createFile(c, func(c *gc.C, w io.Writer) {
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
			c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

	var records []HistoryRecord
	err = history.Walk(func(rec HistoryRecord) (bool, error) {
		s.appendToFile(c, path, func(c *gc.C, w io.Writer) {
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
				c.Assert(err, jc.ErrorIsNil)
			}
		})
		if rec.ModelUUID != model.UUID(modelUUID) {
			return false, nil
		}

		records = append(records, rec)
		return false, nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(records, gc.DeepEquals, expected)
}

func (s *statusHistoryReaderSuite) TestWalkWithDifferentLabel(c *gc.C) {
	var expected []HistoryRecord

	modelUUID := "model-uuid"

	path := s.createFile(c, func(c *gc.C, w io.Writer) {
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
			c.Assert(err, jc.ErrorIsNil)
		}
	})

	history, err := ModelStatusHistoryReaderFromFile(model.UUID(modelUUID), path)
	c.Assert(err, jc.ErrorIsNil)

	var records []HistoryRecord
	err = history.Walk(func(rec HistoryRecord) (bool, error) {
		records = append(records, rec)
		return false, nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(records, gc.DeepEquals, expected)
}

func (s *statusHistoryReaderSuite) TestWalkNoDocuments(c *gc.C) {
	var expected []HistoryRecord

	modelUUID := "model-uuid"

	path := s.createFile(c, func(c *gc.C, w io.Writer) {})

	history, err := ModelStatusHistoryReaderFromFile(model.UUID(modelUUID), path)
	c.Assert(err, jc.ErrorIsNil)

	var records []HistoryRecord
	err = history.Walk(func(rec HistoryRecord) (bool, error) {
		records = append(records, rec)
		return false, nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(records, gc.DeepEquals, expected)
}

func (s *statusHistoryReaderSuite) TestWalkCorruptLine(c *gc.C) {
	var expected []HistoryRecord

	modelUUID := "model-uuid"

	rnd := rand.IntN(9)

	path := s.createFile(c, func(c *gc.C, w io.Writer) {
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
			c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

	var records []HistoryRecord
	err = history.Walk(func(rec HistoryRecord) (bool, error) {
		records = append(records, rec)
		return false, nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(records, gc.DeepEquals, expected)
}

func (s *statusHistoryReaderSuite) createFile(c *gc.C, fn func(*gc.C, io.Writer)) string {
	path := c.MkDir()

	filePath := filepath.Join(path, "logsink.log")
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0644)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		_ = file.Close()
	}()

	fn(c, file)

	err = file.Sync()
	c.Assert(err, jc.ErrorIsNil)

	return filePath
}

func (s *statusHistoryReaderSuite) appendToFile(c *gc.C, path string, fn func(*gc.C, io.Writer)) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		_ = file.Close()
	}()

	fn(c, file)

	err = file.Sync()
	c.Assert(err, jc.ErrorIsNil)
}
