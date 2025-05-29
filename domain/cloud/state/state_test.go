// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/cloud"
	corecloud "github.com/juju/juju/core/cloud"
	cloudtesting "github.com/juju/juju/core/cloud/testing"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/model"
	modelstate "github.com/juju/juju/domain/model/state"
	modelstatetesting "github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/internal/changestream/testing"
	jujudb "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	testing.ControllerSuite
	adminUUID uuid.UUID
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.adminUUID = uuid.MustNewUUID()
	s.ensureUser(c, s.adminUUID.String(), "admin", s.adminUUID.String())
}

var (
	testCloud = cloud.Cloud{
		Name:             "fluffy",
		Type:             "ec2",
		AuthTypes:        []cloud.AuthType{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Endpoint:         "https://endpoint",
		IdentityEndpoint: "https://identity-endpoint",
		StorageEndpoint:  "https://storage-endpoint",
		Regions: []cloud.Region{{
			Name:             "region1",
			Endpoint:         "http://region-endpoint1",
			IdentityEndpoint: "http://region-identity-endpoint1",
			StorageEndpoint:  "http://region-identity-endpoint1",
		}, {
			Name:             "region2",
			Endpoint:         "http://region-endpoint2",
			IdentityEndpoint: "http://region-identity-endpoint2",
			StorageEndpoint:  "http://region-identity-endpoint2",
		}},
		CACertificates:    []string{"cert1", "cert2"},
		SkipTLSVerify:     true,
		IsControllerCloud: false,
	}
	testCloud2 = cloud.Cloud{
		Name:             "fluffy2",
		Type:             "ec2",
		AuthTypes:        []cloud.AuthType{cloud.AccessKeyAuthType, cloud.OAuth2AuthType},
		Endpoint:         "https://endpoint2",
		IdentityEndpoint: "https://identity-endpoint2",
		StorageEndpoint:  "https://storage-endpoint2",
		Regions: []cloud.Region{{
			Name:             "region1",
			Endpoint:         "http://region-endpoint1",
			IdentityEndpoint: "http://region-identity-endpoint1",
			StorageEndpoint:  "http://region-identity-endpoint1",
		}, {
			Name:             "region3",
			Endpoint:         "http://region-endpoint3",
			IdentityEndpoint: "http://region-identity-endpoint3",
			StorageEndpoint:  "http://region-identity-endpoint3",
		}},
		CACertificates:    []string{"cert1", "cert3"},
		SkipTLSVerify:     false,
		IsControllerCloud: false,
	}
)

func (s *stateSuite) assertCloud(c *tc.C, cloud cloud.Cloud) string {
	db := s.DB()

	// Check the cloud record.
	row := db.QueryRow("SELECT uuid, name, cloud_type, endpoint, identity_endpoint, storage_endpoint, skip_tls_verify FROM v_cloud WHERE name = ?", "fluffy")
	c.Assert(row.Err(), tc.ErrorIsNil)

	var dbCloud dbCloud
	err := row.Scan(&dbCloud.UUID, &dbCloud.Name, &dbCloud.Type, &dbCloud.Endpoint, &dbCloud.IdentityEndpoint, &dbCloud.StorageEndpoint, &dbCloud.SkipTLSVerify)
	c.Assert(err, tc.ErrorIsNil)
	if !utils.IsValidUUIDString(dbCloud.UUID) {
		c.Fatalf("invalid cloud uuid %q", dbCloud.UUID)
	}
	c.Check(dbCloud.Name, tc.Equals, cloud.Name)
	c.Check(dbCloud.Type, tc.Equals, "ec2")
	c.Check(dbCloud.Endpoint, tc.Equals, cloud.Endpoint)
	c.Check(dbCloud.IdentityEndpoint, tc.Equals, cloud.IdentityEndpoint)
	c.Check(dbCloud.StorageEndpoint, tc.Equals, cloud.StorageEndpoint)
	c.Check(dbCloud.SkipTLSVerify, tc.Equals, cloud.SkipTLSVerify)

	s.assertAuthTypes(c, dbCloud.UUID, cloud.AuthTypes)
	s.assertCaCerts(c, dbCloud.UUID, cloud.CACertificates)
	s.assertRegions(c, dbCloud.UUID, cloud.Regions)

	return dbCloud.UUID
}

func (s *stateSuite) assertAuthTypes(c *tc.C, cloudUUID string, expected cloud.AuthTypes) {
	db := s.DB()

	var dbAuthTypes = map[int]string{}

	rows, err := db.Query("SELECT id, type FROM auth_type")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, tc.ErrorIsNil)
		dbAuthTypes[id] = value
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)

	rows, err = db.Query("SELECT auth_type_id FROM cloud_auth_type WHERE cloud_uuid = ?", cloudUUID)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	authTypes := set.NewStrings()
	for rows.Next() {
		var id int
		err = rows.Scan(&id)
		c.Assert(err, tc.ErrorIsNil)
		authTypes.Add(dbAuthTypes[id])
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)

	c.Check(authTypes, tc.HasLen, len(expected))
	for _, a := range expected {
		c.Check(authTypes.Contains(string(a)), tc.IsTrue)
	}
}

func (s *stateSuite) assertCaCerts(c *tc.C, cloudUUID string, expected []string) {
	db := s.DB()

	rows, err := db.Query("SELECT ca_cert FROM cloud_ca_cert WHERE cloud_uuid = ?", cloudUUID)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	certs := set.NewStrings()
	for rows.Next() {
		var cert string
		err = rows.Scan(&cert)
		c.Assert(err, tc.ErrorIsNil)
		certs.Add(cert)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)

	c.Check(certs.Values(), tc.SameContents, expected)
}

func regionsFromDbRegions(dbRegions []cloudRegion) []cloud.Region {
	regions := make([]cloud.Region, len(dbRegions))
	for i, region := range dbRegions {
		regions[i] = cloud.Region{
			Name:             region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
		}
	}
	return regions
}

func (s *stateSuite) assertRegions(c *tc.C, cloudUUID string, expected []cloud.Region) {
	db := s.DB()

	rows, err := db.Query("SELECT name, endpoint, identity_endpoint, storage_endpoint FROM cloud_region WHERE cloud_uuid = ?", cloudUUID)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var dbRegions []cloudRegion
	for rows.Next() {
		var dbRegion cloudRegion
		err = rows.Scan(&dbRegion.Name, &dbRegion.Endpoint, &dbRegion.IdentityEndpoint, &dbRegion.StorageEndpoint)
		c.Assert(err, tc.ErrorIsNil)
		dbRegions = append(dbRegions, dbRegion)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)

	regions := regionsFromDbRegions(dbRegions)
	c.Check(regions, tc.SameContents, expected)
}

func (s *stateSuite) assertInsertCloud(c *tc.C, st *State, cloud cloud.Cloud) string {
	cloudUUID := uuid.MustNewUUID()
	err := st.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), cloud)
	c.Assert(err, tc.ErrorIsNil)

	foundCloudUUID := s.assertCloud(c, cloud)
	s.checkPermissionRow(c, permission.AdminAccess, "admin", cloud.Name)
	return foundCloudUUID
}

func (s *stateSuite) TestCreateCloud(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)
}

func (s *stateSuite) TestCreateCloudNewNoRegions(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	cld := testCloud
	cld.Regions = nil
	s.assertInsertCloud(c, st, cld)
}

func (s *stateSuite) TestCreateCloudNewNoCertificates(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	cld := testCloud
	cld.CACertificates = nil
	s.assertInsertCloud(c, st, cld)
}

func (s *stateSuite) TestCreateCloudUpdateExisting(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	originalUUID := s.assertInsertCloud(c, st, testCloud)

	cld := cloud.Cloud{
		Name:             "fluffy",
		Type:             "ec2",
		AuthTypes:        []cloud.AuthType{cloud.AccessKeyAuthType, cloud.OAuth2AuthType},
		Endpoint:         "https://endpoint2",
		IdentityEndpoint: "https://identity-endpoint2",
		StorageEndpoint:  "https://storage-endpoint2",
		Regions: []cloud.Region{{
			Name:             "region1",
			Endpoint:         "http://region-endpoint1",
			IdentityEndpoint: "http://region-identity-endpoint1",
			StorageEndpoint:  "http://region-identity-endpoint1",
		}, {
			Name:             "region3",
			Endpoint:         "http://region-endpoint3",
			IdentityEndpoint: "http://region-identity-endpoint3",
			StorageEndpoint:  "http://region-identity-endpoint3",
		}},
		CACertificates:    []string{"cert1", "cert3"},
		SkipTLSVerify:     false,
		IsControllerCloud: true,
	}

	err := st.UpdateCloud(c.Context(), cld)
	c.Assert(err, tc.ErrorIsNil)

	cloudUUID := s.assertCloud(c, cld)
	c.Assert(originalUUID, tc.Equals, cloudUUID)
}

func (s *stateSuite) TestCreateCloudInvalidType(c *tc.C) {
	cld := testCloud
	cld.Type = "mycloud"

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), uuid.MustNewUUID().String(), cld)
	c.Assert(err, tc.ErrorMatches, `.* cloud type "mycloud" not valid`)
}

func (s *stateSuite) TestCloudWithEmptyNameFails(c *tc.C) {
	cld := testCloud
	cld.Name = ""

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), uuid.MustNewUUID().String(), cld)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *stateSuite) TestCreateCloudInvalidAuthType(c *tc.C) {
	cld := testCloud
	cld.AuthTypes = []cloud.AuthType{"myauth"}

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), uuid.MustNewUUID().String(), cld)
	c.Assert(err, tc.ErrorMatches, `.* auth type "myauth" not valid`)
}

func (s *stateSuite) TestListClouds(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), uuid.MustNewUUID().String(), testCloud)
	c.Assert(err, tc.ErrorIsNil)
	err = st.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), uuid.MustNewUUID().String(), testCloud2)
	c.Assert(err, tc.ErrorIsNil)

	clouds, err := st.ListClouds(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 2)
	if clouds[0].Name == testCloud.Name {
		c.Assert(clouds[0], tc.DeepEquals, testCloud)
		c.Assert(clouds[1], tc.DeepEquals, testCloud2)
	} else {
		c.Assert(clouds[1], tc.DeepEquals, testCloud)
		c.Assert(clouds[0], tc.DeepEquals, testCloud2)
	}
}

func (s *stateSuite) TestCloudIsControllerCloud(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), uuid.MustNewUUID().String(), testCloud)
	c.Assert(err, tc.ErrorIsNil)
	err = st.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), uuid.MustNewUUID().String(), testCloud2)
	c.Assert(err, tc.ErrorIsNil)

	clouds, err := st.ListClouds(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 2)

	for _, cloud := range clouds {
		c.Assert(cloud.IsControllerCloud, tc.Equals, false)
	}

	modelUUID := modeltesting.GenModelUUID(c)
	modelSt := modelstate.NewState(s.TxnRunnerFactory())
	modelstatetesting.CreateInternalSecretBackend(c, s.ControllerTxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	err = modelSt.Create(
		c.Context(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:         testCloud.Name,
			Name:          coremodel.ControllerModelName,
			Qualifier:     "admin",
			AdminUsers:    []user.UUID{user.UUID(s.adminUUID.String())},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = modelSt.Activate(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	clouds, err = st.ListClouds(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 2)

	for _, cloud := range clouds {
		if cloud.Name == testCloud.Name {
			c.Assert(cloud.IsControllerCloud, tc.Equals, true)
		} else {
			c.Assert(cloud.IsControllerCloud, tc.Equals, false)
		}
	}
}

func (s *stateSuite) TestCloud(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), uuid.MustNewUUID().String(), testCloud)
	c.Assert(err, tc.ErrorIsNil)
	err = st.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), uuid.MustNewUUID().String(), testCloud2)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.Cloud(c.Context(), "fluffy3")
	c.Assert(err, tc.ErrorIs, clouderrors.NotFound)

	cloud, err := st.Cloud(c.Context(), "fluffy2")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cloud, tc.DeepEquals, &testCloud2)
}

func (s *stateSuite) TestDeleteCloud(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)

	err := st.DeleteCloud(c.Context(), "fluffy")
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.Cloud(c.Context(), "fluffy")
	c.Assert(err, tc.ErrorIs, clouderrors.NotFound)

	// Do not find the permission
	row := s.DB().QueryRow(`
SELECT uuid, access_type, grant_to, grant_on
FROM v_permission p
WHERE p.grant_to = ?
`, "fluffy")
	c.Assert(row.Err(), tc.ErrorIsNil)
	var grantOn string
	err = row.Scan(&grantOn)
	c.Assert(err, tc.ErrorMatches, "sql: no rows in result set")
}

func (s *stateSuite) TestDeleteCloudInUse(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)

	credUUID := uuid.MustNewUUID().String()
	var numRows int64
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `
INSERT INTO cloud_credential (uuid, name, cloud_uuid, auth_type_id, owner_uuid)
SELECT ?, 'default', uuid, 1, ? FROM cloud
WHERE cloud.name = ?
`
		result, err := tx.ExecContext(ctx, stmt, credUUID, s.adminUUID.String(), "fluffy")
		if err != nil {
			return err
		}
		numRows, err = result.RowsAffected()
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(numRows, tc.Equals, int64(1))

	err = st.DeleteCloud(c.Context(), "fluffy")
	c.Assert(err, tc.ErrorMatches, "cannot delete cloud as it is still in use")

	cloud, err := st.Cloud(c.Context(), "fluffy")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloud.Name, tc.Equals, "fluffy")
}

// TestNullCloudType is a regression test to make sure that we don't allow null
// cloud types.
func (s *stateSuite) TestNullCloudType(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO cloud_type (id, type) VALUES (99, NULL)")
		return err
	})
	c.Assert(jujudb.IsErrConstraintNotNull(err), tc.IsTrue)
}

func (s *stateSuite) ensureUser(c *tc.C, userUUID, name, createdByUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, userUUID, name, name, false, false, createdByUUID, time.Now())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user_authentication (user_uuid, disabled)
			VALUES (?, ?)
		`, userUUID, false)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) checkPermissionRow(c *tc.C, access permission.Access, expectedGrantTo, expectedGrantON string) {
	db := s.DB()

	row := db.QueryRow(`
SELECT uuid
FROM user
WHERE name = ?
`, expectedGrantTo)
	c.Assert(row.Err(), tc.ErrorIsNil)
	var userUUID string
	err := row.Scan(&userUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Find the permission
	row = db.QueryRow(`
SELECT uuid, access_type, grant_to, grant_on
FROM v_permission
`)
	c.Assert(row.Err(), tc.ErrorIsNil)
	var (
		accessType, userUuid, grantTo, grantOn string
	)
	err = row.Scan(&userUuid, &accessType, &grantTo, &grantOn)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the permission as expected.
	c.Check(userUuid, tc.Not(tc.Equals), "")
	c.Check(accessType, tc.Equals, string(access))
	c.Check(grantTo, tc.Equals, userUUID)
	c.Check(grantOn, tc.Equals, expectedGrantON)
}

func (s *stateSuite) TestGetCloudForNonExistentID(c *tc.C) {
	fakeID := cloudtesting.GenCloudUUID(c)
	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetCloudForUUID(c.Context(), fakeID)
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)
}

func (s *stateSuite) TestGetCloudForUUID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)

	db := s.DB()
	var uuid corecloud.UUID
	err := db.QueryRow("SELECT uuid FROM v_cloud where name = ?", testCloud.Name).Scan(&uuid)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(err, tc.ErrorIsNil)
	cloud, err := st.GetCloudForUUID(c.Context(), uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(cloud, tc.DeepEquals, testCloud)
}
