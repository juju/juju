// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"strconv"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/domain/schema"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/database/testing"
)

type stateSuite struct {
	testing.DqliteSuite
	state *State
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.DqliteSuite.SetUpTest(c)
	s.DqliteSuite.ApplyDDL(c, &schematesting.SchemaApplier{
		Schema:  schema.ControllerDDL(),
		Verbose: s.Verbose,
	})
	s.state = NewState(s.TxnRunnerFactory())
}

func (s *stateSuite) TestAddDqliteNodeID(c *tc.C) {
	err := s.state.AddDqliteNodeID(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)

	var controllerID string
	err = s.DB().QueryRowContext(c.Context(), `
SELECT controller_id FROM controller_node WHERE controller_id = '1'
`).Scan(&controllerID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerID, tc.Equals, "1")
}

func (s *stateSuite) TestAddDqliteNodeIDIsIdempotent(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "1"), tc.ErrorIsNil)
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "1"), tc.ErrorIsNil)
}

func (s *stateSuite) TestAddDqliteNodeIDDoesNotReviveDeadNode(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "1"), tc.ErrorIsNil)
	_, err := s.DB().ExecContext(c.Context(), `
UPDATE controller_node SET life_id = 2 WHERE controller_id = '1'`)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.AddDqliteNodeID(c.Context(), "1")
	c.Assert(err, tc.ErrorIs, controllernodeerrors.NotFound)
}

func (s *stateSuite) TestIsControllerNodeWhenDying(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "1"), tc.ErrorIsNil)
	_, err := s.DB().ExecContext(c.Context(), `
UPDATE controller_node SET life_id = 1 WHERE controller_id = '1'`)
	c.Assert(err, tc.ErrorIsNil)

	isController, err := s.state.IsControllerNode(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isController, tc.IsTrue)
}

func (s *stateSuite) TestAddDqliteNode(c *tc.C) {
	db := s.DB()

	controllerID0 := "0"

	_, err := db.ExecContext(c.Context(), "INSERT INTO controller_node (controller_id) VALUES ('0')")
	c.Assert(err, tc.ErrorIsNil)

	nodeID1 := uint64(15237855465837235027)
	controllerID1 := "1"
	c.Assert(s.state.AddDqliteNodeID(c.Context(), controllerID1), tc.ErrorIsNil)
	err = s.state.AddDqliteNode(c.Context(), controllerID1, nodeID1, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	nodeID2 := uint64(15237855465837235026)
	controllerID2 := "2"
	c.Assert(s.state.AddDqliteNodeID(c.Context(), controllerID2), tc.ErrorIsNil)
	err = s.state.AddDqliteNode(c.Context(), controllerID2, nodeID2, "10.0.0.2")
	c.Assert(err, tc.ErrorIsNil)

	rows, err := db.QueryContext(c.Context(), "SELECT controller_id FROM controller_node")
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		c.Check(rows.Close(), tc.ErrorIsNil)
	}()

	ids := set.NewStrings()
	for rows.Next() {
		var addr string
		err := rows.Scan(&addr)
		c.Assert(err, tc.ErrorIsNil)
		ids.Add(addr)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Assert(ids.Values(), tc.HasLen, 3)

	c.Check(ids.Contains(controllerID0), tc.IsTrue)
	c.Check(ids.Contains(controllerID1), tc.IsTrue)
	c.Check(ids.Contains(controllerID2), tc.IsTrue)
}

func (s *stateSuite) TestUpdateDqliteNode(c *tc.C) {
	// This value would cause a driver error to be emitted if we
	// tried to pass it directly as a uint64 query parameter.
	nodeID := uint64(15237855465837235027)
	controllerID := "0"
	c.Assert(s.state.AddDqliteNodeID(c.Context(), controllerID), tc.ErrorIsNil)
	err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "192.168.5.60")
	c.Assert(err, tc.ErrorIsNil)

	var (
		id   uint64
		addr string
	)
	row := s.DB().QueryRowContext(c.Context(), "SELECT dqlite_node_id, dqlite_bind_address FROM controller_node WHERE controller_id = '0'")
	err = row.Scan(&id, &addr)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(id, tc.Equals, nodeID)
	c.Check(addr, tc.Equals, "192.168.5.60")
}

func (s *stateSuite) TestAddDqliteNodeDoesNotReviveDeadNode(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "1"), tc.ErrorIsNil)
	_, err := s.DB().ExecContext(c.Context(), `
UPDATE controller_node SET life_id = 2 WHERE controller_id = '1'`)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.AddDqliteNode(c.Context(), "1", 123, "10.0.0.1")
	c.Assert(err, tc.ErrorIs, controllernodeerrors.NotFound)

	var (
		lifeID int
		nodeID sql.NullString
	)
	err = s.DB().QueryRowContext(c.Context(), `
SELECT life_id, dqlite_node_id
FROM controller_node
WHERE controller_id = '1'`).Scan(&lifeID, &nodeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 2)
	c.Check(nodeID.Valid, tc.IsFalse)
}

func (s *stateSuite) TestAddDqliteNodeDoesNotReviveDyingNode(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "1"), tc.ErrorIsNil)
	_, err := s.DB().ExecContext(c.Context(), `
UPDATE controller_node SET life_id = 1 WHERE controller_id = '1'`)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.AddDqliteNode(c.Context(), "1", 123, "10.0.0.1")
	c.Assert(err, tc.ErrorIs, controllernodeerrors.NotFound)

	var lifeID int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT life_id FROM controller_node WHERE controller_id = '1'`).Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

// TestSelectDatabaseNamespace is testing success for existing namespaces and
// a not found error for namespaces that don't exist.
func (s *stateSuite) TestSelectDatabaseNamespace(c *tc.C) {
	db := s.DB()
	_, err := db.ExecContext(c.Context(), "INSERT INTO namespace_list (namespace) VALUES ('simon!!')")
	c.Assert(err, tc.ErrorIsNil)

	st := s.state
	namespace, err := st.SelectDatabaseNamespace(c.Context(), "simon!!")
	c.Check(err, tc.ErrorIsNil)
	c.Check(namespace, tc.Equals, "simon!!")

	namespace, err = st.SelectDatabaseNamespace(c.Context(), "SIMon!!")
	c.Check(err, tc.ErrorIs, controllernodeerrors.NotFound)
	c.Check(namespace, tc.Equals, "")
}

func (s *stateSuite) TestSetRunningAgentBinaryVersionSuccess(c *tc.C) {
	ver := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   corearch.ARM64,
	}

	nodeID := uint64(15237855465837235027)
	controllerID := "1"
	err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	// Tests insert running agent binary version.
	err = s.state.SetRunningAgentBinaryVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIsNil)

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
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {

		return tx.QueryRowContext(ctx, selectAgentVerQuery, controllerID).Scan(
			&obtainedControllerID,
			&obtainedVersion,
			&obtainedArchName,
		)
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedControllerID, tc.Equals, controllerID)
	c.Check(obtainedVersion, tc.Equals, ver.Number.String())
	c.Check(obtainedArchName, tc.Equals, ver.Arch)

	// Tests update running agent binary version.
	updatedVer := coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.AMD64,
	}
	err = s.state.SetRunningAgentBinaryVersion(
		c.Context(),
		controllerID,
		updatedVer,
	)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, selectAgentVerQuery, controllerID).Scan(
			&obtainedControllerID,
			&obtainedVersion,
			&obtainedArchName,
		)
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedControllerID, tc.Equals, controllerID)
	c.Check(obtainedVersion, tc.Equals, updatedVer.Number.String())
	// A controller's node cannot change its architecture so the architecture
	// value still refers to the one during insertion.
	c.Check(obtainedArchName, tc.Equals, ver.Arch)
}

func (s *stateSuite) TestSetRunningAgentBinaryVersionControllerNodeNotFound(c *tc.C) {
	ver := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   corearch.ARM64,
	}

	nodeID := uint64(15237855465837235027)
	controllerID := "1"
	err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRunningAgentBinaryVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestSetRunningAgentBinaryVersionArchNotSupported(c *tc.C) {
	ver := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   corearch.UnsupportedArches[0],
	}

	nodeID := uint64(15237855465837235027)
	controllerID := "1"
	err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRunningAgentBinaryVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *stateSuite) TestGetControllerIDs(c *tc.C) {
	for i := range 3 {
		controllerID := strconv.Itoa(i)
		nodeID := uint64(1523785546583723502 + i)

		err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0."+controllerID)
		c.Assert(err, tc.ErrorIsNil)
	}

	controllerIDs, err := s.state.GetControllerIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerIDs, tc.HasLen, 3)
	c.Check(controllerIDs, tc.DeepEquals, []string{"0", "1", "2"})
}

func (s *stateSuite) TestGetControllerIDsEmpty(c *tc.C) {
	controllerIDs, err := s.state.GetControllerIDs(c.Context())
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyControllerIDs)
	c.Check(controllerIDs, tc.HasLen, 0)
}
