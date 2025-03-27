// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
	state *State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory())
}

func (s *stateSuite) TestCurateNodes(c *gc.C) {
	db := s.DB()

	_, err := db.ExecContext(context.Background(), "INSERT INTO controller_node (controller_id) VALUES ('1')")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.CurateNodes(
		context.Background(), []string{"2", "3"}, []string{"1"})
	c.Assert(err, jc.ErrorIsNil)

	rows, err := db.QueryContext(context.Background(), "SELECT controller_id FROM controller_node")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	ids := set.NewStrings()
	for rows.Next() {
		var addr string
		err := rows.Scan(&addr)
		c.Assert(err, jc.ErrorIsNil)
		ids.Add(addr)
	}
	c.Check(ids.Values(), gc.HasLen, 3)

	// Controller "0" is inserted as part of the bootstrapped schema.
	c.Check(ids.Contains("0"), jc.IsTrue)
	c.Check(ids.Contains("2"), jc.IsTrue)
	c.Check(ids.Contains("3"), jc.IsTrue)
}

func (s *stateSuite) TestUpdateDqliteNode(c *gc.C) {
	// This value would cause a driver error to be emitted if we
	// tried to pass it directly as a uint64 query parameter.
	nodeID := uint64(15237855465837235027)

	err := s.state.UpdateDqliteNode(
		context.Background(), "0", nodeID, "192.168.5.60")
	c.Assert(err, jc.ErrorIsNil)

	row := s.DB().QueryRowContext(context.Background(), "SELECT dqlite_node_id, bind_address FROM controller_node WHERE controller_id = '0'")
	c.Assert(row.Err(), jc.ErrorIsNil)

	var (
		id   uint64
		addr string
	)
	err = row.Scan(&id, &addr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(id, gc.Equals, nodeID)
	c.Check(addr, gc.Equals, "192.168.5.60")
}

// TestSelectDatabaseNamespace is testing success for existing namespaces and
// a not found error for namespaces that don't exist.
func (s *stateSuite) TestSelectDatabaseNamespace(c *gc.C) {
	db := s.DB()
	_, err := db.ExecContext(context.Background(), "INSERT INTO namespace_list (namespace) VALUES ('simon!!')")
	c.Assert(err, jc.ErrorIsNil)

	st := s.state
	namespace, err := st.SelectDatabaseNamespace(context.Background(), "simon!!")
	c.Check(err, jc.ErrorIsNil)
	c.Check(namespace, gc.Equals, "simon!!")

	namespace, err = st.SelectDatabaseNamespace(context.Background(), "SIMon!!")
	c.Check(err, jc.ErrorIs, controllernodeerrors.NotFound)
	c.Check(namespace, gc.Equals, "")
}

func (s *stateSuite) TestSetRunningAgentBinaryVersionSuccess(c *gc.C) {
	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   corearch.ARM64,
	}

	err := s.state.CurateNodes(context.Background(), []string{controllerID}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Tests insert running agent binary version.
	err = s.state.SetRunningAgentBinaryVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, jc.ErrorIsNil)

	var (
		obtainedControllerID string
		obtainedVersion      string
		obtainedArchName     string
	)
	selectAgentVerQuery := `
	SELECT controller_id,
			c.version,
			a.name
	FROM controller_node_agent_version as c
	INNER JOIN architecture as a
	ON c.architecture_id = a.id
	WHERE controller_id = ?
			`
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {

		return tx.QueryRowContext(ctx, selectAgentVerQuery, controllerID).Scan(
			&obtainedControllerID,
			&obtainedVersion,
			&obtainedArchName,
		)
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedControllerID, gc.Equals, controllerID)
	c.Check(obtainedVersion, gc.Equals, ver.Number.String())
	c.Check(obtainedArchName, gc.Equals, ver.Arch)

	// Tests update running agent binary version.
	updatedVer := coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.AMD64,
	}
	err = s.state.SetRunningAgentBinaryVersion(
		context.Background(),
		controllerID,
		updatedVer,
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, selectAgentVerQuery, controllerID).Scan(
			&obtainedControllerID,
			&obtainedVersion,
			&obtainedArchName,
		)
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedControllerID, gc.Equals, controllerID)
	c.Check(obtainedVersion, gc.Equals, updatedVer.Number.String())
	c.Check(obtainedArchName, gc.Equals, updatedVer.Arch)
}

func (s *stateSuite) TestSetRunningAgentBinaryVersionControllerNodeNotFound(c *gc.C) {
	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   corearch.ARM64,
	}

	err := s.state.CurateNodes(context.Background(), []string{controllerID}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetRunningAgentBinaryVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestSetRunningAgentBinaryVersionArchNotSupported(c *gc.C) {
	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   corearch.UnsupportedArches[0],
	}

	err := s.state.CurateNodes(context.Background(), []string{controllerID}, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetRunningAgentBinaryVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}
