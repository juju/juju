// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/controllernode"
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

func (s *stateSuite) TestCurateNodes(c *tc.C) {
	db := s.DB()

	_, err := db.ExecContext(c.Context(), "INSERT INTO controller_node (controller_id) VALUES ('0')")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.CurateNodes(
		c.Context(), []string{"1", "2"}, []string{"0"})
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
	c.Check(ids.Values(), tc.HasLen, 2)

	c.Check(ids.Contains("1"), tc.IsTrue)
	c.Check(ids.Contains("2"), tc.IsTrue)
}

func (s *stateSuite) TestUpdateDqliteNode(c *tc.C) {
	// This value would cause a driver error to be emitted if we
	// tried to pass it directly as a uint64 query parameter.
	nodeID := uint64(15237855465837235027)
	controllerID := "0"
	err := s.state.CurateNodes(
		c.Context(), []string{controllerID}, nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.UpdateDqliteNode(
		c.Context(), controllerID, nodeID, "192.168.5.60")
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

func (s *stateSuite) TestSetAPIAddressesToAddOnly(c *tc.C) {
	controllerID := "1"

	err := s.state.CurateNodes(c.Context(), []string{controllerID}, nil)
	c.Assert(err, tc.ErrorIsNil)

	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "192.168.0.1:17070", IsAgent: false},
	}

	err = s.state.SetAPIAddresses(
		c.Context(),
		controllerID,
		addrs,
	)
	c.Assert(err, tc.ErrorIsNil)

	var (
		resultAddresses []string
		isAgent         []bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultAddresses, tc.HasLen, 2)
	c.Check(resultAddresses[0], tc.Equals, addrs[0].Address)
	c.Check(isAgent[0], tc.Equals, addrs[0].IsAgent)
	c.Check(resultAddresses[1], tc.Equals, addrs[1].Address)
	c.Check(isAgent[1], tc.Equals, addrs[1].IsAgent)
}

func (s *stateSuite) TestSetAPIAddressesToDeleteOnly(c *tc.C) {
	controllerID := "1"

	err := s.state.CurateNodes(c.Context(), []string{controllerID}, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert 3 addresses.
	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO controller_api_address (controller_id, address, is_agent) VALUES (?, ?, ?)"
		for _, addr := range addrs {
			_, err := tx.ExecContext(ctx, stmt, controllerID, addr.Address, addr.IsAgent)
			if err != nil {
				return err
			}
		}
		return err
	})

	// Set API addresses that delete two nodes.
	newAddrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
	}
	err = s.state.SetAPIAddresses(
		c.Context(),
		controllerID,
		newAddrs,
	)
	c.Assert(err, tc.ErrorIsNil)

	var (
		resultAddresses []string
		isAgent         []bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultAddresses, tc.HasLen, 1)
	c.Check(resultAddresses[0], tc.Equals, newAddrs[0].Address)
	c.Check(isAgent[0], tc.Equals, newAddrs[0].IsAgent)
}

func (s *stateSuite) TestSetAPIAddressesAddsDeletes(c *tc.C) {
	controllerID := "1"

	err := s.state.CurateNodes(c.Context(), []string{controllerID}, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert 3 addresses.
	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO controller_api_address (controller_id, address, is_agent) VALUES (?, ?, ?)"
		for _, addr := range addrs {
			_, err := tx.ExecContext(ctx, stmt, controllerID, addr.Address, addr.IsAgent)
			if err != nil {
				return err
			}
		}
		return err
	})

	// Set API addresses that delete two nodes and insert one new.
	newAddrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "192.168.0.1:17070", IsAgent: false},
	}
	err = s.state.SetAPIAddresses(
		c.Context(),
		controllerID,
		newAddrs,
	)
	c.Assert(err, tc.ErrorIsNil)

	var (
		resultAddresses []string
		isAgent         []bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultAddresses, tc.HasLen, 2)
	c.Check(resultAddresses[0], tc.Equals, newAddrs[0].Address)
	c.Check(isAgent[0], tc.Equals, newAddrs[0].IsAgent)
	c.Check(resultAddresses[1], tc.Equals, newAddrs[1].Address)
	c.Check(isAgent[1], tc.Equals, newAddrs[1].IsAgent)
}

func (s *stateSuite) TestSetAPIAddressesNoDelta(c *tc.C) {
	controllerID := "1"

	err := s.state.CurateNodes(c.Context(), []string{controllerID}, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert 3 addresses.
	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO controller_api_address (controller_id, address, is_agent) VALUES (?, ?, ?)"
		for _, addr := range addrs {
			_, err := tx.ExecContext(ctx, stmt, controllerID, addr.Address, addr.IsAgent)
			if err != nil {
				return err
			}
		}
		return err
	})

	// Set API but with the same addresses already present in the db.
	err = s.state.SetAPIAddresses(
		c.Context(),
		controllerID,
		addrs,
	)
	c.Assert(err, tc.ErrorIsNil)

	var (
		resultAddresses []string
		isAgent         []bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultAddresses, tc.HasLen, 3)
	c.Check(resultAddresses[0], tc.Equals, addrs[0].Address)
	c.Check(isAgent[0], tc.Equals, addrs[0].IsAgent)
	c.Check(resultAddresses[1], tc.Equals, addrs[1].Address)
	c.Check(isAgent[1], tc.Equals, addrs[1].IsAgent)
	c.Check(resultAddresses[2], tc.Equals, addrs[2].Address)
	c.Check(isAgent[2], tc.Equals, addrs[2].IsAgent)
}

func (s *stateSuite) TestDeltaAddressesEmpty(c *tc.C) {
	existing := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}
	new := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}

	toAdd, toUpdate, toRemove := calculateAddressDeltas(existing, new)
	c.Check(toAdd, tc.IsNil)
	c.Check(toRemove, tc.IsNil)
	c.Check(toUpdate, tc.IsNil)
}

func (s *stateSuite) TestDeltaAddressesAddOnly(c *tc.C) {
	existing := []controllerAPIAddress{}
	new := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}

	toAdd, toUpdate, toRemove := calculateAddressDeltas(existing, new)
	c.Check(toAdd, tc.SameContents, new)
	c.Check(toRemove, tc.IsNil)
	c.Check(toUpdate, tc.IsNil)
}

func (s *stateSuite) TestDeltaAddressesRemoveOnly(c *tc.C) {
	existing := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}
	new := []controllerAPIAddress{}
	expected := []string{
		"10.0.0.1:17070",
		"10.0.0.2:17070",
		"10.0.0.3:17070",
	}

	toAdd, toUpdate, toRemove := calculateAddressDeltas(existing, new)
	c.Check(toAdd, tc.IsNil)
	c.Check(toRemove, tc.SameContents, expected)
	c.Check(toUpdate, tc.IsNil)
}

func (s *stateSuite) TestDeltaAddressesUpdateOnly(c *tc.C) {
	existing := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}
	new := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: false},
		{Address: "10.0.0.2:17070", IsAgent: false},
		{Address: "10.0.0.3:17070", IsAgent: false},
	}

	toAdd, toUpdate, toRemove := calculateAddressDeltas(existing, new)
	c.Check(toAdd, tc.IsNil)
	c.Check(toRemove, tc.IsNil)
	c.Check(toUpdate, tc.SameContents, new)
}

func (s *stateSuite) TestDeltaAddressesAllChanges(c *tc.C) {
	existing := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
		{Address: "10.0.0.4:17070", IsAgent: true},
	}
	new := []controllerAPIAddress{
		// To add.
		{Address: "10.0.0.5:17070", IsAgent: true},
		{Address: "10.0.0.6:17070", IsAgent: false},
		// To update.
		{Address: "10.0.0.3:17070", IsAgent: false},
		{Address: "10.0.0.4:17070", IsAgent: false},
		// 10.0.0.1 and 10.0.0.2 will be removed.
	}

	toAdd, toUpdate, toRemove := calculateAddressDeltas(existing, new)
	c.Check(toAdd, tc.SameContents, new[0:2])
	c.Check(toUpdate, tc.SameContents, new[2:4])
	c.Check(toRemove, tc.SameContents, []string{
		"10.0.0.1:17070",
		"10.0.0.2:17070",
	})
}

func (s *stateSuite) TestSetAPIAddressControllerNodeExists(c *tc.C) {
	controllerID := "1"

	err := s.state.CurateNodes(c.Context(), []string{controllerID}, nil)
	c.Assert(err, tc.ErrorIsNil)

	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "192.168.0.1:17070", IsAgent: false},
	}

	err = s.state.SetAPIAddresses(
		c.Context(),
		controllerID,
		addrs,
	)
	c.Assert(err, tc.ErrorIsNil)

	var (
		resultAddresses []string
		isAgent         []bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultAddresses, tc.HasLen, 2)
	c.Check(resultAddresses[0], tc.Equals, addrs[0].Address)
	c.Check(isAgent[0], tc.Equals, addrs[0].IsAgent)
	c.Check(resultAddresses[1], tc.Equals, addrs[1].Address)
	c.Check(isAgent[1], tc.Equals, addrs[1].IsAgent)

	// Update api address.
	newAddrs := []controllernode.APIAddress{
		{Address: "10.0.255.255:17070", IsAgent: true},
		{Address: "192.168.255.255:17070", IsAgent: false},
	}

	err = s.state.SetAPIAddresses(
		c.Context(),
		controllerID,
		newAddrs,
	)
	c.Assert(err, tc.ErrorIsNil)

	var (
		updatedResultAddresses []string
		updatedIsAgent         []bool
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

	c.Assert(err, tc.ErrorIsNil)
	c.Check(updatedResultAddresses, tc.HasLen, 2)
	c.Check(updatedResultAddresses[0], tc.Equals, newAddrs[0].Address)
	c.Check(updatedIsAgent[0], tc.Equals, newAddrs[0].IsAgent)
	c.Check(updatedResultAddresses[1], tc.Equals, newAddrs[1].Address)
	c.Check(updatedIsAgent[1], tc.Equals, newAddrs[1].IsAgent)
}

func (s *stateSuite) TestSetAPIAddressControllerNodeNotFound(c *tc.C) {
	err := s.state.SetAPIAddresses(
		c.Context(),
		"unknown-controller-id",
		[]controllernode.APIAddress{},
	)
	c.Assert(err, tc.ErrorMatches, "controller node .* does not exist")
}

func (s *stateSuite) TestGetAPIAddresses(c *tc.C) {
	err := s.state.CurateNodes(context.Background(), []string{"0", "1", "2"}, nil)
	c.Assert(err, tc.ErrorIsNil)

	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "192.168.0.1:17070", IsAgent: false},
	}

	err = s.state.SetAPIAddresses(
		context.Background(),
		"0",
		addrs,
	)
	c.Assert(err, tc.ErrorIsNil)

	expectedAddresses := []string{
		"10.0.0.1:17070",
		"192.168.0.1:17070",
	}
	resultAddresses, err := s.state.GetAPIAddresses(context.Background(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultAddresses, tc.DeepEquals, expectedAddresses)
}

func (s *stateSuite) TestGetAPIAddressesEmpty(c *tc.C) {
	err := s.state.CurateNodes(context.Background(), []string{"0", "1", "2"}, nil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetAPIAddresses(context.Background(), "42")
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)
}

func (s *stateSuite) TestGetAPIAddressesForAgents(c *tc.C) {
	err := s.state.CurateNodes(context.Background(), []string{"0", "1", "2"}, nil)
	c.Assert(err, tc.ErrorIsNil)

	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "192.168.0.1:17070", IsAgent: false},
	}

	err = s.state.SetAPIAddresses(
		context.Background(),
		"0",
		addrs,
	)
	c.Assert(err, tc.ErrorIsNil)

	expectedAddresses := []string{
		"10.0.0.1:17070",
		"10.0.0.2:17070",
	}
	resultAddresses, err := s.state.GetAPIAddressesForAgents(context.Background(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultAddresses, tc.DeepEquals, expectedAddresses)
}

func (s *stateSuite) TestGetAPIAddressesForAgentsEmpty(c *tc.C) {
	err := s.state.CurateNodes(context.Background(), []string{"0", "1", "2"}, nil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetAPIAddressesForAgents(context.Background(), "42")
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)
}

func (s *stateSuite) TestGetControllerIDs(c *tc.C) {
	err := s.state.CurateNodes(c.Context(), []string{"0", "1", "2"}, nil)
	c.Assert(err, tc.ErrorIsNil)

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
