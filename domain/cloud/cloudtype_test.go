// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

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

		// If this sub test fails it indicates that the cloud type enums have
		// not been defined correctly.
		c.Run(fmt.Sprintf("cloud type id %d/%s bounds check", id, typeName),
			func(t *testing.T) {
				ct := CloudType(id)
				if ct <= cloudTypeInvalidLow || ct >= cloudTypeInvalidHigh {
					t.Errorf("database cloud type %d/%q lives not within enum range", id, typeName)
				}
			},
		)
	}

	c.Check(dbValues, tc.DeepEquals, map[CloudType]string{
		CloudTypeKubernetes: CloudTypeKubernetes.String(),
		CloudTypeLXD:        CloudTypeLXD.String(),
		CloudTypeMAAS:       CloudTypeMAAS.String(),
		CloudTypeManual:     CloudTypeManual.String(),
		CloudTypeAzure:      CloudTypeAzure.String(),
		CloudTypeEC2:        CloudTypeEC2.String(),
		CloudTypeGCE:        CloudTypeGCE.String(),
		CloudTypeOCI:        CloudTypeOCI.String(),
		CloudTypeOpenStack:  CloudTypeOpenStack.String(),
		CloudTypeVSphere:    CloudTypeVSphere.String(),
	})
}

// TestCloudTypeIsValid tests that the values in the cloud_type table are all
// considered valid when converted to a [CloudType] enum.
func (s *cloudTypeSuite) TestCloudTypeIsValid(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id FROM cloud_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	for rows.Next() {
		var id int
		err := rows.Scan(&id)
		c.Assert(err, tc.ErrorIsNil)

		c.Run(fmt.Sprintf("cloud type id %d", id), func(t *testing.T) {
			ct := CloudType(id)
			tc.Check(t, ct.IsValid(), tc.IsTrue)
		})
	}
}
