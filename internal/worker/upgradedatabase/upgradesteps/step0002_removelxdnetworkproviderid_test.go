// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"
	"database/sql"
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type step0002Suite struct {
	schematesting.ModelSuite
}

func TestStep0002Suite(t *stdtesting.T) {
	tc.Run(t, &step0002Suite{})
}

func (s *step0002Suite) TestRemoveLXDSubnetProviderIDSuccess(c *tc.C) {
	// Arrange: this upgrade step should only run on LXD clouds.
	s.addModel(c, "lxd")
	subnetOneUUID := s.addSubnet(c, "203.0.113.0/24")
	s.addProviderNetwork(c, "net-lxdbr0", subnetOneUUID)
	s.addProviderSubnet(c, "subnet-lxdbr0-203.0.113.0/24", subnetOneUUID)
	subnetTwoUUID := s.addSubnet(c, "198.51.100.0/24")
	s.addProviderNetwork(c, "net-docker0", subnetTwoUUID)
	s.addProviderSubnet(c, "subnet-docker0-198.51.100.0/24", subnetTwoUUID)
	// subnet 3 ensures that only provider network IDs starting with net-
	// have it removed.
	subnetThreeUUID := s.addSubnet(c, "2001:DB8::/32")
	s.addProviderNetwork(c, "test-net-me", subnetThreeUUID)
	s.addProviderSubnet(c, "subnet-test-net-me-2001:DB8::/32", subnetThreeUUID)
	modelDB := s.ModelTxnRunner()

	// Act
	err := Step0002_RemoveLXDSubnetProviderID(c.Context(), nil, modelDB, "")

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	s.checkRowCount(c, "provider_subnet", 0)

	obtainedProviderNetworks := s.readProviderNetwork(c)
	c.Check(obtainedProviderNetworks, tc.SameContents, []providerNetwork{
		{
			UUID:              subnetOneUUID,
			ProviderNetworkID: "lxdbr0",
		}, {
			UUID:              subnetTwoUUID,
			ProviderNetworkID: "docker0",
		}, {
			UUID:              subnetThreeUUID,
			ProviderNetworkID: "test-net-me",
		},
	})
}

func (s *step0002Suite) TestRemoveLXDSubnetProviderIDIdempotent(c *tc.C) {
	// Arrange: this upgrade step should only run on LXD clouds.
	s.addModel(c, "lxd")
	subnetOneUUID := s.addSubnet(c, "203.0.113.0/24")
	s.addProviderNetwork(c, "net-lxdbr0", subnetOneUUID)
	s.addProviderSubnet(c, "subnet-lxdbr0-203.0.113.0/24", subnetOneUUID)
	subnetTwoUUID := s.addSubnet(c, "198.51.100.0/24")
	s.addProviderNetwork(c, "net-docker0", subnetTwoUUID)
	s.addProviderSubnet(c, "subnet-docker0-198.51.100.0/24", subnetTwoUUID)
	// subnet 3 ensures that only provider network IDs starting with net-
	// have it removed.
	subnetThreeUUID := s.addSubnet(c, "2001:DB8::/32")
	s.addProviderNetwork(c, "test-net-me", subnetThreeUUID)
	s.addProviderSubnet(c, "subnet-test-net-me-2001:DB8::/32", subnetThreeUUID)
	modelDB := s.ModelTxnRunner()

	// Act
	err := Step0002_RemoveLXDSubnetProviderID(c.Context(), nil, modelDB, "")

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	// Act
	err = Step0002_RemoveLXDSubnetProviderID(c.Context(), nil, modelDB, "")

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	s.checkRowCount(c, "provider_subnet", 0)

	obtainedProviderNetworks := s.readProviderNetwork(c)
	c.Check(obtainedProviderNetworks, tc.SameContents, []providerNetwork{
		{
			UUID:              subnetOneUUID,
			ProviderNetworkID: "lxdbr0",
		}, {
			UUID:              subnetTwoUUID,
			ProviderNetworkID: "docker0",
		}, {
			UUID:              subnetThreeUUID,
			ProviderNetworkID: "test-net-me",
		},
	})
}

func (s *step0002Suite) TestDoNotRunRemoveLXDSubnetProviderIDEC2(c *tc.C) {
	s.addModel(c, "ec2")
	s.testDoNotRunStep(c)
}

func (s *step0002Suite) TestDoNotRunRemoveLXDSubnetProviderIDAzure(c *tc.C) {
	s.addModel(c, "azure")
	s.testDoNotRunStep(c)
}

func (s *step0002Suite) testDoNotRunStep(c *tc.C) {
	// Arrange
	subnetOneUUID := s.addSubnet(c, "203.0.113.0/24")
	s.addProviderNetwork(c, "net-lxdbr0", subnetOneUUID)
	s.addProviderSubnet(c, "subnet-lxdbr0-203.0.113.0/24", subnetOneUUID)
	subnetTwoUUID := s.addSubnet(c, "198.51.100.0/24")
	s.addProviderNetwork(c, "net-docker0", subnetTwoUUID)
	s.addProviderSubnet(c, "subnet-docker0-198.51.100.0/24", subnetTwoUUID)
	modelDB := s.ModelTxnRunner()

	// Act
	err := Step0002_RemoveLXDSubnetProviderID(c.Context(), nil, modelDB, "")

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	s.checkRowCount(c, "provider_subnet", 2)
}

// query executes a given SQL query with optional arguments within a
// transactional context using the test database.
func (s *step0002Suite) query(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

func (s *step0002Suite) addModel(c *tc.C, cloudType string) {
	s.query(c, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, 'test-model', 'admin', 'iaas', 'test-cloud', ?)
		`, "model-uuid", "controller-uuid", cloudType)
}

func (s *step0002Suite) addSubnet(c *tc.C, cidr string) string {
	subnetUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.query(c, `INSERT INTO subnet (uuid, cidr) VALUES (?, ?)`,
		subnetUUID, cidr)
	return subnetUUID
}

func (s *step0002Suite) addProviderSubnet(c *tc.C, providerSubnetID, subnetUUID string) {
	s.query(c, `INSERT INTO provider_subnet (subnet_uuid, provider_id) VALUES (?, ?)`,
		subnetUUID, providerSubnetID)
}

func (s *step0002Suite) addProviderNetwork(c *tc.C, providerNetworkID, subnetUUID string) {
	s.query(c, `INSERT INTO provider_network (uuid, provider_network_id) VALUES (?, ?)`,
		subnetUUID, providerNetworkID)
}

// checkRowCount checks that the given table has the expected number of rows.
func (s *step0002Suite) checkRowCount(c *tc.C, table string, expected int) {
	obtained := -1
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		return tx.QueryRowContext(ctx, query).Scan(&obtained)
	})
	c.Assert(err, tc.IsNil, tc.Commentf("counting rows in table %q", table))
	c.Check(obtained, tc.Equals, expected, tc.Commentf("count of %q rows", table))
}

func (s *step0002Suite) readProviderNetwork(c *tc.C) []providerNetwork {
	rows, err := s.DB().QueryContext(c.Context(), `SELECT * FROM provider_network`)
	c.Assert(err, tc.IsNil)
	defer func() { _ = rows.Close() }()
	foundOfferEndpoints := []providerNetwork{}
	for rows.Next() {
		var found providerNetwork
		err = rows.Scan(&found.UUID, &found.ProviderNetworkID)
		c.Assert(err, tc.IsNil)
		foundOfferEndpoints = append(foundOfferEndpoints, found)
	}
	return foundOfferEndpoints
}

type providerNetwork struct {
	UUID              string `db:"uuid"`
	ProviderNetworkID string `db:"provider_network_id"`
}
