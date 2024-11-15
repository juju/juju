// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package changestreampruner

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.DBGetter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) getConfig(c *gc.C) WorkerConfig {
	return WorkerConfig{
		DBGetter: s.dbGetter,
		Clock:    s.clock,
		Logger:   loggertesting.WrapCheckLog(c),
	}
}

func (s *workerSuite) TestPrune(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerDBGet()

	pruner := s.newPruner(c)

	result, err := pruner.prune(context.Background())
	c.Check(err, jc.ErrorIsNil)

	// This ensures that we always prune the controller namespace.
	c.Check(result, gc.DeepEquals, map[string]int64{
		coredatabase.ControllerNS: 0,
	})
}

func (s *workerSuite) TestPruneControllerNS(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerDBGet()
	s.expectClock()

	pruner := s.newPruner(c)

	now := time.Now()

	s.insertControllerNodes(c, 1)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1002, UpdatedAt: now.Add(-time.Minute)})
	s.truncateChangeLog(c, s.TxnRunner())
	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.prune(context.Background())
	c.Check(err, jc.ErrorIsNil)

	// This ensures that we always prune the controller namespace.
	c.Check(result, gc.DeepEquals, map[string]int64{
		coredatabase.ControllerNS: 3,
	})
}

func (s *workerSuite) TestPruneModelList(c *gc.C) {
	defer s.setupMocks(c).Finish()

	txnRunner, db := s.OpenDB(c)
	defer db.Close()

	s.ApplyDDLForRunner(c, txnRunner)

	s.expectControllerDBGet()
	s.expectClock()

	pruner := s.newPruner(c)

	now := time.Now()

	s.insertControllerNodes(c, 1)
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "foo")
	s.expectDBGet(modelUUID.String(), txnRunner)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1002, UpdatedAt: now.Add(-time.Minute)})
	s.truncateChangeLog(c, s.TxnRunner())
	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.prune(context.Background())
	c.Check(err, jc.ErrorIsNil)

	// This ensures that we always prune the controller namespace.
	c.Check(result, gc.DeepEquals, map[string]int64{
		coredatabase.ControllerNS: 3,
		modelUUID.String():        0,
	})
}

func (s *workerSuite) TestPruneModelListWithChangeLogItems(c *gc.C) {
	defer s.setupMocks(c).Finish()

	txnRunner, db := s.OpenDB(c)
	defer db.Close()

	s.ApplyDDLForRunner(c, txnRunner)

	s.expectControllerDBGet()
	s.expectClock()

	pruner := s.newPruner(c)

	now := time.Now()

	s.insertControllerNodes(c, 1)
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "foo")
	s.expectDBGet(modelUUID.String(), txnRunner)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1002, UpdatedAt: now.Add(-time.Minute)})
	s.truncateChangeLog(c, s.TxnRunner())
	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	s.insertChangeLogWitness(c, txnRunner, Watermark{ControllerID: "0", LowerBound: 1003, UpdatedAt: now.Add(-time.Second)})
	s.truncateChangeLog(c, txnRunner)
	s.insertChangeLogItems(c, txnRunner, 0, 6, now)

	result, err := pruner.prune(context.Background())
	c.Check(err, jc.ErrorIsNil)

	// This ensures that we always prune the controller namespace.
	c.Check(result, gc.DeepEquals, map[string]int64{
		coredatabase.ControllerNS: 3,
		modelUUID.String():        4,
	})
}

func (s *workerSuite) TestPruneModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())

	pruner := s.newPruner(c)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(0))
}

func (s *workerSuite) TestPruneModelGetDBError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.dbGetter.EXPECT().GetDB("foo").Return(nil, errors.New("boom"))

	pruner := s.newPruner(c)

	_, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *workerSuite) TestPruneModelChangeLogWitness(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())
	s.expectClock()

	pruner := s.newPruner(c)

	now := time.Now()

	s.insertControllerNodes(c, 2)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1, UpdatedAt: now})

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(1))

	s.expectChangeLogWitnesses(c, s.TxnRunner(), []Watermark{{
		ControllerID: "0",
		LowerBound:   1,
		UpdatedAt:    now,
	}})
}

func (s *workerSuite) TestPruneModelLogsWarning(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// We request the db

	s.expectDBGetTimes("foo", s.TxnRunner(), 3)
	s.expectClock()

	var entries []string
	recorder := loggertesting.RecordLog(func(s string, a ...any) {
		entries = append(entries, s)
	})

	pruner := s.newPrunerWithLogger(c, loggertesting.WrapCheckLog(recorder))

	now := time.Now()

	s.insertControllerNodes(c, 2)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "3", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration + time.Minute))})

	s.insertChangeLogItems(c, s.TxnRunner(), 0, 1, now)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(1))

	// Should not prune anything as there are no new changes. Notice that the
	// warning is not logged.

	result, err = pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(0))

	// Add some new changes and it should log the warning.

	now = time.Now()

	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 2, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "3", LowerBound: 2, UpdatedAt: now.Add(-(defaultWindowDuration + time.Minute))})

	s.insertChangeLogItems(c, s.TxnRunner(), 1, 1, now)

	result, err = pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(1))

	c.Check(entries, gc.DeepEquals, []string{
		"WARNING: namespace %s watermarks %q are outside of window, check logs to see if the change stream is keeping up",
		"WARNING: namespace %s watermarks %q are outside of window, check logs to see if the change stream is keeping up",
	})
}

func (s *workerSuite) TestPruneModelRemovesChangeLogItems(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())
	s.expectClock()

	pruner := s.newPruner(c)

	now := time.Now()

	totalCtrlNodes := 2
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1002, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "3", LowerBound: 1003, UpdatedAt: now.Add(-time.Second)})

	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(3+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 7, 1003, 1010)
}

func (s *workerSuite) TestPruneModelRemovesChangeLogItemsWithMultipleWatermarks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())
	s.expectClock()

	pruner := s.newPruner(c)

	now := time.Now()

	totalCtrlNodes := 2
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1005, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "1", LowerBound: 1002, UpdatedAt: now.Add(-time.Second)})

	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(3+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 7, 1003, 1010)
}

func (s *workerSuite) TestPruneModelRemovesChangeLogItemsWithMultipleWatermarksWithOneOutsideWindow(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())
	s.expectClock()

	pruner := s.newPruner(c)

	now := time.Now()

	totalCtrlNodes := 3
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1005, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "1", LowerBound: 1002, UpdatedAt: now.Add(-time.Second)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "2", LowerBound: 1001, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))})

	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(2+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 8, 1002, 1010)
}

func (s *workerSuite) TestPruneModelRemovesChangeLogItemsWithMultipleWatermarksMoreWatermarks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())
	s.expectClock()

	pruner := s.newPruner(c)

	now := time.Now()

	totalCtrlNodes := 3
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1005, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "1", LowerBound: 1002, UpdatedAt: now.Add(-time.Second)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "2", LowerBound: 1001, UpdatedAt: now.Add(-time.Second)})

	s.insertChangeLogItems(c, s.TxnRunner(), 0, 10, now)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(2+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 8, 1002, 1010)
}

func (s *workerSuite) TestWindowContains(c *gc.C) {
	now := time.Now()
	testCases := []struct {
		window   window
		other    window
		expected bool
	}{{
		window:   window{start: now, end: now},
		other:    window{start: now, end: now},
		expected: true,
	}, {
		window:   window{start: now.Add(-time.Minute), end: now.Add(time.Minute)},
		other:    window{start: now, end: now},
		expected: true,
	}, {
		window:   window{start: now.Add(time.Minute), end: now.Add(-time.Minute)},
		other:    window{start: now, end: now},
		expected: false,
	}, {
		window:   window{start: now.Add(time.Minute), end: now.Add(time.Minute)},
		other:    window{start: now, end: now},
		expected: false,
	}, {
		window:   window{start: now.Add(-time.Minute), end: now.Add(-time.Minute)},
		other:    window{start: now, end: now},
		expected: false,
	}, {
		window:   window{start: now, end: now.Add(time.Minute * 2)},
		other:    window{start: now.Add(time.Minute), end: now.Add(time.Minute + time.Second)},
		expected: true,
	}, {
		window:   window{start: now, end: now.Add(time.Minute * 2)},
		other:    window{start: now.Add(time.Nanosecond), end: now.Add((time.Minute * 2) - time.Nanosecond)},
		expected: true,
	}, {
		window:   window{start: now, end: now.Add(time.Minute * 2)},
		other:    window{start: now, end: now.Add((time.Minute * 2) - time.Nanosecond)},
		expected: false,
	}, {
		window:   window{start: now, end: now.Add(time.Minute * 2)},
		other:    window{start: now.Add(time.Nanosecond), end: now.Add(time.Minute * 2)},
		expected: false,
	}}
	for i, test := range testCases {
		c.Logf("test %d", i)

		got := test.window.Contains(test.other)
		c.Check(got, gc.Equals, test.expected)
	}
}

func (s *workerSuite) TestWindowEquals(c *gc.C) {
	now := time.Now()
	testCases := []struct {
		window   window
		other    window
		expected bool
	}{{
		window:   window{start: now, end: now},
		other:    window{start: now, end: now},
		expected: true,
	}, {
		window:   window{start: now.Add(-time.Minute), end: now.Add(time.Minute)},
		other:    window{start: now, end: now},
		expected: false,
	}}
	for i, test := range testCases {
		c.Logf("test %d", i)

		got := test.window.Equals(test.other)
		c.Check(got, gc.Equals, test.expected)
	}
}

func (s *workerSuite) TestLowestWatermark(c *gc.C) {
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

		got := sortWatermarks("foo", test.watermarks)
		c.Check(got, jc.DeepEquals, test.expected)
	}
}

func (s *workerSuite) newPruner(c *gc.C) *Pruner {
	return s.newPrunerWithLogger(c, loggertesting.WrapCheckLog(c))
}

func (s *workerSuite) newPrunerWithLogger(c *gc.C, logger logger.Logger) *Pruner {
	return &Pruner{
		cfg: WorkerConfig{
			DBGetter: s.dbGetter,
			Clock:    s.clock,
			Logger:   logger,
		},
		windows: make(map[string]window),
	}
}

func (s *workerSuite) insertControllerNodes(c *gc.C, amount int) {
	query, err := sqlair.Prepare(`
INSERT INTO controller_node (controller_id, dqlite_node_id, bind_address)
VALUES ($M.ctrl_id, $M.node_id, $M.addr)
			`, sqlair.M{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for i := 0; i < amount; i++ {
			err := tx.Query(ctx, query, sqlair.M{
				"ctrl_id": strconv.Itoa(i + 1),
				"node_id": i,
				"addr":    fmt.Sprintf("127.0.1.%d", i+2),
			}).Run()
			c.Assert(err, jc.ErrorIsNil)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerSuite) insertChangeLogWitness(c *gc.C, runner coredatabase.TxnRunner, watermarks ...Watermark) {
	query, err := sqlair.Prepare(`
INSERT INTO change_log_witness (controller_id, lower_bound, updated_at)
VALUES ($M.ctrl_id, $M.lower_bound, $M.updated_at)
ON CONFLICT (controller_id) DO UPDATE SET lower_bound = $M.lower_bound, updated_at = $M.updated_at;
			`, sqlair.M{})
	c.Assert(err, jc.ErrorIsNil)

	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for _, watermark := range watermarks {
			err := tx.Query(ctx, query, sqlair.M{
				"ctrl_id":     watermark.ControllerID,
				"lower_bound": watermark.LowerBound,
				"updated_at":  watermark.UpdatedAt,
			}).Run()
			c.Assert(err, jc.ErrorIsNil)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerSuite) insertChangeLogItems(c *gc.C, runner coredatabase.TxnRunner, start, amount int, now time.Time) {
	query, err := sqlair.Prepare(`
INSERT INTO change_log (id, edit_type_id, namespace_id, changed, created_at)
VALUES ($M.id, 4, 2, 0, $M.created_at);
			`, sqlair.M{})
	c.Assert(err, jc.ErrorIsNil)

	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for i := start; i < amount; i++ {
			err := tx.Query(ctx, query, sqlair.M{
				"id":         i + 1000,
				"created_at": now,
			}).Run()
			c.Assert(err, jc.ErrorIsNil)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerSuite) expectChangeLogWitnesses(c *gc.C, runner coredatabase.TxnRunner, watermarks []Watermark) {
	query, err := sqlair.Prepare(`
SELECT (controller_id, lower_bound, updated_at) AS (&Watermark.*) FROM change_log_witness;
`, Watermark{})
	c.Assert(err, jc.ErrorIsNil)

	var got []Watermark
	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).GetAll(&got)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, jc.DeepEquals, watermarks)
}

func (s *workerSuite) expectChangeLogItems(c *gc.C, runner coredatabase.TxnRunner, amount, lowerBound, upperBound int) {
	query, err := sqlair.Prepare(`
SELECT (id, edit_type_id, namespace_id, changed, created_at) AS (&ChangeLogItem.*) FROM change_log;
	`, ChangeLogItem{})
	c.Assert(err, jc.ErrorIsNil)

	var got []ChangeLogItem
	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).GetAll(&got)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(got), gc.Equals, amount)
	for i, item := range got {
		if item.ID < lowerBound || item.ID > upperBound {
			c.Errorf("item %d: id %d not in range %d-%d", i, item.ID, lowerBound, upperBound)
		}

		c.Check(item.EditTypeID, gc.Equals, 4)
		c.Check(item.Namespace, gc.Equals, 2)
		c.Check(item.Changed, gc.Equals, 0)
	}
}

func (s *workerSuite) truncateChangeLog(c *gc.C, runner coredatabase.TxnRunner) {
	query, err := sqlair.Prepare(`DELETE FROM change_log;`, sqlair.M{})
	c.Assert(err, jc.ErrorIsNil)

	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, query).Run()
	})
	c.Assert(err, jc.ErrorIsNil)
}

type ChangeLogItem struct {
	ID         int       `db:"id"`
	EditTypeID int       `db:"edit_type_id"`
	Namespace  int       `db:"namespace_id"`
	Changed    int       `db:"changed"`
	CreatedAt  time.Time `db:"created_at"`
}
