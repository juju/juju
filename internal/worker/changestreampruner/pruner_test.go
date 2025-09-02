// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package changestreampruner

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	time "time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type prunerWorkerSuite struct {
	baseSuite
}

func TestPrunerWorkerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &prunerWorkerSuite{})
}

func (s *prunerWorkerSuite) TestPrunerDies(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// No update to the window, as there is nothing to prune.
	done := make(chan struct{})
	s.namespaceWindow.EXPECT().CurrentWindow().DoAndReturn(func() Window {
		return Window{start: time.Now().Add(-time.Hour), end: time.Now()}
	})
	s.namespaceWindow.EXPECT().UpdateWindow(gomock.Any()).DoAndReturn(func(w Window) {
		close(done)
	})

	pruner := s.newPruner(c)
	defer workertest.CleanKill(c, pruner)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for pruner to run")
	}
}

func (s *prunerWorkerSuite) TestPruneModelList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)

	s.namespaceWindow.EXPECT().CurrentWindow().Return(Window{
		start: now,
		end:   now,
	})
	s.namespaceWindow.EXPECT().UpdateWindow(gomock.Any()).Do(func(w Window) {
		c.Check(w, tc.DeepEquals, Window{
			start: now.Add(-time.Minute),
			end:   now.Add(-time.Minute),
		})
	})

	// We want to test the prune method, not the loop.
	pruner := &modelPruner{
		db:              s.TxnRunner(),
		namespaceWindow: s.namespaceWindow,
		clock:           clock.WallClock,
		logger:          loggertesting.WrapCheckLog(c),
	}

	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1002, UpdatedAt: now.Add(-time.Minute)})
	s.truncateChangeLog(c, s.TxnRunner())
	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.prune(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// This ensures that we always prune the controller namespace.
	c.Check(result, tc.Equals, int64(3))
}

func (s *prunerWorkerSuite) TestPruneModelChangeLogWitness(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	s.namespaceWindow.EXPECT().CurrentWindow().Return(Window{
		start: now,
		end:   now,
	})
	s.namespaceWindow.EXPECT().UpdateWindow(gomock.Any()).Do(func(w Window) {
		c.Check(w, tc.DeepEquals, Window{
			start: now,
			end:   now,
		})
	})

	// We want to test the prune method, not the loop.
	pruner := &modelPruner{
		db:              s.TxnRunner(),
		namespaceWindow: s.namespaceWindow,
		clock:           clock.WallClock,
		logger:          loggertesting.WrapCheckLog(c),
	}

	s.insertControllerNodes(c, 2)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1, UpdatedAt: now})

	result, err := pruner.prune(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, int64(1))

	s.expectChangeLogWitnesses(c, s.TxnRunner(), []Watermark{{
		ControllerID: "0",
		LowerBound:   1,
		UpdatedAt:    now,
	}})
}

func (s *prunerWorkerSuite) TestPruneModelLogsWarning(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)
	window := &Window{
		start: now,
		end:   now,
	}
	s.namespaceWindow.EXPECT().CurrentWindow().DoAndReturn(func() Window {
		return *window
	}).Times(3)
	s.namespaceWindow.EXPECT().UpdateWindow(gomock.Any()).DoAndReturn(func(w Window) {
		window = &w
	}).Times(3)

	var entries []string
	recorder := loggertesting.RecordLog(func(s string, a ...any) {
		entries = append(entries, s)
	})

	pruner := &modelPruner{
		db:              s.TxnRunner(),
		namespaceWindow: s.namespaceWindow,
		clock:           clock.WallClock,
		logger:          loggertesting.WrapCheckLog(recorder),
	}

	s.insertControllerNodes(c, 2)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "3", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration + time.Minute))})

	s.insertChangeLogItems(c, s.TxnRunner(), 0, 1, now)

	result, err := pruner.prune(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, int64(1))

	c.Assert(entries, tc.DeepEquals, []string{
		"WARNING: watermarks %q are outside of window, check logs to see if the change stream is keeping up",
	})

	// Should not prune anything as there are no new changes. Notice that the
	// warning is not logged.

	result, err = pruner.prune(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, int64(0))

	// Add some new changes and it should log the warning.

	now = time.Now()

	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 2, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "3", LowerBound: 2, UpdatedAt: now.Add(-(defaultWindowDuration + time.Minute))})

	s.insertChangeLogItems(c, s.TxnRunner(), 1, 1, now)

	result, err = pruner.prune(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, int64(1))

	c.Check(entries, tc.DeepEquals, []string{
		"WARNING: watermarks %q are outside of window, check logs to see if the change stream is keeping up",
		"WARNING: watermarks %q are outside of window, check logs to see if the change stream is keeping up",
	})
}

func (s *prunerWorkerSuite) TestPruneModelRemovesChangeLogItems(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)

	s.namespaceWindow.EXPECT().CurrentWindow().Return(Window{
		start: now,
		end:   now,
	})
	s.namespaceWindow.EXPECT().UpdateWindow(gomock.Any()).Do(func(w Window) {
		c.Check(w, tc.DeepEquals, Window{
			start: now.Add(-time.Minute),
			end:   now.Add(-time.Second),
		})
	})

	pruner := &modelPruner{
		db:              s.TxnRunner(),
		namespaceWindow: s.namespaceWindow,
		clock:           clock.WallClock,
		logger:          loggertesting.WrapCheckLog(c),
	}

	totalCtrlNodes := 2
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1002, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "3", LowerBound: 1003, UpdatedAt: now.Add(-time.Second)})

	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.prune(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, int64(3+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 7, 1003, 1010)
}

func (s *prunerWorkerSuite) TestPruneModelRemovesChangeLogItemsWithMultipleWatermarks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)

	s.namespaceWindow.EXPECT().CurrentWindow().Return(Window{
		start: now,
		end:   now,
	})
	s.namespaceWindow.EXPECT().UpdateWindow(gomock.Any()).Do(func(w Window) {
		c.Check(w, tc.DeepEquals, Window{
			start: now.Add(-time.Second),
			end:   now.Add(-time.Minute),
		})
	})

	pruner := &modelPruner{
		db:              s.TxnRunner(),
		namespaceWindow: s.namespaceWindow,
		clock:           clock.WallClock,
		logger:          loggertesting.WrapCheckLog(c),
	}

	totalCtrlNodes := 2
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1005, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "1", LowerBound: 1002, UpdatedAt: now.Add(-time.Second)})

	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.prune(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, int64(3+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 7, 1003, 1010)
}

func (s *prunerWorkerSuite) TestPruneModelRemovesChangeLogItemsWithMultipleWatermarksWithOneOutsideWindow(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)

	s.namespaceWindow.EXPECT().CurrentWindow().Return(Window{
		start: now,
		end:   now,
	})
	s.namespaceWindow.EXPECT().UpdateWindow(gomock.Any()).Do(func(w Window) {
		c.Check(w, tc.DeepEquals, Window{
			start: now.Add(-(defaultWindowDuration + time.Second)),
			end:   now.Add(-time.Minute),
		})
	})

	pruner := &modelPruner{
		db:              s.TxnRunner(),
		namespaceWindow: s.namespaceWindow,
		clock:           clock.WallClock,
		logger:          loggertesting.WrapCheckLog(c),
	}

	totalCtrlNodes := 3
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1005, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "1", LowerBound: 1002, UpdatedAt: now.Add(-time.Second)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "2", LowerBound: 1001, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))})

	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.prune(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, int64(2+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 8, 1002, 1010)
}

func (s *prunerWorkerSuite) TestPruneModelRemovesChangeLogItemsWithMultipleWatermarksMoreWatermarks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)

	s.namespaceWindow.EXPECT().CurrentWindow().Return(Window{
		start: now,
		end:   now,
	})
	s.namespaceWindow.EXPECT().UpdateWindow(gomock.Any()).Do(func(w Window) {
		c.Check(w, tc.DeepEquals, Window{
			start: now.Add(-time.Second),
			end:   now.Add(-time.Minute),
		})
	})

	pruner := &modelPruner{
		db:              s.TxnRunner(),
		namespaceWindow: s.namespaceWindow,
		clock:           clock.WallClock,
		logger:          loggertesting.WrapCheckLog(c),
	}

	totalCtrlNodes := 3
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1005, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "1", LowerBound: 1002, UpdatedAt: now.Add(-time.Second)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "2", LowerBound: 1001, UpdatedAt: now.Add(-time.Second)})

	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.prune(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, int64(2+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 8, 1002, 1010)
}

func (s *prunerWorkerSuite) TestWindowContains(c *tc.C) {
	now := time.Now()
	testCases := []struct {
		window   Window
		other    Window
		expected bool
	}{{
		window:   Window{start: now, end: now},
		other:    Window{start: now, end: now},
		expected: true,
	}, {
		window:   Window{start: now.Add(-time.Minute), end: now.Add(time.Minute)},
		other:    Window{start: now, end: now},
		expected: true,
	}, {
		window:   Window{start: now.Add(time.Minute), end: now.Add(-time.Minute)},
		other:    Window{start: now, end: now},
		expected: false,
	}, {
		window:   Window{start: now.Add(time.Minute), end: now.Add(time.Minute)},
		other:    Window{start: now, end: now},
		expected: false,
	}, {
		window:   Window{start: now.Add(-time.Minute), end: now.Add(-time.Minute)},
		other:    Window{start: now, end: now},
		expected: false,
	}, {
		window:   Window{start: now, end: now.Add(time.Minute * 2)},
		other:    Window{start: now.Add(time.Minute), end: now.Add(time.Minute + time.Second)},
		expected: true,
	}, {
		window:   Window{start: now, end: now.Add(time.Minute * 2)},
		other:    Window{start: now.Add(time.Nanosecond), end: now.Add((time.Minute * 2) - time.Nanosecond)},
		expected: true,
	}, {
		window:   Window{start: now, end: now.Add(time.Minute * 2)},
		other:    Window{start: now, end: now.Add((time.Minute * 2) - time.Nanosecond)},
		expected: false,
	}, {
		window:   Window{start: now, end: now.Add(time.Minute * 2)},
		other:    Window{start: now.Add(time.Nanosecond), end: now.Add(time.Minute * 2)},
		expected: false,
	}}
	for i, test := range testCases {
		c.Logf("test %d", i)

		got := test.window.Contains(test.other)
		c.Check(got, tc.Equals, test.expected)
	}
}

func (s *prunerWorkerSuite) TestWindowEquals(c *tc.C) {
	now := time.Now()
	testCases := []struct {
		window   Window
		other    Window
		expected bool
	}{{
		window:   Window{start: now, end: now},
		other:    Window{start: now, end: now},
		expected: true,
	}, {
		window:   Window{start: now.Add(-time.Minute), end: now.Add(time.Minute)},
		other:    Window{start: now, end: now},
		expected: false,
	}}
	for i, test := range testCases {
		c.Logf("test %d", i)

		got := test.window.Equals(test.other)
		c.Check(got, tc.Equals, test.expected)
	}
}

func (s *prunerWorkerSuite) TestLowestWatermark(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()
	testCases := []struct {
		watermarks []Watermark
		expected   []Watermark
	}{{
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
		},
		expected: []Watermark{
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
		},
	}, {
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now},
		},
		expected: []Watermark{
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now},
		},
	}, {
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
			{ControllerID: "1", LowerBound: 10, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))},
		},
		expected: []Watermark{
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
			{ControllerID: "1", LowerBound: 10, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))},
		},
	}, {
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 2, UpdatedAt: now},
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration - time.Second))},
		},
		expected: []Watermark{
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration - time.Second))},
			{ControllerID: "0", LowerBound: 2, UpdatedAt: now},
		},
	}, {
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration - time.Second))},
		},
		expected: []Watermark{
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration - time.Second))},
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
		},
	}, {
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 2, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))},
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))},
		},
		// TODO (stickupkid): This should be false, but we need a strategy for
		// removing nodes that are not keeping up. We're logging a warning
		// instead.
		expected: []Watermark{
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))},
			{ControllerID: "0", LowerBound: 2, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))},
		},
	}}

	for i, test := range testCases {
		c.Logf("test %d", i)

		got := sortWatermarks(test.watermarks)
		c.Check(got, tc.DeepEquals, test.expected)
	}
}

func (s *prunerWorkerSuite) newPruner(c *tc.C) *modelPruner {
	return s.newPrunerWithLogger(c, loggertesting.WrapCheckLog(c))
}

func (s *prunerWorkerSuite) newPrunerWithLogger(c *tc.C, logger logger.Logger) *modelPruner {
	w := NewModelPruner(s.TxnRunner(), s.namespaceWindow, s.clock, logger)
	return w.(*modelPruner)
}

func (s *prunerWorkerSuite) insertControllerNodes(c *tc.C, amount int) {
	query, err := sqlair.Prepare(`
INSERT INTO controller_node (controller_id, dqlite_node_id, dqlite_bind_address)
VALUES ($M.ctrl_id, $M.node_id, $M.addr)
			`, sqlair.M{})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		for i := 0; i < amount; i++ {
			err := tx.Query(ctx, query, sqlair.M{
				"ctrl_id": strconv.Itoa(i + 1),
				"node_id": i,
				"addr":    fmt.Sprintf("127.0.1.%d", i+2),
			}).Run()
			c.Assert(err, tc.ErrorIsNil)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *prunerWorkerSuite) insertChangeLogWitness(c *tc.C, runner coredatabase.TxnRunner, watermarks ...Watermark) {
	query, err := sqlair.Prepare(`
INSERT INTO change_log_witness (controller_id, lower_bound, updated_at)
VALUES ($M.ctrl_id, $M.lower_bound, $M.updated_at)
ON CONFLICT (controller_id) DO UPDATE SET lower_bound = $M.lower_bound, updated_at = $M.updated_at;
			`, sqlair.M{})
	c.Assert(err, tc.ErrorIsNil)

	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		for _, watermark := range watermarks {
			err := tx.Query(ctx, query, sqlair.M{
				"ctrl_id":     watermark.ControllerID,
				"lower_bound": watermark.LowerBound,
				"updated_at":  watermark.UpdatedAt,
			}).Run()
			c.Assert(err, tc.ErrorIsNil)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *prunerWorkerSuite) insertChangeLogItems(c *tc.C, runner coredatabase.TxnRunner, start, amount int, now time.Time) {
	query, err := sqlair.Prepare(`
INSERT INTO change_log (id, edit_type_id, namespace_id, changed, created_at)
VALUES ($M.id, 4, 10002, 0, $M.created_at);
			`, sqlair.M{})
	c.Assert(err, tc.ErrorIsNil)

	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		for i := start; i < amount; i++ {
			err := tx.Query(ctx, query, sqlair.M{
				"id":         i + 1000,
				"created_at": now,
			}).Run()
			c.Assert(err, tc.ErrorIsNil)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *prunerWorkerSuite) expectChangeLogWitnesses(c *tc.C, runner coredatabase.TxnRunner, watermarks []Watermark) {
	query, err := sqlair.Prepare(`
SELECT (controller_id, lower_bound, updated_at) AS (&Watermark.*) FROM change_log_witness;
`, Watermark{})
	c.Assert(err, tc.ErrorIsNil)

	var got []Watermark
	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).GetAll(&got)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, watermarks)
}

func (s *prunerWorkerSuite) expectChangeLogItems(c *tc.C, runner coredatabase.TxnRunner, amount, lowerBound, upperBound int) {
	query, err := sqlair.Prepare(`
SELECT (id, edit_type_id, namespace_id, changed, created_at) AS (&ChangeLogItem.*) FROM change_log;
	`, ChangeLogItem{})
	c.Assert(err, tc.ErrorIsNil)

	var got []ChangeLogItem
	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).GetAll(&got)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(got), tc.Equals, amount)
	for i, item := range got {
		if item.ID < lowerBound || item.ID > upperBound {
			c.Errorf("item %d: id %d not in range %d-%d", i, item.ID, lowerBound, upperBound)
		}

		c.Check(item.EditTypeID, tc.Equals, 4)
		c.Check(item.Namespace, tc.Equals, 10002)
		c.Check(item.Changed, tc.Equals, 0)
	}
}

func (s *prunerWorkerSuite) truncateChangeLog(c *tc.C, runner coredatabase.TxnRunner) {
	query, err := sqlair.Prepare(`DELETE FROM change_log;`)
	c.Assert(err, tc.ErrorIsNil)

	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, query).Run()
	})
	c.Assert(err, tc.ErrorIsNil)
}

type ChangeLogItem struct {
	ID         int       `db:"id"`
	EditTypeID int       `db:"edit_type_id"`
	Namespace  int       `db:"namespace_id"`
	Changed    int       `db:"changed"`
	CreatedAt  time.Time `db:"created_at"`
}
