// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
)

type addMachineInternalSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&addMachineInternalSuite{})

type retryCommittedDatabase struct {
	Database
	committed   bool
	afterCommit func()
}

func (db *retryCommittedDatabase) Run(source jujutxn.TransactionSource) error {
	return db.Database.Run(func(attempt int) ([]txn.Op, error) {
		ops, err := source(attempt)
		if err != nil || db.committed {
			return ops, err
		}
		if err := db.Database.RunTransaction(ops); err != nil {
			return nil, err
		}
		db.committed = true
		if db.afterCommit != nil {
			db.afterCommit()
		}
		// Replay the builder after a committed transaction, as Run does
		// when the commit succeeds but its acknowledgement is lost.
		return nil, jujutxn.ErrTransientFailure
	})
}

func (s *addMachineInternalSuite) TestAddMachinesRetryAfterCommit(c *gc.C) {
	s.testAddMachinesRetryAfterCommit(c, false)
}

func (s *addMachineInternalSuite) TestAddMachinesRetryAfterCommitModelMigrating(c *gc.C) {
	s.testAddMachinesRetryAfterCommit(c, true)
}

func (s *addMachineInternalSuite) testAddMachinesRetryAfterCommit(c *gc.C, migrating bool) {
	for _, variant := range []string{"machines", "bootstrap", "container", "new-parent"} {
		c.Logf("creation variant: %s", variant)
		st := s.state
		if variant != "bootstrap" {
			st = s.newState(c)
		}
		template := MachineTemplate{Base: UbuntuBase("22.04"), Jobs: []MachineJob{JobHostUnits}}
		var parent *Machine
		if variant == "bootstrap" {
			template.Jobs = []MachineJob{JobManageModel}
		} else if variant == "container" {
			var err error
			parent, err = st.AddOneMachine(template)
			c.Assert(err, jc.ErrorIsNil)
		}
		db := &retryCommittedDatabase{Database: st.database}
		if migrating {
			db.afterCommit = func() {
				model, err := st.Model()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(model.SetMigrationMode(MigrationModeExporting), jc.ErrorIsNil)
			}
		}
		st.database = db
		s.AddCleanup(func(*gc.C) { st.database = db.Database })
		var machines []*Machine
		var err error
		expectedIds := []string{"0", "1"}
		switch variant {
		case "machines":
			machines, err = st.AddMachines(template, template)
		case "bootstrap":
			machines, err = st.AddMachines(template)
			expectedIds = []string{"0"}
		default:
			var child *Machine
			if variant == "container" {
				child, err = st.AddMachineInsideMachine(template, parent.Id(), instance.LXD)
			} else {
				child, err = st.AddMachineInsideNewMachine(template, template, instance.LXD)
			}
			c.Assert(err, jc.ErrorIsNil)
			parentId, ok := child.ParentId()
			c.Assert(ok, jc.IsTrue)
			parent, err = st.Machine(parentId)
			c.Assert(err, jc.ErrorIsNil)
			children, err := parent.Containers()
			c.Assert(err, jc.ErrorIsNil)
			c.Check(children, gc.DeepEquals, []string{child.Id()})
			machines = []*Machine{parent, child}
			expectedIds = []string{"0", "0/lxd/0"}
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Check(db.committed, jc.IsTrue)
		var ids []string
		for _, machine := range machines {
			ids = append(ids, machine.Id())
		}
		c.Check(ids, gc.DeepEquals, expectedIds)
		all, err := st.AllMachines()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(all, gc.HasLen, len(expectedIds))
		if variant == "bootstrap" {
			ids, err := st.ControllerIds()
			c.Assert(err, jc.ErrorIsNil)
			c.Check(ids, gc.DeepEquals, expectedIds)
		}
		st.database = db.Database
	}
}

func (s *addMachineInternalSuite) TestAddMachinesRetryAfterCommitReadError(c *gc.C) {
	for _, container := range []bool{false, true} {
		st := s.newState(c)
		originalDB := st.database
		s.AddCleanup(func(*gc.C) { st.database = originalDB })
		failure := errors.New("cannot read committed machine")
		st.database = &retryCommittedDatabase{
			Database: originalDB,
			afterCommit: func() {
				st.database = &failCollectionDatabase{
					Database: originalDB, collection: machinesC, failAt: 1, err: failure,
				}
			},
		}
		template := MachineTemplate{Base: UbuntuBase("22.04"), Jobs: []MachineJob{JobHostUnits}}
		var err error
		if container {
			_, err = st.AddMachineInsideNewMachine(template, template, instance.LXD)
		} else {
			_, err = st.AddMachines(template)
		}
		c.Check(err, jc.ErrorIs, failure)
	}
}
