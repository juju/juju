package state

import (
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/uuid"
)

type spaceMachineSuite struct {
	linkLayerBaseSuite
}

func TestSpaceMachineSuite(t *testing.T) {
	tc.Run(t, &spaceMachineSuite{})
}

// TestGetMachinesBoundToSpacesEmptyList tests that GetMachinesBoundToSpaces
// returns nil when given an empty list of space UUIDs.
func (s *spaceMachineSuite) TestGetMachinesBoundToSpacesEmptyList(c *tc.C) {

	// Act: Call GetMachinesBoundToSpaces with an empty list of space UUIDs
	machines, err := s.state.GetMachinesBoundToSpaces(c.Context(), []string{})

	// Assert: No error nor machines
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.HasLen, 0)
}

// TestGetMachinesBoundToSpacesNoMachines tests that GetMachinesBoundToSpaces
// returns an empty slice when no machines are bound to the specified spaces.
func (s *spaceMachineSuite) TestGetMachinesBoundToSpacesNoMachines(c *tc.C) {
	// Arrange: add a space
	spaceUUID := s.addSpace(c)

	// Act: Call GetMachinesBoundToSpaces with the space UUID
	machines, err := s.state.GetMachinesBoundToSpaces(c.Context(), []string{spaceUUID})

	// Assert: No error nor machines
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.HasLen, 0)
}

// TestGetMachinesBoundToSpaces tests that GetMachinesBoundToSpaces
// returns machines that have units belonging to applications with bindings to
// the specified spaces or have positive constraints on it.
func (s *spaceMachineSuite) TestGetMachinesBoundToSpaces(c *tc.C) {
	// Arrange
	spaceUUID := s.addSpace(c)
	charmUUID := s.addCharm(c)
	relationUUID := s.addCharmRelation(c, corecharm.ID(charmUUID), charm.Relation{
		Name:  "whatever",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	})

	// Create applications with a relation bound to the specific space,
	// another bound by default and another one not bound
	appBoundByRelUUID := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	s.addApplicationEndpoint(c, coreapplication.ID(appBoundByRelUUID), relationUUID, spaceUUID)
	appBoundByDefaultUUID := s.addApplication(c, charmUUID, spaceUUID)
	s.addApplicationEndpoint(c, coreapplication.ID(appBoundByDefaultUUID), relationUUID, "")
	appNotBoundUUID := s.addApplication(c, charmUUID, network.AlphaSpaceId.String())
	s.addApplicationEndpoint(c, coreapplication.ID(appNotBoundUUID), relationUUID, "")

	// Create a network nodes and machines
	netNodeUUID1 := s.addNetNode(c)
	s.addMachine(c, "bound-by-relation", netNodeUUID1)
	netNodeUUID2 := s.addNetNode(c)
	s.addMachine(c, "bound-by-default", netNodeUUID2)
	netNodeUUID3 := s.addNetNode(c)
	_ = s.addMachine(c, "not-bound", netNodeUUID3)
	netNodeUUID4 := s.addNetNode(c)
	machineBoundByConstraint := s.addMachine(c, "bound-by-constraint", netNodeUUID4)
	s.addSpaceConstraint(c, machineBoundByConstraint.String(), spaceUUID, true)

	// Create units for the applications on the machines
	s.addUnit(c, appBoundByRelUUID, charmUUID, netNodeUUID1)
	s.addUnit(c, appBoundByDefaultUUID, charmUUID, netNodeUUID2)
	s.addUnit(c, appNotBoundUUID, charmUUID, netNodeUUID3)

	// Add a link layer device and IP address to the "bound-by-default" machine
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID2, "eth0", "aa:bb:cc:dd:ee:ff", network.EthernetDevice)
	s.addIPAddress(c, deviceUUID, netNodeUUID2, "192.168.1.10/24")

	// Act: Call GetMachinesBoundToSpaces with the space UUID
	spaceUUIDs := []string{spaceUUID}
	machines, err := s.state.GetMachinesBoundToSpaces(c.Context(), spaceUUIDs)

	// Assert: check that expected machines are retrieved
	c.Assert(err, tc.ErrorIsNil)
	cleanMachines := transform.Slice(machines, func(f internal.CheckableMachine) boundMachine {
		s, ok := f.(boundMachine)
		c.Assert(ok, tc.Equals, true, tc.Commentf("Machine %s is not a boundMachine", s.machineName))
		s.logger = nil
		return s
	})
	c.Check(cleanMachines, tc.SameContents, []boundMachine{
		{
			machineName:  "bound-by-relation",
			inSpaceUUIDs: spaceUUIDs,
		},
		{
			machineName:  "bound-by-constraint",
			inSpaceUUIDs: spaceUUIDs,
		},
		{
			machineName:  "bound-by-default",
			addresses:    []string{"192.168.1.10"},
			inSpaceUUIDs: spaceUUIDs,
		},
	})
}

// TestGetMachinesNotAllowedInSpaceNoMachines tests that GetMachinesNotAllowedInSpace
// returns an empty slice when no machines are bound to the specified spaces.
func (s *spaceMachineSuite) TestGetMachinesNotAllowedInSpaceNoMachines(c *tc.C) {
	// Arrange: add a space
	spaceUUID := s.addSpace(c)

	// Act: Call GetMachinesBoundToSpaces with the space UUID
	machines, err := s.state.GetMachinesNotAllowedInSpace(c.Context(), spaceUUID)

	// Assert: No error nor machines
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.HasLen, 0)
}

// TestGetMachinesNotAllowedInSpace tests that GetMachinesNotAllowedInSpace
// returns machines that have negative constraints against the specified space.
func (s *spaceMachineSuite) TestGetMachinesNotAllowedInSpace(c *tc.C) {
	// Arrange: add a space
	spaceUUID := s.addSpace(c)

	// Create network nodes and machines
	netNodeUUID1 := s.addNetNode(c)
	machineAllergicUUID := s.addMachine(c, "allergic-1", netNodeUUID1)

	netNodeUUID2 := s.addNetNode(c)
	machineNotAllergicUUID := s.addMachine(c, "not-allergic", netNodeUUID2)

	netNodeUUID3 := s.addNetNode(c)
	machineAllergicUUID2 := s.addMachine(c, "allergic-2", netNodeUUID3)

	// Add negative constraint (allergic) for machine 0 and 2
	s.addSpaceConstraint(c, machineAllergicUUID.String(), spaceUUID, false)
	s.addSpaceConstraint(c, machineAllergicUUID2.String(), spaceUUID, false)

	// Add positive constraint for machine 1
	s.addSpaceConstraint(c, machineNotAllergicUUID.String(), spaceUUID, true)

	// Add a link layer device and IP address to the "allergic-1" machine
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID1, "eth0", "aa:bb:cc:dd:ee:ff", network.EthernetDevice)
	s.addIPAddress(c, deviceUUID, netNodeUUID1, "192.168.1.20/24")

	// Act: Call GetMachinesNotAllowedInSpace with the space UUID
	machines, err := s.state.GetMachinesNotAllowedInSpace(c.Context(), spaceUUID)

	// Assert: No error and correct machines returned
	c.Assert(err, tc.ErrorIsNil)
	cleanMachines := transform.Slice(machines, func(f internal.CheckableMachine) allergicMachine {
		s, ok := f.(allergicMachine)
		c.Assert(ok, tc.Equals, true, tc.Commentf("Machine %s is not an allergicMachine", s.machineName))
		s.logger = nil
		return s
	})
	c.Check(cleanMachines, tc.SameContents, []allergicMachine{
		{
			machineName:      "allergic-1",
			addresses:        []string{"192.168.1.20"},
			excludeSpaceUUID: spaceUUID,
		},
		{
			machineName:      "allergic-2",
			excludeSpaceUUID: spaceUUID,
		},
	})
}

// TestBoundMachineAccept tests the Accept method of boundMachine.
func (s *spaceMachineSuite) TestBoundMachineAccept(c *tc.C) {
	// Create a test space topology
	fakeSpaceInfos := func(cidr string) network.SpaceInfo {
		return network.SpaceInfo{
			ID:   network.SpaceUUID("id-" + cidr),
			Name: network.SpaceName("name-" + cidr),
			Subnets: network.SubnetInfos{
				{
					CIDR: cidr,
				},
			},
		}
	}
	space1 := fakeSpaceInfos("192.168.1.0/24")
	space2 := fakeSpaceInfos("10.0.0.0/24")
	topology := network.SpaceInfos{space1, space2}

	// Test cases
	tests := []struct {
		name          string
		machine       boundMachine
		expectedError string
	}{
		{
			name: "machine with all required addresses",
			machine: boundMachine{
				machineName:  "machine-1",
				addresses:    []string{"192.168.1.10"},
				inSpaceUUIDs: []string{"id-192.168.1.0/24"},
				logger:       s.state.logger,
			},
			expectedError: "",
		},
		{
			name: "machine with multiple required spaces and all addresses",
			machine: boundMachine{
				machineName:  "machine-2",
				addresses:    []string{"192.168.1.10", "10.0.0.10"},
				inSpaceUUIDs: []string{"id-192.168.1.0/24", "id-10.0.0.0/24"},
				logger:       s.state.logger,
			},
			expectedError: "",
		},
		{
			name: "machine missing address in required space",
			machine: boundMachine{
				machineName:  "machine-3",
				addresses:    []string{"10.0.0.10"},
				inSpaceUUIDs: []string{"id-192.168.1.0/24"},
				logger:       s.state.logger,
			},
			expectedError: `machine "machine-3" is missing addresses in spaces name-192.168.1.0/24`,
		},
		{
			name: "machine with multiple required spaces but missing one address",
			machine: boundMachine{
				machineName:  "machine-4",
				addresses:    []string{"192.168.1.10"},
				inSpaceUUIDs: []string{"id-192.168.1.0/24", "id-10.0.0.0/24"},
				logger:       s.state.logger,
			},
			expectedError: `machine "machine-4" is missing addresses in spaces name-10.0.0.0/24`,
		},
		{
			name: "machine with multiple required spaces but missing all addresses",
			machine: boundMachine{
				machineName:  "machine-5",
				addresses:    []string{"11.0.0.10"},
				inSpaceUUIDs: []string{"id-192.168.1.0/24", "id-10.0.0.0/24"},
				logger:       s.state.logger,
			},
			expectedError: `machine "machine-5" is missing addresses in spaces name-10.0.0.0/24, name-192.168.1.0/24`,
		},
		{
			name: "machine with no required spaces",
			machine: boundMachine{
				machineName:  "machine-7",
				addresses:    []string{"192.168.1.10"},
				inSpaceUUIDs: []string{},
				logger:       s.state.logger,
			},
			expectedError: "",
		},
	}

	// Run tests
	for _, test := range tests {
		c.Logf("Running test: %s", test.name)
		err := test.machine.Accept(c.Context(), topology)
		if test.expectedError == "" {
			c.Check(err, tc.IsNil)
		} else {
			c.Check(err, tc.ErrorMatches, test.expectedError)
		}
	}
}

// TestAllergicMachineAccept tests the Accept method of allergic machine.
func (s *spaceMachineSuite) TestAllergicMachineAccept(c *tc.C) {
	// Create a test space topology
	fakeSpaceInfos := func(cidr string) network.SpaceInfo {
		return network.SpaceInfo{
			ID:   network.SpaceUUID("id-" + cidr),
			Name: network.SpaceName("name-" + cidr),
			Subnets: network.SubnetInfos{
				{
					CIDR: cidr,
				},
			},
		}
	}
	space1 := fakeSpaceInfos("192.168.1.0/24")
	space2 := fakeSpaceInfos("10.0.0.0/24")
	topology := network.SpaceInfos{space1, space2}

	// Test cases
	tests := []struct {
		name          string
		machine       allergicMachine
		expectedError string
	}{
		{
			name: "machine with an address not in excluded space",
			machine: allergicMachine{
				machineName:      "machine-1",
				addresses:        []string{"192.168.1.10"},
				excludeSpaceUUID: "id-10.0.0.0/24",
				logger:           s.state.logger,
			},
			expectedError: "",
		},
		{
			name: "machine with several addresses, one in excluded space",
			machine: allergicMachine{
				machineName:      "machine-2",
				addresses:        []string{"192.168.1.10", "172.0.0.10", "10.10.1.10"},
				excludeSpaceUUID: "id-192.168.1.0/24",
				logger:           s.state.logger,
			},
			expectedError: `machine "machine-2" would have 1 addresses in excluded space "name-192.168.1.0/24"` +
				` .192.168.1.10.`,
		},
		{
			name: "machine with several addresses, some in excluded space",
			machine: allergicMachine{
				machineName:      "machine-3",
				addresses:        []string{"192.168.1.10", "192.168.1.11", "10.10.1.10"},
				excludeSpaceUUID: "id-192.168.1.0/24",
				logger:           s.state.logger,
			},
			expectedError: `machine "machine-3" would have 2 addresses in excluded space "name-192.168.1.0/24"` +
				` .192.168.1.10, 192.168.1.11.`,
		},
		{
			name: "machine with no addresses",
			machine: allergicMachine{
				machineName:      "machine-4",
				addresses:        nil,
				excludeSpaceUUID: "id-192.168.1.0/24",
				logger:           s.state.logger,
			},
			expectedError: "",
		},
	}

	// Run tests
	for _, test := range tests {
		c.Logf("Running test: %s", test.name)
		err := test.machine.Accept(c.Context(), topology)
		if test.expectedError == "" {
			c.Check(err, tc.IsNil)
		} else {
			c.Check(err, tc.ErrorMatches, test.expectedError)
		}
	}
}

func (s *spaceMachineSuite) addSpaceConstraint(c *tc.C, machineUUID, spaceUUID string, positive bool) string {
	constraintUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO "constraint" (uuid) VALUES (?)`, constraintUUID)
	s.query(c, `INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?, ?)`, machineUUID,
		constraintUUID)
	s.query(c, `INSERT INTO constraint_space (constraint_uuid, space, exclude) VALUES (?, ?, ?)`,
		constraintUUID, spaceUUID, !positive)
	return constraintUUID
}
