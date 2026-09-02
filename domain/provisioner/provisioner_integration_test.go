// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/domain/provisioner/service"
	modelstate "github.com/juju/juju/domain/provisioner/state/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type provisionerIntegrationSuite struct {
	schematesting.ModelSuite
}

func TestProvisionerIntegrationSuite(t *testing.T) {
	tc.Run(t, &provisionerIntegrationSuite{})
}

func (s *provisionerIntegrationSuite) newService(c *tc.C) *service.Service {
	modelState := modelstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	return service.NewService(
		modelState,
		stubControllerState{},
		stubImageMetadataFetcher{},
		coremodel.UUID(s.ModelUUID()),
		loggertesting.WrapCheckLog(c),
	)
}

func (s *provisionerIntegrationSuite) sharedInfo(c *tc.C) provisioner.SharedProvisioningInfo {
	svc := s.newService(c)
	shared, err := svc.GetPreludeProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	shared.ControllerConfig = controller.Config{
		"controller-uuid": "ctrl-uuid-1",
	}
	return shared
}

// TestPreludeProvisioningInfoMinimal verifies that GetPreludeProvisioningInfo
// returns model-level data (model name, cloud info, spaces) from the DB.
func (s *provisionerIntegrationSuite) TestPreludeProvisioningInfoMinimal(c *tc.C) {
	s.addModelInfo(c, "testmodel", "ec2", "us-east-1")

	svc := s.newService(c)
	shared, err := svc.GetPreludeProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(shared.ModelName, tc.Equals, "testmodel")
	c.Check(shared.CloudType, tc.Equals, "ec2")
	c.Check(shared.CloudRegion, tc.Equals, "us-east-1")
	c.Check(shared.CloudName, tc.Equals, "aws")
	c.Check(shared.CloudEndpoint, tc.Equals, "https://ec2.us-east-1.amazonaws.com")
}

// TestPreludeProvisioningInfoWithSpaces verifies spaces and subnets are
// aggregated correctly in the shared info.
func (s *provisionerIntegrationSuite) TestPreludeProvisioningInfoWithSpaces(c *tc.C) {
	s.addModelInfo(c, "testmodel", "ec2", "us-east-1")

	spaceUUID := s.addSpace(c, "production")
	s.addProviderSpace(c, spaceUUID, "provider-space-1")
	subnetUUID := s.addSubnet(c, spaceUUID, "10.0.0.0/24")
	s.addProviderSubnet(c, subnetUUID, "subnet-abc123")
	s.addAvailabilityZone(c, subnetUUID, "us-east-1a")

	svc := s.newService(c)
	shared, err := svc.GetPreludeProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var found bool
	for _, sp := range shared.Spaces {
		if string(sp.Name) == "production" {
			found = true
			c.Check(string(sp.ProviderId), tc.Equals, "provider-space-1")
			c.Assert(sp.Subnets, tc.HasLen, 1)
			c.Check(sp.Subnets[0].CIDR, tc.Equals, "10.0.0.0/24")
			c.Check(string(sp.Subnets[0].ProviderId), tc.Equals, "subnet-abc123")
			c.Assert(sp.Subnets[0].AvailabilityZones, tc.HasLen, 1)
			c.Check(sp.Subnets[0].AvailabilityZones[0], tc.Equals, "us-east-1a")
			break
		}
	}
	c.Assert(found, tc.IsTrue)
}

// TestPreludeProvisioningInfoImageStream verifies that image stream from
// model config is returned.
func (s *provisionerIntegrationSuite) TestPreludeProvisioningInfoImageStream(c *tc.C) {
	s.addModelInfo(c, "testmodel", "ec2", "us-east-1")
	s.setModelConfig(c, "image-stream", "daily")

	svc := s.newService(c)
	shared, err := svc.GetPreludeProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(shared.ImageStream, tc.Equals, "daily")
}

// TestGetProvisioningInfoMinimal verifies a minimal machine returns
// correct provisioning info through the full service+state stack.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoMinimal(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "0", "ubuntu", "22.04/stable")

	shared := s.sharedInfo(c)
	svc := s.newService(c)

	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false, shared)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.Base.OS, tc.Equals, "ubuntu")
	c.Check(info.Base.Channel.Track, tc.Equals, "22.04")
	c.Check(info.Jobs, tc.DeepEquals, []coremodel.MachineJob{coremodel.JobHostUnits})
	c.Check(info.Tags[tags.JujuController], tc.Equals, "ctrl-uuid-1")
	c.Check(info.Tags[tags.JujuModel], tc.Equals, s.ModelUUID())
	c.Check(info.Tags[tags.JujuMachine], tc.Equals, "mymodel-machine-0")
	c.Check(info.ControllerConfig.ControllerUUID(), tc.Equals, "ctrl-uuid-1")
	c.Check(info.PlacementDirective, tc.IsNil)
	c.Check(info.Volumes, tc.HasLen, 0)
}

// TestGetProvisioningInfoWithPlacement verifies placement directives are
// returned through the full stack.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoWithPlacement(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	machineUUID := s.addMachineWithPlatform(c, "1", "ubuntu", "24.04/stable")
	s.addMachinePlacement(c, machineUUID, "zone=us-east-1a")

	shared := s.sharedInfo(c)
	svc := s.newService(c)

	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("1"), false, shared)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.PlacementDirective, tc.Not(tc.IsNil))
	c.Check(*info.PlacementDirective, tc.Equals, "zone=us-east-1a")
}

// TestGetProvisioningInfoWithConstraints verifies constraints flow through
// the full stack correctly.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoWithConstraints(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	machineUUID := s.addMachineWithPlatform(c, "2", "ubuntu", "22.04/stable")
	s.addConstraint(c, machineUUID, "arm64", 8192)

	shared := s.sharedInfo(c)
	svc := s.newService(c)

	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("2"), false, shared)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.Constraints.HasArch(), tc.IsTrue)
	c.Check(*info.Constraints.Arch, tc.Equals, "arm64")
	c.Assert(info.Constraints.HasMem(), tc.IsTrue)
	c.Check(*info.Constraints.Mem, tc.Equals, uint64(8192))
}

// TestGetProvisioningInfoWithEndpointBindings verifies endpoint bindings are
// resolved to space provider IDs through the full stack.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoWithEndpointBindings(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")

	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, "3", life.Alive, netNodeUUID)
	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,0,?,0)`,
		machineUUID, "22.04/stable")

	defaultSpace := s.addSpace(c, "default")
	s.addProviderSpace(c, defaultSpace, "provider-default")
	defaultSubnet := s.addSubnet(c, defaultSpace, "10.0.0.0/24")
	s.addProviderSubnet(c, defaultSubnet, "subnet-default")
	s.addAvailabilityZone(c, defaultSubnet, "us-east-1a")

	dataSpace := s.addSpace(c, "data")
	s.addProviderSpace(c, dataSpace, "provider-data")
	dataSubnet := s.addSubnet(c, dataSpace, "10.0.1.0/24")
	s.addProviderSubnet(c, dataSubnet, "subnet-data")
	s.addAvailabilityZone(c, dataSubnet, "us-east-1a")

	charmUUID := s.addCharm(c, "wordpress")
	appUUID := s.addApplication(c, "wordpress", charmUUID, defaultSpace)
	s.addUnit(c, "wordpress/0", appUUID, charmUUID, netNodeUUID)

	relUUID := s.addCharmRelation(c, charmUUID, "db")
	s.addApplicationEndpoint(c, appUUID, relUUID, &dataSpace)

	shared := s.sharedInfo(c)
	svc := s.newService(c)

	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("3"), false, shared)
	c.Assert(err, tc.ErrorIsNil)

	// The "db" endpoint should resolve to the data space's provider ID.
	c.Check(info.EndpointBindings["db"], tc.Equals, "provider-data")
}

// TestGetProvisioningInfoWithNetworkTopology verifies that space/subnet/AZ
// topology is constructed correctly.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoWithNetworkTopology(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")

	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, "4", life.Alive, netNodeUUID)
	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,0,?,0)`,
		machineUUID, "22.04/stable")

	// Create a space with a subnet and AZ, bind an app endpoint to it.
	appSpace := s.addSpace(c, "appspace")
	s.addProviderSpace(c, appSpace, "provider-appspace")
	subnetUUID := s.addSubnet(c, appSpace, "10.0.1.0/24")
	s.addProviderSubnet(c, subnetUUID, "subnet-app1")
	s.addAvailabilityZone(c, subnetUUID, "us-east-1b")

	charmUUID := s.addCharm(c, "myapp")
	appUUID := s.addApplication(c, "myapp", charmUUID, appSpace)
	s.addUnit(c, "myapp/0", appUUID, charmUUID, netNodeUUID)

	shared := s.sharedInfo(c)
	svc := s.newService(c)

	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("4"), false, shared)
	c.Assert(err, tc.ErrorIsNil)

	// The space should be mapped to its provider subnet IDs.
	c.Assert(info.SpaceSubnets, tc.Not(tc.IsNil))
	c.Check(info.SpaceSubnets["appspace"], tc.DeepEquals, []string{"subnet-app1"})

	// The subnet should be mapped to its availability zones.
	c.Assert(info.SubnetAZs, tc.Not(tc.IsNil))
	c.Check(info.SubnetAZs["subnet-app1"], tc.DeepEquals, []string{"us-east-1b"})
}

// TestGetProvisioningInfoWithRootDisk verifies root disk storage pool
// flows through the full stack.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoWithRootDisk(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	machineUUID := s.addMachineWithPlatform(c, "5", "ubuntu", "22.04/stable")

	// Add constraint with root-disk-source.
	consUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO "constraint" (uuid, root_disk_source) VALUES (?,?)`,
		consUUID, "ebs-fast")
	s.runQuery(c, `INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?,?)`,
		machineUUID, consUUID)

	// Add storage pool.
	poolUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO storage_pool (uuid, name, type) VALUES (?,?,?)`,
		poolUUID, "ebs-fast", "ebs")
	s.runQuery(c, `INSERT INTO storage_pool_attribute (storage_pool_uuid, "key", value) VALUES (?,?,?)`,
		poolUUID, "volume-type", "gp3")

	shared := s.sharedInfo(c)
	svc := s.newService(c)

	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("5"), false, shared)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.RootDisk, tc.Not(tc.IsNil))
	c.Check(info.RootDisk.Provider, tc.Equals, "ebs")
	c.Check(info.RootDisk.Attributes["volume-type"], tc.Equals, "gp3")
}

// TestGetProvisioningInfoCloudInitUserData verifies cloud-init user data
// from model config is parsed and returned.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoCloudInitUserData(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "6", "ubuntu", "22.04/stable")

	// Set cloud-init user data in model config.
	s.setModelConfig(c, "cloudinit-userdata", "packages:\n- curl\n")

	shared := s.sharedInfo(c)
	svc := s.newService(c)

	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("6"), false, shared)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.CloudInitUserData, tc.Not(tc.IsNil))
	c.Check(info.CloudInitUserData["packages"], tc.DeepEquals, []any{"curl"})
}

// TestGetProvisioningInfoControllerMachine verifies that a controller machine
// gets the JobManageModel job.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoControllerMachine(c *tc.C) {
	s.addModelInfo(c, "controller", "ec2", "us-east-1")

	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, "0", life.Alive, netNodeUUID)
	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,0,?,0)`,
		machineUUID, "22.04/stable")

	// Create a controller application with a unit on this machine.
	spaceUUID := s.addSpace(c, "default")
	s.addProviderSpace(c, spaceUUID, "provider-default")
	subnetUUID := s.addSubnet(c, spaceUUID, "10.0.0.0/24")
	s.addProviderSubnet(c, subnetUUID, "subnet-ctrl")
	s.addAvailabilityZone(c, subnetUUID, "us-east-1a")

	charmUUID := s.addCharm(c, "juju-controller")
	appUUID := s.addApplication(c, "controller", charmUUID, spaceUUID)
	s.addApplicationController(c, appUUID)
	s.addUnit(c, "controller/0", appUUID, charmUUID, netNodeUUID)

	shared := s.sharedInfo(c)
	svc := s.newService(c)

	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), true, shared)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.Jobs, tc.DeepEquals, []coremodel.MachineJob{coremodel.JobHostUnits, coremodel.JobManageModel})
}

// TestGetProvisioningInfoResourceTags verifies resource tags from model
// config are applied to machine tags.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoResourceTags(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "7", "ubuntu", "22.04/stable")
	s.setModelConfig(c, "resource-tags", "department=engineering team=platform")

	shared := s.sharedInfo(c)
	svc := s.newService(c)

	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("7"), false, shared)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.Tags["department"], tc.Equals, "engineering")
	c.Check(info.Tags["team"], tc.Equals, "platform")
}

// TestGetProvisioningInfoMachineNotFound verifies that querying a
// non-existent machine returns a MachineNotFound error.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoMachineNotFound(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")

	shared := s.sharedInfo(c)
	svc := s.newService(c)

	_, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("99"), false, shared)
	c.Assert(err, tc.Not(tc.IsNil))
}

// TestGetProvisioningInfoMultipleMachinesBatch verifies the intended usage
// pattern: fetch prelude once, then call GetProvisioningInfo for each machine.
func (s *provisionerIntegrationSuite) TestGetProvisioningInfoMultipleMachinesBatch(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "0", "ubuntu", "22.04/stable")
	s.addMachineWithPlatform(c, "1", "ubuntu", "24.04/stable")

	svc := s.newService(c)

	// Fetch shared info once.
	shared, err := svc.GetPreludeProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	shared.ControllerConfig = controller.Config{"controller-uuid": "ctrl-uuid-1"}

	// Fetch per-machine info for each.
	info0, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false, shared)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info0.Base.Channel.Track, tc.Equals, "22.04")
	c.Check(info0.Tags[tags.JujuMachine], tc.Equals, "mymodel-machine-0")

	info1, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("1"), false, shared)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info1.Base.Channel.Track, tc.Equals, "24.04")
	c.Check(info1.Tags[tags.JujuMachine], tc.Equals, "mymodel-machine-1")
}

// --- Test helpers ---

func (s *provisionerIntegrationSuite) runQuery(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %v)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB"))
}

func (s *provisionerIntegrationSuite) addNetNode(c *tc.C) string {
	netNodeUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	return netNodeUUID
}

func (s *provisionerIntegrationSuite) addMachineWithPlatform(c *tc.C, name, osName, channel string) string {
	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, name, life.Alive, netNodeUUID)

	var osID int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT id FROM os WHERE name = ?`, osName).Scan(&osID)
	})
	c.Assert(err, tc.ErrorIsNil)

	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,?,?,?)`,
		machineUUID, osID, channel, 0)
	return machineUUID
}

func (s *provisionerIntegrationSuite) addMachinePlacement(c *tc.C, machineUUID, directive string) {
	s.runQuery(c, `INSERT INTO machine_placement (machine_uuid, scope_id, directive) VALUES (?,?,?)`,
		machineUUID, 0, directive)
}

func (s *provisionerIntegrationSuite) addConstraint(c *tc.C, machineUUID, arch string, mem int64) string {
	consUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO "constraint" (uuid, arch, mem) VALUES (?,?,?)`,
		consUUID, arch, mem)
	s.runQuery(c, `INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?,?)`,
		machineUUID, consUUID)
	return consUUID
}

func (s *provisionerIntegrationSuite) addSpace(c *tc.C, name string) string {
	spaceUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO space (uuid, name) VALUES (?,?)`, spaceUUID, name)
	return spaceUUID
}

func (s *provisionerIntegrationSuite) addSubnet(c *tc.C, spaceUUID, cidr string) string {
	subnetUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?,?,?)`,
		subnetUUID, cidr, spaceUUID)
	return subnetUUID
}

func (s *provisionerIntegrationSuite) addProviderSubnet(c *tc.C, subnetUUID, providerID string) {
	s.runQuery(c, `INSERT INTO provider_subnet (subnet_uuid, provider_id) VALUES (?,?)`,
		subnetUUID, providerID)
}

func (s *provisionerIntegrationSuite) addProviderSpace(c *tc.C, spaceUUID, providerID string) {
	s.runQuery(c, `INSERT INTO provider_space (space_uuid, provider_id) VALUES (?,?)`,
		spaceUUID, providerID)
}

func (s *provisionerIntegrationSuite) addAvailabilityZone(c *tc.C, subnetUUID, azName string) {
	azUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT OR IGNORE INTO availability_zone (uuid, name) VALUES (?,?)`, azUUID, azName)
	var actualUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT uuid FROM availability_zone WHERE name = ?`, azName).Scan(&actualUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	s.runQuery(c, `INSERT INTO availability_zone_subnet (availability_zone_uuid, subnet_uuid) VALUES (?,?)`,
		actualUUID, subnetUUID)
}

func (s *provisionerIntegrationSuite) addCharm(c *tc.C, name string) string {
	charmUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO charm (uuid, source_id, reference_name, revision) VALUES (?,1,?,0)`,
		charmUUID, name)
	return charmUUID
}

func (s *provisionerIntegrationSuite) addApplication(c *tc.C, name, charmUUID, spaceUUID string) string {
	appUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?,?,?,?,?)`,
		appUUID, name, life.Alive, charmUUID, spaceUUID)
	return appUUID
}

func (s *provisionerIntegrationSuite) addUnit(c *tc.C, name, appUUID, charmUUID, netNodeUUID string) string {
	unitUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid) VALUES (?,?,?,?,?,?)`,
		unitUUID, name, life.Alive, appUUID, netNodeUUID, charmUUID)
	return unitUUID
}

func (s *provisionerIntegrationSuite) addCharmRelation(c *tc.C, charmUUID, name string) string {
	relUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id) VALUES (?,?,?,0,0)`,
		relUUID, charmUUID, name)
	return relUUID
}

func (s *provisionerIntegrationSuite) addApplicationEndpoint(c *tc.C, appUUID, charmRelUUID string, spaceUUID *string) {
	epUUID := uuid.MustNewUUID().String()
	if spaceUUID != nil {
		s.runQuery(c, `INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?,?,?,?)`,
			epUUID, appUUID, *spaceUUID, charmRelUUID)
	} else {
		s.runQuery(c, `INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid) VALUES (?,?,?)`,
			epUUID, appUUID, charmRelUUID)
	}
}

func (s *provisionerIntegrationSuite) addApplicationController(c *tc.C, appUUID string) {
	s.runQuery(c, `INSERT INTO application_controller (application_uuid) VALUES (?)`, appUUID)
}

func (s *provisionerIntegrationSuite) addModelInfo(c *tc.C, name, cloudType, cloudRegion string) {
	modelUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type, cloud_region) VALUES (?,?,?,?,?,?,?,?)`,
		modelUUID, "ctrl-uuid", name, "", "iaas", "aws", cloudType, cloudRegion)
}

func (s *provisionerIntegrationSuite) setModelConfig(c *tc.C, key, value string) {
	s.runQuery(c, `INSERT OR REPLACE INTO model_config (key, value) VALUES (?,?)`, key, value)
}

// --- Stubs for controller state and image metadata fetcher ---

// stubControllerState implements service.ControllerState with simple stubs.
type stubControllerState struct{}

func (stubControllerState) GetCloudEndpoint(_ context.Context, cloudName, regionName string) (string, error) {
	// Return a realistic-looking endpoint for integration test verification.
	if cloudName == "aws" && regionName != "" {
		return "https://ec2." + regionName + ".amazonaws.com", nil
	}
	return "", nil
}

func (stubControllerState) GetCachedImageMetadata(_ context.Context, _, _, _, _, _ string) ([]provisioner.CloudImageMetadata, error) {
	// Return empty — forces fallback to image metadata fetcher.
	return nil, nil
}

// stubImageMetadataFetcher implements service.ImageMetadataFetcher with
// a minimal stub that returns a single image.
type stubImageMetadataFetcher struct{}

func (stubImageMetadataFetcher) FetchImageMetadata(_ context.Context, constraint provisioner.ImageConstraint) ([]provisioner.CloudImageMetadata, error) {
	var arch string
	if len(constraint.Arches) > 0 {
		arch = constraint.Arches[0]
	} else {
		arch = "amd64"
	}
	var version string
	if len(constraint.Releases) > 0 {
		version = constraint.Releases[0]
	}
	return []provisioner.CloudImageMetadata{{
		ImageID:  "img-" + version + "-" + arch,
		Region:   constraint.Region,
		Arch:     arch,
		Version:  version,
		Stream:   constraint.Stream,
		Priority: 10,
	}}, nil
}

// Ensure stubs satisfy interfaces at compile time.
var _ service.ControllerState = stubControllerState{}
var _ service.ImageMetadataFetcher = stubImageMetadataFetcher{}

// Suppress unused import warnings for network package.
var _ = network.SpaceInfo{}
