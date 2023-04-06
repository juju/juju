// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry_test

import (
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/testing"
	"github.com/juju/juju/worker/leaseexpiry"
)

type workerSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestConfigValidate(c *gc.C) {
	validCfg := leaseexpiry.Config{
		Clock:     clock.WallClock,
		Logger:    leaseexpiry.StubLogger{},
		TrackedDB: s.TrackedDB(),
	}

	cfg := validCfg
	cfg.Clock = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = validCfg
	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = validCfg
	cfg.TrackedDB = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *workerSuite) TestWorkerDeletesExpiredLeases(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	clk := NewMockClock(ctrl)
	timer := NewMockTimer(ctrl)

	var w worker.Worker
	var wmutex sync.Mutex

	clk.EXPECT().NewTimer(time.Second).Return(timer)

	// Kill the worker on the first pass through the loop,
	// after we've processed one expiration.
	ch := make(chan time.Time, 1)
	ch <- time.Now()
	timer.EXPECT().Chan().Return(ch).MinTimes(1)
	timer.EXPECT().Reset(time.Second).Do(func(any) {
		wmutex.Lock()
		defer wmutex.Unlock()
		w.Kill()
	})
	timer.EXPECT().Stop().Return(true)

	// Insert 2 leases, one with an expiry time in the past,
	// another in the future.
	q := `
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES (?, 1, 'some-model-uuid', ?, ?, datetime('now'), datetime('now', ?))`[1:]

	stmt, err := s.DB().Prepare(q)
	c.Assert(err, jc.ErrorIsNil)

	_, err = stmt.Exec(utils.MustNewUUID().String(), "postgresql", "postgresql/0", "+2 minutes")
	c.Assert(err, jc.ErrorIsNil)

	_, err = stmt.Exec(utils.MustNewUUID().String(), "redis", "redis/0", "-2 minutes")
	c.Assert(err, jc.ErrorIsNil)

	wmutex.Lock()
	w, err = leaseexpiry.NewWorker(leaseexpiry.Config{
		Clock:     clk,
		Logger:    leaseexpiry.StubLogger{},
		TrackedDB: s.TrackedDB(),
	})
	wmutex.Unlock()
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, jc.ErrorIsNil)

	// Only the postgresql lease (expiring in the future) should remain.
	row := s.DB().QueryRow("SELECT name FROM LEASE")
	var name string
	err = row.Scan(&name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(row.Err(), jc.ErrorIsNil)

	c.Check(name, gc.Equals, "postgresql")
}
