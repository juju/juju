// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
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

func (s *stateSuite) TestAddDqliteNode(c *tc.C) {
	db := s.DB()

	// This
	controllerID0 := "0"

	_, err := db.ExecContext(c.Context(), "INSERT INTO controller_node (controller_id) VALUES ('0')")
	c.Assert(err, tc.ErrorIsNil)

	nodeID1 := uint64(15237855465837235027)
	controllerID1 := "1"
	err = s.state.AddDqliteNode(c.Context(), controllerID1, nodeID1, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	nodeID2 := uint64(15237855465837235026)
	controllerID2 := "2"
	err = s.state.AddDqliteNode(c.Context(), controllerID2, nodeID2, "10.0.0.2")
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
	c.Check(obtainedArchName, tc.Equals, updatedVer.Arch)
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

func (s *stateSuite) TestSetAPIAddressesToAddOnly(c *tc.C) {
	nodeID := uint64(15237855465837235027)
	controllerID := "1"
	err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "192.168.0.1:17070", IsAgent: false, Scope: network.ScopeMachineLocal},
	}

	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID: addrs,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.checkControllerAPIAddress(c, controllerID, addrs)
}

func (s *stateSuite) TestSetAPIAddressesToDeleteOnly(c *tc.C) {
	nodeID := uint64(15237855465837235027)
	controllerID := "1"
	err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	// Insert 3 addresses.
	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.2:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.3:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
	}
	s.addControllerAPIAddresses(c, controllerID, addrs)

	// Set API addresses that delete two nodes.
	newAddrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
	}
	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID: newAddrs,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.checkControllerAPIAddress(c, controllerID, newAddrs)
}

func (s *stateSuite) TestSetAPIAddressesAddsDeletes(c *tc.C) {
	nodeID := uint64(15237855465837235027)
	controllerID := "1"
	err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	// Insert 3 addresses.
	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.2:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.3:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
	}
	s.addControllerAPIAddresses(c, controllerID, addrs)

	// Set API addresses that delete two nodes and insert one new.
	newAddrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "192.168.0.1:17070", IsAgent: false, Scope: network.ScopeMachineLocal},
	}
	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID: newAddrs,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.checkControllerAPIAddress(c, controllerID, newAddrs)
}

func (s *stateSuite) TestSetAPIAddressesNoDelta(c *tc.C) {
	nodeID := uint64(15237855465837235027)
	controllerID := "1"
	err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	// Insert 3 addresses.
	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.2:17070", IsAgent: true, Scope: network.ScopeMachineLocal},
		{Address: "10.0.0.3:17070", IsAgent: true, Scope: network.ScopeMachineLocal},
	}
	s.addControllerAPIAddresses(c, controllerID, addrs)

	// Set API but with the same addresses already present in the db.
	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID: addrs,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.checkControllerAPIAddress(c, controllerID, addrs)
}

func (s *stateSuite) TestDeltaAddressesEmpty(c *tc.C) {
	existing := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}
	newAddrs := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}

	toAdd, toUpdate, toRemove := calculateAddressDeltas(existing, newAddrs)
	c.Check(toAdd, tc.IsNil)
	c.Check(toRemove, tc.IsNil)
	c.Check(toUpdate, tc.IsNil)
}

func (s *stateSuite) TestDeltaAddressesAddOnly(c *tc.C) {
	existing := []controllerAPIAddress{}
	newAddrs := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}

	toAdd, toUpdate, toRemove := calculateAddressDeltas(existing, newAddrs)
	c.Check(toAdd, tc.SameContents, newAddrs)
	c.Check(toRemove, tc.IsNil)
	c.Check(toUpdate, tc.IsNil)
}

func (s *stateSuite) TestDeltaAddressesRemoveOnly(c *tc.C) {
	existing := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}
	newAddrs := []controllerAPIAddress{}

	toAdd, toUpdate, toRemove := calculateAddressDeltas(existing, newAddrs)
	c.Check(toAdd, tc.IsNil)
	c.Check(toRemove, tc.SameContents, existing)
	c.Check(toUpdate, tc.IsNil)
}

func (s *stateSuite) TestDeltaAddressesUpdateOnly(c *tc.C) {
	existing := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
	}
	newAddrs := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: false},
		{Address: "10.0.0.2:17070", IsAgent: false},
		{Address: "10.0.0.3:17070", IsAgent: false},
	}

	toAdd, toUpdate, toRemove := calculateAddressDeltas(existing, newAddrs)
	c.Check(toAdd, tc.IsNil)
	c.Check(toRemove, tc.IsNil)
	c.Check(toUpdate, tc.SameContents, newAddrs)
}

func (s *stateSuite) TestDeltaAddressesAllChanges(c *tc.C) {
	existing := []controllerAPIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.2:17070", IsAgent: true},
		{Address: "10.0.0.3:17070", IsAgent: true},
		{Address: "10.0.0.4:17070", IsAgent: true},
	}
	newAddrs := []controllerAPIAddress{
		// To add.
		{Address: "10.0.0.5:17070", IsAgent: true},
		{Address: "10.0.0.6:17070", IsAgent: false},
		// To update.
		{Address: "10.0.0.3:17070", IsAgent: false},
		{Address: "10.0.0.4:17070", IsAgent: false},
		// 10.0.0.1 and 10.0.0.2 will be removed.
	}

	toAdd, toUpdate, toRemove := calculateAddressDeltas(existing, newAddrs)
	c.Check(toAdd, tc.SameContents, newAddrs[0:2])
	c.Check(toUpdate, tc.SameContents, newAddrs[2:4])
	c.Check(toRemove, tc.SameContents, existing[:2])
}

func (s *stateSuite) TestSetAPIAddressControllerNodeExists(c *tc.C) {
	nodeID := uint64(15237855465837235027)
	controllerID := "1"
	err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0.1")
	c.Assert(err, tc.ErrorIsNil)

	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "192.168.0.1:17070", IsAgent: false},
	}

	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID: addrs,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.checkControllerAPIAddress(c, controllerID, addrs)

	agentAddresses, err := s.state.GetAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agentAddresses, tc.DeepEquals, map[string]controllernode.APIAddresses{
		"1": {{
			Address: "10.0.0.1:17070",
			IsAgent: true,
		}},
	})

	// Update api address.
	newAddrs := []controllernode.APIAddress{
		{Address: "10.0.255.255:17070", IsAgent: true},
		{Address: "192.168.255.255:17070", IsAgent: false},
	}

	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID: newAddrs,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.checkControllerAPIAddress(c, controllerID, newAddrs)

	agentAddresses, err = s.state.GetAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agentAddresses, tc.DeepEquals, map[string]controllernode.APIAddresses{
		"1": {{
			Address: "10.0.255.255:17070",
			IsAgent: true,
		}},
	})
}

func (s *stateSuite) TestGetAllAPIAddressesForAgent(c *tc.C) {
	var controllerIDs []string
	for i := 1; i < 5; i++ {
		controllerID := strconv.Itoa(i)
		nodeID := uint64(1523785546583723502 + i)

		err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0."+controllerID)
		c.Assert(err, tc.ErrorIsNil)

		controllerIDs = append(controllerIDs, controllerID)
	}

	for i, controllerID := range controllerIDs {
		addrs := []controllernode.APIAddress{
			{Address: fmt.Sprintf("10.0.0.%d:17070", i), IsAgent: true},
			{Address: fmt.Sprintf("192.168.0.%d:17070", i), IsAgent: false},
		}

		err := s.state.SetAPIAddresses(
			c.Context(),
			map[string]controllernode.APIAddresses{
				controllerID: addrs,
			},
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	agentAddresses, err := s.state.GetAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agentAddresses, tc.DeepEquals, map[string]controllernode.APIAddresses{
		"1": {{
			Address: "10.0.0.0:17070",
			IsAgent: true,
		}},
		"2": {{
			Address: "10.0.0.1:17070",
			IsAgent: true,
		}},
		"3": {{
			Address: "10.0.0.2:17070",
			IsAgent: true,
		}},
		"4": {{
			Address: "10.0.0.3:17070",
			IsAgent: true,
		}},
	})
}

func (s *stateSuite) TestGetAllAPIAddressesForAgentEmptyAddress(c *tc.C) {
	// If we set an empty address then it should not be included in the
	// GetAPIAddressesByControllerIDForAgents result.

	var controllerIDs []string
	for i := 1; i < 5; i++ {
		controllerID := strconv.Itoa(i)
		nodeID := uint64(1523785546583723502 + i)

		err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0."+controllerID)
		c.Assert(err, tc.ErrorIsNil)

		controllerIDs = append(controllerIDs, controllerID)
	}

	for i, controllerID := range controllerIDs {
		var addrs []controllernode.APIAddress
		if i+1 == 2 {
			addrs = []controllernode.APIAddress{
				{Address: "", IsAgent: true},
			}
		} else {
			addrs = []controllernode.APIAddress{
				{Address: fmt.Sprintf("10.0.0.%d:17070", i), IsAgent: true},
			}
		}

		err := s.state.SetAPIAddresses(
			c.Context(),
			map[string]controllernode.APIAddresses{
				controllerID: addrs,
			},
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	agentAddresses, err := s.state.GetAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agentAddresses, tc.DeepEquals, map[string]controllernode.APIAddresses{
		"1": {{
			Address: "10.0.0.0:17070",
			IsAgent: true,
		}},
		"3": {{
			Address: "10.0.0.2:17070",
			IsAgent: true,
		}},
		"4": {{
			Address: "10.0.0.3:17070",
			IsAgent: true,
		}},
	})
}

func (s *stateSuite) TestSetAPIAddressControllerNodeNotFound(c *tc.C) {
	err := s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			"unknown-controller-id": []controllernode.APIAddress{},
		},
	)
	c.Assert(err, tc.ErrorMatches, "controller nodes .* do not exist")
}

<<<<<<< HEAD
=======
func (s *stateSuite) TestGetAPIAddresses(c *tc.C) {
	var controllerIDs []string
	for i := 0; i < 3; i++ {
		controllerID := strconv.Itoa(i)
		nodeID := uint64(1523785546583723502 + i)

		err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0."+controllerID)
		c.Assert(err, tc.ErrorIsNil)

		controllerIDs = append(controllerIDs, controllerID)
	}

	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "192.168.0.1:17070", IsAgent: false},
	}

	err := s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			"0": addrs,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	expectedAddresses := []string{
		"10.0.0.1:17070",
		"192.168.0.1:17070",
	}
	resultAddresses, err := s.state.GetAPIAddresses(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultAddresses, tc.DeepEquals, expectedAddresses)
}

func (s *stateSuite) TestGetAPIAddressesEmpty(c *tc.C) {
	for i := 0; i < 3; i++ {
		controllerID := strconv.Itoa(i)
		nodeID := uint64(1523785546583723502 + i)

		err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0."+controllerID)
		c.Assert(err, tc.ErrorIsNil)
	}

	_, err := s.state.GetAPIAddresses(c.Context(), "42")
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)
}

func (s *stateSuite) TestGetAPIAddressesForAgents(c *tc.C) {
	for i := 0; i < 3; i++ {
		controllerID := strconv.Itoa(i)
		nodeID := uint64(1523785546583723502 + i)

		err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0."+controllerID)
		c.Assert(err, tc.ErrorIsNil)
	}

	addrs := []controllernode.APIAddress{
		{Address: "10.0.0.1:17070", IsAgent: true, Scope: network.ScopeMachineLocal},
		{Address: "10.0.0.2:17070", IsAgent: true, Scope: network.ScopeMachineLocal},
		{Address: "192.168.0.1:17070", IsAgent: false, Scope: network.ScopeMachineLocal},
	}

	err := s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			"0": addrs,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	expectedAddresses := []string{
		"10.0.0.1:17070",
		"10.0.0.2:17070",
	}
	resultAddresses, err := s.state.GetAPIAddressesForAgents(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultAddresses, tc.DeepEquals, expectedAddresses)
}

func (s *stateSuite) TestGetAPIAddressesForAgentsEmpty(c *tc.C) {
	for i := 0; i < 3; i++ {
		controllerID := strconv.Itoa(i)
		nodeID := uint64(1523785546583723502 + i)

		err := s.state.AddDqliteNode(c.Context(), controllerID, nodeID, "10.0.0."+controllerID)
		c.Assert(err, tc.ErrorIsNil)
	}

	_, err := s.state.GetAPIAddressesForAgents(c.Context(), "42")
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)
}

>>>>>>> 61370002eb (feat: remove curated nodes)
func (s *stateSuite) TestGetControllerIDs(c *tc.C) {
	for i := 0; i < 3; i++ {
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

func (s *stateSuite) TestGetAPIAddressesForAgents(c *tc.C) {
	// Arrange: 2 controller nodes
	controllerID1 := "1"
	nodeID1 := uint64(15237855465837235027)
	err := s.state.AddDqliteNode(c.Context(), controllerID1, nodeID1, "10.0.0."+controllerID1)
	c.Assert(err, tc.ErrorIsNil)

	addrs1 := []controllernode.APIAddress{
		{Address: "10.0.0.2:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.42:18080", IsAgent: true, Scope: network.ScopePublic},
		{Address: "192.168.0.1:17070", IsAgent: false, Scope: network.ScopeMachineLocal},
	}
	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID1: addrs1,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	controllerID2 := "2"
	nodeID2 := uint64(15237855465837235026)
	err = s.state.AddDqliteNode(c.Context(), controllerID2, nodeID2, "10.0.0."+controllerID2)
	c.Assert(err, tc.ErrorIsNil)

	addrs2 := []controllernode.APIAddress{
		{Address: "192.168.10.1:17070", IsAgent: false, Scope: network.ScopeMachineLocal},
		{Address: "10.0.34.2:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.3:18080", IsAgent: true, Scope: network.ScopePublic},
	}

	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID2: addrs2,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	result, err := s.state.GetAPIAddressesForAgents(c.Context())

	// Assert: validate order of slice and addresses are only IsAgent true
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 2)
	// The order of the addresses coming from the db cannot be guaranteed.
	// That's okay in this case as the caller will order the addresses as
	// required.
	c.Assert(result[ctrlID1], tc.SameContents, controllernode.APIAddresses(addrs1[:2]))
	c.Assert(result[ctrlID2], tc.SameContents, controllernode.APIAddresses(addrs2[1:]))
}

func (s *stateSuite) TestGetAllAPIAddressesWithScopeForClients(c *tc.C) {
	// Arrange
	// Arrange: 2 controller nodes
	controllerID1 := "1"
	nodeID1 := uint64(15237855465837235027)
	err := s.state.AddDqliteNode(c.Context(), controllerID1, nodeID1, "10.0.0."+controllerID1)
	c.Assert(err, tc.ErrorIsNil)

	addrs1 := []controllernode.APIAddress{
		{Address: "10.0.0.2:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.42:18080", IsAgent: true, Scope: network.ScopePublic},
		{Address: "192.168.0.1:17070", IsAgent: false, Scope: network.ScopeMachineLocal},
	}
	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID1: addrs1,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	controllerID2 := "2"
	nodeID2 := uint64(15237855465837235026)
	err = s.state.AddDqliteNode(c.Context(), controllerID2, nodeID2, "10.0.0."+controllerID2)
	c.Assert(err, tc.ErrorIsNil)

	addrs2 := []controllernode.APIAddress{
		{Address: "192.168.10.1:17070", IsAgent: false, Scope: network.ScopeMachineLocal},
		{Address: "10.0.34.2:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.3:18080", IsAgent: true, Scope: network.ScopePublic},
	}
	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID2: addrs2,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	result, err := s.state.GetAPIAddressesForClients(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 2)
	// The order of the addresses coming from the db cannot be guaranteed.
	// That's okay in this case as the caller will order the addresses as
	// required.
	c.Assert(result[controllerID1], tc.SameContents, controllernode.APIAddresses(addrs1))
	c.Assert(result[controllerID2], tc.SameContents, controllernode.APIAddresses(addrs2))
}

func (s *stateSuite) TestGetAllCloudLocalAPIAddresses(c *tc.C) {
	// Arrange
	controllerID1 := "1"
	nodeID1 := uint64(15237855465837235027)
	err := s.state.AddDqliteNode(c.Context(), controllerID1, nodeID1, "10.0.0."+controllerID1)
	c.Assert(err, tc.ErrorIsNil)

	addrs := controllernode.APIAddresses{
		{Address: "10.0.0.1:17070", IsAgent: true, Scope: network.ScopeCloudLocal},
		{Address: "10.0.0.42:18080", IsAgent: true, Scope: network.ScopePublic},
		{Address: "192.168.0.1:17070", IsAgent: false, Scope: network.ScopeMachineLocal},
	}

	err = s.state.SetAPIAddresses(
		c.Context(),
		map[string]controllernode.APIAddresses{
			controllerID1: addrs,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	result, err := s.state.GetAllCloudLocalAPIAddresses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Assert(result, tc.SameContents, []string{"10.0.0.1:17070"})
}

func (s *stateSuite) checkControllerAPIAddress(c *tc.C, controllerID string, addrs []controllernode.APIAddress) {
	var (
		resultAddresses, resultScopes []string
		isAgent                       []bool
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT address, is_agent, scope FROM controller_api_address WHERE controller_id = ?", controllerID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var (
				addressVal, scopeVal string
				isAgentVal           bool
			)
			if err := rows.Scan(&addressVal, &isAgentVal, &scopeVal); err != nil {
				return err
			}
			resultAddresses = append(resultAddresses, addressVal)
			isAgent = append(isAgent, isAgentVal)
			resultScopes = append(resultScopes, scopeVal)
		}
		return rows.Err()
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultAddresses, tc.HasLen, len(addrs))
	for i, addr := range addrs {
		c.Check(resultAddresses[i], tc.Equals, addr.Address)
		c.Check(isAgent[i], tc.Equals, addr.IsAgent)
		c.Check(resultScopes[i], tc.Equals, addr.Scope.String())
	}
}

func (s *stateSuite) addControllerAPIAddresses(c *tc.C, controllerID string, addrs []controllernode.APIAddress) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO controller_api_address (controller_id, address, is_agent, scope) VALUES (?, ?, ?, ?)"
		for _, addr := range addrs {
			_, err := tx.ExecContext(ctx, stmt, controllerID, addr.Address, addr.IsAgent, addr.Scope)
			if err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}
