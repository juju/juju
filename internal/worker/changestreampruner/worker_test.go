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
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.DBGetter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) getConfig() WorkerConfig {
	return WorkerConfig{
		DBGetter: s.dbGetter,
		Clock:    s.clock,
		Logger:   s.logger,
	}
}

func (s *workerSuite) TestPrune(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerDBGet()
	s.expectAnyLogs(c)

	pruner := s.newPruner(c)

	result, err := pruner.prune()
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
	s.expectAnyLogs(c)

	pruner := s.newPruner(c)

	now := time.Now()

	s.insertControllerNodes(c, 1)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1002, UpdatedAt: now.Add(-time.Minute)})
	s.truncateChangeLog(c, s.TxnRunner())
	s.insertChangeLogItems(c, s.TxnRunner(), 10, now)

	result, err := pruner.prune()
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
	s.expectDBGet("foo", txnRunner)
	s.expectClock()
	s.expectAnyLogs(c)

	pruner := s.newPruner(c)

	now := time.Now()

	s.insertControllerNodes(c, 1)
	s.insertModelList(c, "foo")
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1002, UpdatedAt: now.Add(-time.Minute)})
	s.truncateChangeLog(c, s.TxnRunner())
	s.insertChangeLogItems(c, s.TxnRunner(), 10, now)

	result, err := pruner.prune()
	c.Check(err, jc.ErrorIsNil)

	// This ensures that we always prune the controller namespace.
	c.Check(result, gc.DeepEquals, map[string]int64{
		coredatabase.ControllerNS: 3,
		"foo":                     0,
	})
}

func (s *workerSuite) TestPruneModelListWithChangeLogItems(c *gc.C) {
	defer s.setupMocks(c).Finish()

	txnRunner, db := s.OpenDB(c)
	defer db.Close()

	s.ApplyDDLForRunner(c, txnRunner)

	s.expectControllerDBGet()
	s.expectDBGet("foo", txnRunner)
	s.expectClock()
	s.expectAnyLogs(c)

	pruner := s.newPruner(c)

	now := time.Now()

	s.insertControllerNodes(c, 1)
	s.insertModelList(c, "foo")
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1002, UpdatedAt: now.Add(-time.Minute)})
	s.truncateChangeLog(c, s.TxnRunner())
	s.insertChangeLogItems(c, s.TxnRunner(), 10, now)

	s.insertChangeLogWitness(c, txnRunner, Watermark{ControllerID: "0", LowerBound: 1003, UpdatedAt: now.Add(-time.Second)})
	s.truncateChangeLog(c, txnRunner)
	s.insertChangeLogItems(c, txnRunner, 6, now)

	result, err := pruner.prune()
	c.Check(err, jc.ErrorIsNil)

	// This ensures that we always prune the controller namespace.
	c.Check(result, gc.DeepEquals, map[string]int64{
		coredatabase.ControllerNS: 3,
		"foo":                     4,
	})
}

func (s *workerSuite) TestPruneModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())
	s.expectAnyLogs(c)

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
	s.expectAnyLogs(c)

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

	s.expectDBGet("foo", s.TxnRunner())
	s.expectClock()

	s.logger.EXPECT().Warningf("Watermark %q is outside of window, check logs to see if the change stream is keeping up", gomock.Any()).Do(c.Logf).Times(2)

	pruner := s.newPruner(c)

	now := time.Now()

	s.insertControllerNodes(c, 2)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "3", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration + time.Minute))})

	s.insertChangeLogItems(c, s.TxnRunner(), 1, now)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(1))
}

func (s *workerSuite) TestPruneModelRemovesChangeLogItems(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())
	s.expectClock()
	s.expectAnyLogs(c)

	pruner := s.newPruner(c)

	now := time.Now()

	totalCtrlNodes := 2
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1002, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "3", LowerBound: 1003, UpdatedAt: now.Add(-time.Second)})

	s.insertChangeLogItems(c, s.TxnRunner(), 10, now)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(3+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 7, 1003, 1010)
}

func (s *workerSuite) TestPruneModelRemovesChangeLogItemsWithMultipleWatermarks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())
	s.expectClock()
	s.expectAnyLogs(c)

	pruner := s.newPruner(c)

	now := time.Now()

	totalCtrlNodes := 2
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1005, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "1", LowerBound: 1002, UpdatedAt: now.Add(-time.Second)})

	s.insertChangeLogItems(c, s.TxnRunner(), 10, now)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(3+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 7, 1003, 1010)
}

func (s *workerSuite) TestPruneModelRemovesChangeLogItemsWithMultipleWatermarksWithOneOutsideWindow(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())
	s.expectClock()
	s.expectAnyLogs(c)

	pruner := s.newPruner(c)

	now := time.Now()

	totalCtrlNodes := 3
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1005, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "1", LowerBound: 1002, UpdatedAt: now.Add(-time.Second)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "2", LowerBound: 1001, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))})

	s.insertChangeLogItems(c, s.TxnRunner(), 10, now)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(2+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 8, 1002, 1010)
}

func (s *workerSuite) TestPruneModelRemovesChangeLogItemsWithMultipleWatermarksMoreWatermarks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDBGet("foo", s.TxnRunner())
	s.expectClock()
	s.expectAnyLogs(c)

	pruner := s.newPruner(c)

	now := time.Now()

	totalCtrlNodes := 3
	s.insertControllerNodes(c, totalCtrlNodes)
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "0", LowerBound: 1005, UpdatedAt: now.Add(-time.Minute)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "1", LowerBound: 1002, UpdatedAt: now.Add(-time.Second)})
	s.insertChangeLogWitness(c, s.TxnRunner(), Watermark{ControllerID: "2", LowerBound: 1001, UpdatedAt: now.Add(-time.Second)})

	s.insertChangeLogItems(c, s.TxnRunner(), 10, now)

	result, err := pruner.pruneModel(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, int64(2+totalCtrlNodes))

	s.expectChangeLogItems(c, s.TxnRunner(), 8, 1002, 1010)
}

func (s *workerSuite) TestWindowContains(c *gc.C) {
	now := time.Now()
	testCases := []struct {
		window   window
		now      time.Time
		expected bool
	}{{
		window:   window{start: now.Add(-time.Minute), end: now},
		now:      now,
		expected: true,
	}, {
		window:   window{start: now.Add(-time.Minute), end: now},
		now:      now.Add(-time.Minute),
		expected: true,
	}, {
		window:   window{start: now.Add(-time.Minute), end: now},
		now:      now.Add(-(time.Minute * 2)),
		expected: false,
	}, {
		window:   window{start: now.Add(-time.Minute), end: now},
		now:      now.Add(time.Minute * 2),
		expected: false,
	}}
	for i, test := range testCases {
		c.Logf("test %d", i)

		got := test.window.contains(test.now)
		c.Check(got, gc.Equals, test.expected)
	}
}

func (s *workerSuite) TestLowestWatermark(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs(c)

	now := time.Now()
	testCases := []struct {
		watermarks []Watermark
		now        time.Time
		expected   Watermark
		expectedOK bool
	}{{
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
		},
		now:        now,
		expected:   Watermark{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
		expectedOK: true,
	}, {
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now},
		},
		now:        now,
		expected:   Watermark{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
		expectedOK: true,
	}, {
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
			{ControllerID: "1", LowerBound: 10, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))},
		},
		now:        now,
		expected:   Watermark{ControllerID: "0", LowerBound: 1, UpdatedAt: now},
		expectedOK: true,
	}, {
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 2, UpdatedAt: now},
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration - time.Second))},
		},
		now:        now,
		expected:   Watermark{ControllerID: "1", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration - time.Second))},
		expectedOK: true,
	}, {
		watermarks: []Watermark{
			{ControllerID: "0", LowerBound: 2, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))},
			{ControllerID: "1", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))},
		},
		now: now,
		// TODO (stickupkid): This should be false, but we need a strategy for
		// removing nodes that are not keeping up. We're logging a warning
		// instead.
		expected:   Watermark{ControllerID: "1", LowerBound: 1, UpdatedAt: now.Add(-(defaultWindowDuration + time.Second))},
		expectedOK: true,
	}}

	for i, test := range testCases {
		c.Logf("test %d", i)

		got, ok := s.newPruner(c).lowestWatermark(test.watermarks, test.now)
		c.Check(got, jc.DeepEquals, test.expected)
		c.Check(ok, gc.Equals, test.expectedOK)
	}
}

func (s *workerSuite) newPruner(c *gc.C) *Pruner {
	return &Pruner{
		cfg: WorkerConfig{
			DBGetter: s.dbGetter,
			Clock:    s.clock,
			Logger:   s.logger,
		},
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

func (s *workerSuite) insertModelList(c *gc.C, namespace string) {
	query, err := sqlair.Prepare(`
INSERT INTO model_list (uuid)
VALUES ($M.uuid);
`, sqlair.M{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, sqlair.M{"uuid": namespace}).Run()
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerSuite) insertChangeLogWitness(c *gc.C, runner coredatabase.TxnRunner, watermarks ...Watermark) {
	query, err := sqlair.Prepare(`
INSERT INTO change_log_witness (controller_id, lower_bound, updated_at)
VALUES ($M.ctrl_id, $M.lower_bound, $M.updated_at)
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

func (s *workerSuite) insertChangeLogItems(c *gc.C, runner coredatabase.TxnRunner, amount int, now time.Time) {
	query, err := sqlair.Prepare(`
INSERT INTO change_log (id, edit_type_id, namespace_id, changed, created_at)
VALUES ($M.id, 4, 2, 0, $M.created_at);
			`, sqlair.M{})
	c.Assert(err, jc.ErrorIsNil)

	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for i := 0; i < amount; i++ {
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

	var called bool
	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		called = true

		var got []Watermark
		err := tx.Query(ctx, query).GetAll(&got)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(got, jc.DeepEquals, watermarks)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *workerSuite) expectChangeLogItems(c *gc.C, runner coredatabase.TxnRunner, amount, lowerBound, upperBound int) {
	query, err := sqlair.Prepare(`
SELECT (id, edit_type_id, namespace_id, changed, created_at) AS (&ChangeLogItem.*) FROM change_log;
	`, ChangeLogItem{})
	c.Assert(err, jc.ErrorIsNil)

	var called bool
	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		called = true

		var got []ChangeLogItem
		err := tx.Query(ctx, query).GetAll(&got)
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

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
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
