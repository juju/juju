// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/testing"
)

type stateSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestEnsureUpgradeInfo(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	info, nodeInfos, err := st.EnsureUpgradeInfo(context.Background(), "0", version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(info, gc.Equals, Info{
		UUID:            info.UUID,
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		InitTime:        info.InitTime,
	})
	c.Assert(nodeInfos, gc.HasLen, 1)
	c.Assert(nodeInfos[0], gc.Equals, InfoControllerNode{
		ControllerNodeID: "0",
		NodeStatus:       "ready",
	})
}

func (s *stateSuite) TestEnsureUpgradeInfoIdempotent(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	_, _, err := st.EnsureUpgradeInfo(context.Background(), "0", version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	info, nodeInfos, err := st.EnsureUpgradeInfo(context.Background(), "0", version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(info, gc.Equals, Info{
		UUID:            info.UUID,
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		InitTime:        info.InitTime,
	})
	c.Assert(nodeInfos, gc.HasLen, 1)
	c.Assert(nodeInfos[0], gc.Equals, InfoControllerNode{
		ControllerNodeID: "0",
		NodeStatus:       "ready",
	})
}

func (s *stateSuite) TestEnsureUpgradeInfoMultipleControllers(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id) VALUES ('1')")
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = st.EnsureUpgradeInfo(context.Background(), "0", version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	info, nodeInfos, err := st.EnsureUpgradeInfo(context.Background(), "1", version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(info, gc.Equals, Info{
		UUID:            info.UUID,
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		InitTime:        info.InitTime,
	})
	c.Assert(nodeInfos, gc.HasLen, 2)
	c.Assert(nodeInfos[0], gc.Equals, InfoControllerNode{
		ControllerNodeID: "0",
		NodeStatus:       "ready",
	})
	c.Assert(nodeInfos[1], gc.Equals, InfoControllerNode{
		ControllerNodeID: "1",
		NodeStatus:       "ready",
	})
}

func (s *stateSuite) TestEnsureUpgradeInfoWithBadVersion(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	_, _, err := st.EnsureUpgradeInfo(context.Background(), "0", version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = st.EnsureUpgradeInfo(context.Background(), "0", version.MustParse("3.0.0"), version.MustParse("3.0.2"))
	c.Assert(errors.IsNotValid(err), jc.IsTrue)

	_, _, err = st.EnsureUpgradeInfo(context.Background(), "0", version.MustParse("2.9.44"), version.MustParse("3.0.1"))
	c.Assert(errors.IsNotValid(err), jc.IsTrue)
}

func (s *stateSuite) TestGetCurrentUpgrade(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))
	db, err := st.DB()
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = st.EnsureUpgradeInfo(context.Background(), "0", version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	var (
		info      Info
		nodeInfos []InfoControllerNode
	)
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		info, nodeInfos, err = st.getCurrentUpgrade(ctx, tx)
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(info, gc.Equals, Info{
		UUID:            info.UUID,
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		InitTime:        info.InitTime,
	})
	c.Assert(nodeInfos, gc.HasLen, 1)
	c.Assert(nodeInfos[0], gc.Equals, InfoControllerNode{
		ControllerNodeID: "0",
		NodeStatus:       "ready",
	})
}

func (s *stateSuite) TestIsUpgradingFalse(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	upgrading, err := st.IsUpgrading(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(upgrading, jc.IsFalse)
}

func (s *stateSuite) TestIsUpgradingTrue(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	_, _, err := st.EnsureUpgradeInfo(context.Background(), "0", version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	upgrading, err := st.IsUpgrading(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(upgrading, jc.IsTrue)
}
