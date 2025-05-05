// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/controllernode"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
	state *State
}

var _ = tc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory())
}

func (s *stateSuite) TestCurateNodes(c *tc.C) {
	db := s.DB()

	_, err := db.ExecContext(c.Context(), "INSERT INTO controller_node (controller_id) VALUES ('1')")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.CurateNodes(
		c.Context(), []string{"2", "3"}, []string{"1"})
	c.Assert(err, tc.ErrorIsNil)

	rows, err := db.QueryContext(c.Context(), "SELECT controller_id FROM controller_node")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	ids := set.NewStrings()
	for rows.Next() {
		var addr string
		err := rows.Scan(&addr)
		c.Assert(err, tc.ErrorIsNil)
		ids.Add(addr)
	}
	c.Check(ids.Values(), tc.HasLen, 3)

	// Controller "0" is inserted as part of the bootstrapped schema.
	c.Check(ids.Contains("0"), tc.IsTrue)
	c.Check(ids.Contains("2"), tc.IsTrue)
	c.Check(ids.Contains("3"), tc.IsTrue)
}

func (s *stateSuite) TestUpdateDqliteNode(c *tc.C) {
	// This value would cause a driver error to be emitted if we
	// tried to pass it directly as a uint64 query parameter.
	nodeID := uint64(15237855465837235027)

	err := s.state.UpdateDqliteNode(
		c.Context(), "0", nodeID, "192.168.5.60")
	c.Assert(err, tc.ErrorIsNil)

	row := s.DB().QueryRowContext(c.Context(), "SELECT dqlite_node_id, dqlite_bind_address FROM controller_node WHERE controller_id = '0'")
	c.Assert(row.Err(), tc.ErrorIsNil)

	var (
		id   uint64
		addr string
	)
	err = row.Scan(&id, &addr)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(id, tc.Equals, nodeID)
	c.Check(addr, tc.Equals, "192.168.5.60")
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
	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   corearch.ARM64,
	}

	err := s.state.CurateNodes(c.Context(), []string{controllerID}, nil)
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
	c.Check(obtainedArchName, tc.Equals, updatedVer.Arch)
}

func (s *stateSuite) TestSetRunningAgentBinaryVersionControllerNodeNotFound(c *tc.C) {
	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   corearch.ARM64,
	}

	err := s.state.CurateNodes(c.Context(), []string{controllerID}, nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRunningAgentBinaryVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestSetRunningAgentBinaryVersionArchNotSupported(c *tc.C) {
	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   corearch.UnsupportedArches[0],
	}

	err := s.state.CurateNodes(c.Context(), []string{controllerID}, nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRunningAgentBinaryVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *stateSuite) TestIsControllerNode(c *tc.C) {
	controllerID := "1"

	err := s.state.CurateNodes(c.Context(), []string{controllerID}, nil)
	c.Assert(err, tc.ErrorIsNil)

	isControllerNode, err := s.state.IsControllerNode(c.Context(), controllerID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isControllerNode, tc.Equals, true)

	isControllerNode, err = s.state.IsControllerNode(c.Context(), "99")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isControllerNode, tc.Equals, false)
}

func (s *stateSuite) TestSetAPIAddressesNew(c *gc.C) {
	controllerID := "1"

	err := s.state.CurateNodes(context.Background(), []string{controllerID}, nil)
	c.Assert(err, jc.ErrorIsNil)

	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "192.168.0.1:17070", IsAgent: false},
	}

	err = s.state.SetAPIAddresses(
		context.Background(),
		controllerID,
		addrs,
	)
	c.Assert(err, jc.ErrorIsNil)

	var (
		resultAddresses []string
		isAgent         []bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT address, is_agent FROM controller_api_address WHERE controller_id = ?", controllerID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var (
				addressVal string
				isAgentVal bool
			)
			if err := rows.Scan(&addressVal, &isAgentVal); err != nil {
				return err
			}
			resultAddresses = append(resultAddresses, addressVal)
			isAgent = append(isAgent, isAgentVal)
		}
		return rows.Err()
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultAddresses, gc.HasLen, 2)
	c.Check(resultAddresses[0], gc.Equals, addrs[0].Address)
	c.Check(isAgent[0], gc.Equals, addrs[0].IsAgent)
	c.Check(resultAddresses[1], gc.Equals, addrs[1].Address)
	c.Check(isAgent[1], gc.Equals, addrs[1].IsAgent)
}

func (s *stateSuite) TestSetAPIAddressControllerNodeExists(c *gc.C) {
	controllerID := "1"

	err := s.state.CurateNodes(context.Background(), []string{controllerID}, nil)
	c.Assert(err, jc.ErrorIsNil)

	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "192.168.0.1:17070", IsAgent: false},
	}

	err = s.state.SetAPIAddresses(
		context.Background(),
		controllerID,
		addrs,
	)
	c.Assert(err, jc.ErrorIsNil)

	var (
		resultAddresses []string
		isAgent         []bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT address, is_agent FROM controller_api_address WHERE controller_id = ?", controllerID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var (
				addressVal string
				isAgentVal bool
			)
			if err := rows.Scan(&addressVal, &isAgentVal); err != nil {
				return err
			}
			resultAddresses = append(resultAddresses, addressVal)
			isAgent = append(isAgent, isAgentVal)
		}
		return rows.Err()
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultAddresses, gc.HasLen, 2)
	c.Check(resultAddresses[0], gc.Equals, addrs[0].Address)
	c.Check(isAgent[0], gc.Equals, addrs[0].IsAgent)
	c.Check(resultAddresses[1], gc.Equals, addrs[1].Address)
	c.Check(isAgent[1], gc.Equals, addrs[1].IsAgent)

	// Update api address.
	newAddrs := []controllernode.APIAddress{
		{Address: "10.0.255.255:17070", IsAgent: true},
		{Address: "192.168.255.255:17070", IsAgent: false},
	}

	err = s.state.SetAPIAddresses(
		context.Background(),
		controllerID,
		newAddrs,
	)
	c.Assert(err, jc.ErrorIsNil)

	var (
		updatedResultAddresses []string
		updatedIsAgent         []bool
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT address, is_agent FROM controller_api_address WHERE controller_id = ?", controllerID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var (
				addressVal string
				isAgentVal bool
			)
			if err := rows.Scan(&addressVal, &isAgentVal); err != nil {
				return err
			}
			updatedResultAddresses = append(updatedResultAddresses, addressVal)
			updatedIsAgent = append(updatedIsAgent, isAgentVal)
		}
		return rows.Err()
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(updatedResultAddresses, gc.HasLen, 2)
	c.Check(updatedResultAddresses[0], gc.Equals, newAddrs[0].Address)
	c.Check(updatedIsAgent[0], gc.Equals, newAddrs[0].IsAgent)
	c.Check(updatedResultAddresses[1], gc.Equals, newAddrs[1].Address)
	c.Check(updatedIsAgent[1], gc.Equals, newAddrs[1].IsAgent)
}

func (s *stateSuite) TestSetAPIAddressControllerNodeNotFound(c *gc.C) {
	err := s.state.SetAPIAddresses(
		context.Background(),
		"unknown-controller-id",
		[]controllernode.APIAddress{},
	)
	c.Assert(err, gc.ErrorMatches, "controller node .* does not exist")
}

func (s *stateSuite) TestGetControllerIDs(c *gc.C) {
	err := s.state.CurateNodes(context.Background(), []string{"1", "2"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	controllerIDs, err := s.state.GetControllerIDs(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerIDs, gc.HasLen, 3)
	c.Check(controllerIDs, gc.DeepEquals, []string{"0", "1", "2"})
}
