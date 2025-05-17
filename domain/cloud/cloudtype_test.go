// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

// cloudTypeSuite is a test suite for making sure that the cloud type enums
// defined in both this package and in [corecloud.CloudType] are in sync with
// the database.
type cloudTypeSuite struct {
	schematesting.ControllerSuite
}

// TestCloudTypeSuite runs the tests located in [cloudTypeSuite].
func TestCloudTypeSuite(t *testing.T) {
	tc.Run(t, &cloudTypeSuite{})
}

// TestCloudTypeDBValues tests that the values in the cloud_type table against
// the established [CloudType] enums in this package to make sure there is no
// skew between the database and this package.
func (s *cloudTypeSuite) TestCloudTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, type FROM cloud_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[CloudType]string)
	for rows.Next() {
		var (
			id       int
			typeName string
		)
		err := rows.Scan(&id, &typeName)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[CloudType(id)] = typeName
	}

	c.Check(dbValues, tc.DeepEquals, map[CloudType]string{
		CloudTypeKubernetes: "kubernetes",
		CloudTypeLXD:        "lxd",
		CloudTypeMAAS:       "maas",
		CloudTypeManual:     "manual",
		CloudTypeAzure:      "azure",
		CloudTypeEC2:        "ec2",
		CloudTypeGCE:        "gce",
		CloudTypeOCI:        "oci",
		CloudTypeOpenStack:  "openstack",
		CloudTypevSphere:    "vsphere",
	})
}

// TestCloudTypeDBValuesAgainstCoreCloudTypes tests that the database type
// strings in the cloud_type table match the constants defined in
// [corecloud.CloudType]. If this tests fails it means the database has a
// disconnect with the core cloud types and this needs to be corrected to be
// able to establish providers correctly in the controller.
func (s *cloudTypeSuite) TestCloudTypeDBValuesAgainstCoreCloudTypes(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT type FROM cloud_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	var dbValues []corecloud.CloudType
	for rows.Next() {
		var typeName string
		err := rows.Scan(&typeName)
		c.Assert(err, tc.ErrorIsNil)
		dbValues = append(dbValues, corecloud.CloudType(typeName))
	}

	c.Check(dbValues, tc.SameContents, []corecloud.CloudType{
		corecloud.CloudTypeAzure,
		corecloud.CloudTypeEC2,
		corecloud.CloudTypeGCE,
		corecloud.CloudTypeKubernetes,
		corecloud.CloudTypeLXD,
		corecloud.CloudTypeManual,
		corecloud.CloudTypeMAAS,
		corecloud.CloudTypeOCI,
		corecloud.CloudTypeOpenStack,
		corecloud.CloudTypevSphere,
	})
}
