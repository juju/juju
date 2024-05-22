// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	corecloud "github.com/juju/juju/core/cloud"
	cloudtesting "github.com/juju/juju/core/cloud/testing"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/model"
	modelstate "github.com/juju/juju/domain/model/state"
	modelstatetesting "github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/internal/changestream/testing"
	jujudb "github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	testing.ControllerSuite
	adminUUID uuid.UUID
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
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

func (s *stateSuite) assertCloud(c *gc.C, cloud cloud.Cloud) string {
	db := s.DB()

	// Check the cloud record.
	row := db.QueryRow("SELECT uuid, name, cloud_type, endpoint, identity_endpoint, storage_endpoint, skip_tls_verify FROM v_cloud WHERE name = ?", "fluffy")
	c.Assert(row.Err(), jc.ErrorIsNil)

	var dbCloud Cloud
	err := row.Scan(&dbCloud.ID, &dbCloud.Name, &dbCloud.Type, &dbCloud.Endpoint, &dbCloud.IdentityEndpoint, &dbCloud.StorageEndpoint, &dbCloud.SkipTLSVerify)
	c.Assert(err, jc.ErrorIsNil)
	if !utils.IsValidUUIDString(dbCloud.ID) {
		c.Fatalf("invalid cloud uuid %q", dbCloud.ID)
	}
	c.Check(dbCloud.Name, gc.Equals, cloud.Name)
	c.Check(dbCloud.Type, gc.Equals, "ec2")
	c.Check(dbCloud.Endpoint, gc.Equals, cloud.Endpoint)
	c.Check(dbCloud.IdentityEndpoint, gc.Equals, cloud.IdentityEndpoint)
	c.Check(dbCloud.StorageEndpoint, gc.Equals, cloud.StorageEndpoint)
	c.Check(dbCloud.SkipTLSVerify, gc.Equals, cloud.SkipTLSVerify)

	s.assertAuthTypes(c, dbCloud.ID, cloud.AuthTypes)
	s.assertCaCerts(c, dbCloud.ID, cloud.CACertificates)
	s.assertRegions(c, dbCloud.ID, cloud.Regions)

	return dbCloud.ID
}

func (s *stateSuite) assertAuthTypes(c *gc.C, cloudUUID string, expected cloud.AuthTypes) {
	db := s.DB()

	var dbAuthTypes = map[int]string{}

	rows, err := db.Query("SELECT id, type FROM auth_type")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, jc.ErrorIsNil)
		dbAuthTypes[id] = value
	}
	c.Assert(rows.Err(), jc.ErrorIsNil)

	rows, err = db.Query("SELECT auth_type_id FROM cloud_auth_type WHERE cloud_uuid = ?", cloudUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	authTypes := set.NewStrings()
	for rows.Next() {
		var id int
		err = rows.Scan(&id)
		c.Assert(err, jc.ErrorIsNil)
		authTypes.Add(dbAuthTypes[id])
	}
	c.Assert(rows.Err(), jc.ErrorIsNil)

	c.Check(authTypes, gc.HasLen, len(expected))
	for _, a := range expected {
		c.Check(authTypes.Contains(string(a)), jc.IsTrue)
	}
}

func (s *stateSuite) assertCaCerts(c *gc.C, cloudUUID string, expected []string) {
	db := s.DB()

	rows, err := db.Query("SELECT ca_cert FROM cloud_ca_cert WHERE cloud_uuid = ?", cloudUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	certs := set.NewStrings()
	for rows.Next() {
		var cert string
		err = rows.Scan(&cert)
		c.Assert(err, jc.ErrorIsNil)
		certs.Add(cert)
	}
	c.Assert(rows.Err(), jc.ErrorIsNil)

	c.Check(certs.Values(), jc.SameContents, expected)
}

func regionsFromDbRegions(dbRegions []CloudRegion) []cloud.Region {
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

func (s *stateSuite) assertRegions(c *gc.C, cloudUUID string, expected []cloud.Region) {
	db := s.DB()

	rows, err := db.Query("SELECT name, endpoint, identity_endpoint, storage_endpoint FROM cloud_region WHERE cloud_uuid = ?", cloudUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var dbRegions []CloudRegion
	for rows.Next() {
		var dbRegion CloudRegion
		err = rows.Scan(&dbRegion.Name, &dbRegion.Endpoint, &dbRegion.IdentityEndpoint, &dbRegion.StorageEndpoint)
		c.Assert(err, jc.ErrorIsNil)
		dbRegions = append(dbRegions, dbRegion)
	}
	c.Assert(rows.Err(), jc.ErrorIsNil)

	regions := regionsFromDbRegions(dbRegions)
	c.Check(regions, jc.SameContents, expected)
}

func (s *stateSuite) assertInsertCloud(c *gc.C, st *State, cloud cloud.Cloud) string {
	cloudUUID := uuid.MustNewUUID()
	err := st.CreateCloud(context.Background(), "admin", cloudUUID.String(), cloud)
	c.Assert(err, jc.ErrorIsNil)

	foundCloudUUID := s.assertCloud(c, cloud)
	s.checkPermissionRow(c, permission.AdminAccess, "admin", cloud.Name)
	return foundCloudUUID
}

func (s *stateSuite) TestCreateCloud(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)
}

func (s *stateSuite) TestCreateCloudNewNoRegions(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	cld := testCloud
	cld.Regions = nil
	s.assertInsertCloud(c, st, cld)
}

func (s *stateSuite) TestCreateCloudNewNoCertificates(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	cld := testCloud
	cld.CACertificates = nil
	s.assertInsertCloud(c, st, cld)
}

func (s *stateSuite) TestCreateCloudUpdateExisting(c *gc.C) {
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

	err := st.UpdateCloud(context.Background(), cld)
	c.Assert(err, jc.ErrorIsNil)

	cloudUUID := s.assertCloud(c, cld)
	c.Assert(originalUUID, gc.Equals, cloudUUID)
}

func (s *stateSuite) TestCreateCloudInvalidType(c *gc.C) {
	cld := testCloud
	cld.Type = "mycloud"

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, gc.ErrorMatches, `.* cloud type "mycloud" not valid`)
}

func (s *stateSuite) TestCloudWithEmptyNameFails(c *gc.C) {
	cld := testCloud
	cld.Name = ""

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *stateSuite) TestUpdateCloudDefaults(c *gc.C) {
	cld := testCloud

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudDefaults(context.Background(), cld.Name, map[string]string{
		"foo":        "bar",
		"wallyworld": "peachy",
	}, []string{})
	c.Assert(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, map[string]string{
		"foo":        "bar",
		"wallyworld": "peachy",
	})
}

func (s *stateSuite) TestComplexUpdateCloudDefaults(c *gc.C) {
	cld := testCloud

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudDefaults(context.Background(), cld.Name, map[string]string{
		"foo":        "bar",
		"wallyworld": "peachy",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, map[string]string{
		"foo":        "bar",
		"wallyworld": "peachy",
	})

	err = st.UpdateCloudDefaults(context.Background(), cld.Name, map[string]string{
		"wallyworld": "peachy1",
		"foo2":       "bar2",
	}, []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)

	defaults, err = st.CloudDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, map[string]string{
		"wallyworld": "peachy1",
		"foo2":       "bar2",
	})
}

func (s *stateSuite) TestCloudDefaultsUpdateForNonExistentCloud(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.UpdateCloudDefaults(context.Background(), "noexist", map[string]string{
		"wallyworld": "peachy",
	}, nil)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *stateSuite) TestCloudRegionDefaults(c *gc.C) {
	cld := testCloud

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cld.Name,
		cld.Regions[0].Name,
		map[string]string{
			"foo":        "bar",
			"wallyworld": "peachy",
		},
		nil)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cld.Name,
		cld.Regions[1].Name,
		map[string]string{
			"foo":        "bar1",
			"wallyworld": "peachy2",
		},
		nil)
	c.Assert(err, jc.ErrorIsNil)

	regionDefaults, err := st.CloudAllRegionDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(regionDefaults, jc.DeepEquals, map[string]map[string]string{
		cld.Regions[0].Name: {
			"foo":        "bar",
			"wallyworld": "peachy",
		},
		cld.Regions[1].Name: {
			"foo":        "bar1",
			"wallyworld": "peachy2",
		},
	})
}

func (s *stateSuite) TestCloudRegionDefaultsComplex(c *gc.C) {
	cld := testCloud

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cld.Name,
		cld.Regions[0].Name,
		map[string]string{
			"foo":        "bar",
			"wallyworld": "peachy",
		},
		nil)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cld.Name,
		cld.Regions[1].Name,
		map[string]string{
			"foo":        "bar1",
			"wallyworld": "peachy2",
		},
		nil)
	c.Assert(err, jc.ErrorIsNil)

	regionDefaults, err := st.CloudAllRegionDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(regionDefaults, jc.DeepEquals, map[string]map[string]string{
		cld.Regions[0].Name: {
			"foo":        "bar",
			"wallyworld": "peachy",
		},
		cld.Regions[1].Name: {
			"foo":        "bar1",
			"wallyworld": "peachy2",
		},
	})

	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cld.Name,
		cld.Regions[1].Name,
		map[string]string{
			"wallyworld": "peachy3",
		},
		[]string{"foo"})
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cld.Name,
		cld.Regions[0].Name,
		map[string]string{
			"one":   "two",
			"three": "four",
			"five":  "six",
		},
		nil)
	c.Assert(err, jc.ErrorIsNil)

	regionDefaults, err = st.CloudAllRegionDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(regionDefaults, jc.DeepEquals, map[string]map[string]string{
		cld.Regions[0].Name: {
			"foo":        "bar",
			"wallyworld": "peachy",
			"one":        "two",
			"three":      "four",
			"five":       "six",
		},
		cld.Regions[1].Name: {
			"wallyworld": "peachy3",
		},
	})
}

func (s *stateSuite) TestCloudRegionDefaultsNoExist(c *gc.C) {
	cld := testCloud

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudRegionDefaults(context.Background(), cld.Name, "noexistregion", map[string]string{
		"foo": "bar",
	}, nil)
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	defaults, err := st.CloudAllRegionDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(defaults), gc.Equals, 0)
}

func (s *stateSuite) TestCloudDefaultsRemoval(c *gc.C) {
	cld := testCloud

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudDefaults(context.Background(), cld.Name, map[string]string{
		"foo":        "bar",
		"wallyworld": "peachy",
	}, []string{})
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudDefaults(context.Background(), cld.Name, nil, []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, map[string]string{
		"wallyworld": "peachy",
	})

	err = st.UpdateCloudDefaults(context.Background(), cld.Name, nil, []string{"noexist"})
	c.Assert(err, jc.ErrorIsNil)

	defaults, err = st.CloudDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, map[string]string{
		"wallyworld": "peachy",
	})
}

func (s *stateSuite) TestEmptyCloudDefaults(c *gc.C) {
	cld := testCloud

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(defaults), gc.Equals, 0)
}

// TestNotFoundCloudDefaults is testing what happens if we request a cloud
// defaults for a cloud that doesn't exist. It should result in a
// [clouderrors.NotFound] error.
func (s *stateSuite) TestNotFoundCloudDefaults(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	defaults, err := st.CloudDefaults(context.Background(), "notfound")
	c.Assert(err, jc.ErrorIs, clouderrors.NotFound)
	c.Assert(len(defaults), gc.Equals, 0)
}

func (s *stateSuite) TestCreateCloudInvalidAuthType(c *gc.C) {
	cld := testCloud
	cld.AuthTypes = []cloud.AuthType{"myauth"}

	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, gc.ErrorMatches, `.* auth type "myauth" not valid`)
}

func (s *stateSuite) TestListClouds(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), testCloud2)
	c.Assert(err, jc.ErrorIsNil)

	clouds, err := st.ListClouds(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 2)
	if clouds[0].Name == testCloud.Name {
		c.Assert(clouds[0], jc.DeepEquals, testCloud)
		c.Assert(clouds[1], jc.DeepEquals, testCloud2)
	} else {
		c.Assert(clouds[1], jc.DeepEquals, testCloud)
		c.Assert(clouds[0], jc.DeepEquals, testCloud2)
	}
}

func (s *stateSuite) TestCloudIsControllerCloud(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), testCloud2)
	c.Assert(err, jc.ErrorIsNil)

	clouds, err := st.ListClouds(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 2)

	for _, cloud := range clouds {
		c.Assert(cloud.IsControllerCloud, gc.Equals, false)
	}

	modelUUID := modeltesting.GenModelUUID(c)
	modelSt := modelstate.NewState(s.TxnRunnerFactory())
	modelstatetesting.CreateInternalSecretBackend(c, s.ControllerTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	err = modelSt.Create(
		context.Background(),
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud: testCloud.Name,
			Name:  coremodel.ControllerModelName,
			Owner: user.UUID(s.adminUUID.String()),
			UUID:  modelUUID,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = modelSt.Activate(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)

	clouds, err = st.ListClouds(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 2)

	for _, cloud := range clouds {
		if cloud.Name == testCloud.Name {
			c.Assert(cloud.IsControllerCloud, gc.Equals, true)
		} else {
			c.Assert(cloud.IsControllerCloud, gc.Equals, false)
		}
	}
}

func (s *stateSuite) TestCloud(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), testCloud2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.Cloud(context.Background(), "fluffy3")
	c.Assert(err, jc.ErrorIs, clouderrors.NotFound)

	cloud, err := st.Cloud(context.Background(), "fluffy2")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cloud, jc.DeepEquals, &testCloud2)
}

func (s *stateSuite) TestDeleteCloud(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)

	err := st.DeleteCloud(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.Cloud(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIs, clouderrors.NotFound)

	// Do not find the permission
	row := s.DB().QueryRow(`
SELECT uuid, access_type, grant_to, grant_on
FROM v_permission p
WHERE p.grant_to = ?
`, "fluffy")
	c.Assert(row.Err(), jc.ErrorIsNil)
	var grantOn string
	err = row.Scan(&grantOn)
	c.Assert(err, gc.ErrorMatches, "sql: no rows in result set")
}

func (s *stateSuite) TestDeleteCloudInUse(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)

	credUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `
INSERT INTO cloud_credential (uuid, name, cloud_uuid, auth_type_id, owner_uuid)
SELECT ?, 'default', uuid, 1, ? FROM cloud
WHERE cloud.name = ?
`
		result, err := tx.ExecContext(ctx, stmt, credUUID, s.adminUUID.String(), "fluffy")
		if err != nil {
			return err
		}
		numRows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		c.Assert(numRows, gc.Equals, int64(1))
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	err = st.DeleteCloud(context.Background(), "fluffy")
	c.Assert(err, gc.ErrorMatches, "cannot delete cloud as it is still in use")

	cloud, err := st.Cloud(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud.Name, gc.Equals, "fluffy")
}

type watcherFunc func(namespace, changeValue string, changeMask changestream.ChangeType) (watcher.NotifyWatcher, error)

func (f watcherFunc) NewValueWatcher(
	namespace, changeValue string, changeMask changestream.ChangeType,
) (watcher.NotifyWatcher, error) {
	return f(namespace, changeValue, changeMask)
}

func (s *stateSuite) watcherFunc(c *gc.C, expectedChangeValue string) watcherFunc {
	return func(namespace, changeValue string, changeMask changestream.ChangeType) (watcher.NotifyWatcher, error) {
		c.Assert(namespace, gc.Equals, "cloud")
		c.Assert(changeMask, gc.Equals, changestream.All)
		c.Assert(changeValue, gc.Equals, expectedChangeValue)

		db, err := s.GetWatchableDB(namespace)
		c.Assert(err, jc.ErrorIsNil)

		base := eventsource.NewBaseWatcher(db, loggertesting.WrapCheckLog(c))
		return eventsource.NewValueWatcher(base, namespace, changeValue, changeMask), nil
	}
}

func (s *stateSuite) TestWatchCloudNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	ctx := context.Background()
	_, err := st.WatchCloud(ctx, s.watcherFunc(c, ""), "fluffy")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *stateSuite) TestWatchCloud(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	cloudUUID := uuid.MustNewUUID().String()

	cld := testCloud
	err := st.CreateCloud(context.Background(), "admin", cloudUUID, cld)
	c.Assert(err, jc.ErrorIsNil)

	w, err := st.WatchCloud(context.Background(), s.watcherFunc(c, cloudUUID), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := watchertest.NewNotifyWatcherC(c, w)
	wc.AssertChanges(time.Second) // Initial event.

	cld.Endpoint = "https://endpoint2"
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := st.UpdateCloud(ctx, cld)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	s.AssertChangeStreamIdle(c)

	wc.AssertOneChange()
}

// TestNullCloudType is a regression test to make sure that we don't allow null
// cloud types.
func (s *stateSuite) TestNullCloudType(c *gc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO cloud_type (id, type) VALUES (99, NULL)")
		return err
	})
	c.Assert(jujudb.IsErrConstraintNotNull(err), jc.IsTrue)
}

// TestSetCloudDefaults is testing the happy path for [SetCloudDefaults]
func (s *stateSuite) TestSetCloudDefaults(c *gc.C) {
	cld := testCloud
	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return SetCloudDefaults(ctx, tx, cld.Name, map[string]string{
			"clouddefault": "one",
		})
	})
	c.Check(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cld.Name)
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, map[string]string{
		"clouddefault": "one",
	})
}

// TestSetCloudDefaultsNotFound is asserting that if we try and set cloud
// defaults for a cloud that doesn't exist we get back an error that satisfies
// [clouderrors.NotFound].
func (s *stateSuite) TestSetCloudDefaultsNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return SetCloudDefaults(ctx, tx, "noexist", map[string]string{
			"clouddefault": "one",
		})
	})
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)

	defaults, err := st.CloudDefaults(context.Background(), "noexist")
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
	c.Check(len(defaults), gc.Equals, 0)
}

// TestSetCloudDefaultsOverrides checks that successive calls to
// SetCloudDefaults overrides the previously set values for cloud defaults.
func (s *stateSuite) TestSetCloudDefaultsOverrides(c *gc.C) {
	cld := testCloud
	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return SetCloudDefaults(ctx, tx, cld.Name, map[string]string{
			"clouddefault": "one",
		})
	})
	c.Check(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cld.Name)
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, map[string]string{
		"clouddefault": "one",
	})

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return SetCloudDefaults(ctx, tx, cld.Name, map[string]string{
			"clouddefaultnew": "two",
		})
	})
	c.Check(err, jc.ErrorIsNil)

	defaults, err = st.CloudDefaults(context.Background(), cld.Name)
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, map[string]string{
		"clouddefaultnew": "two",
	})
}

// TestSetCloudDefaultsDelete is testing that if we call [SetCloudDefaults] with
// a empty map of defaults the existing cloud defaults are removed and no
// further actions are taken.
func (s *stateSuite) TestSetCloudDefaultsDelete(c *gc.C) {
	cld := testCloud
	st := NewState(s.TxnRunnerFactory())
	err := st.CreateCloud(context.Background(), "admin", uuid.MustNewUUID().String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return SetCloudDefaults(ctx, tx, cld.Name, map[string]string{
			"clouddefault": "one",
		})
	})
	c.Check(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cld.Name)
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, map[string]string{
		"clouddefault": "one",
	})

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return SetCloudDefaults(ctx, tx, cld.Name, nil)
	})
	c.Check(err, jc.ErrorIsNil)

	defaults, err = st.CloudDefaults(context.Background(), cld.Name)
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(defaults), gc.Equals, 0)
}

// TestCloudSupportsAuthTypeTrue is asserting the happy path that for a valid
// cloud and supported auth type we get back true with no errors.
func (s *stateSuite) TestCloudSupportsAuthTypeTrue(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)

	var supports bool
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		s, err := CloudSupportsAuthType(context.Background(), tx, testCloud.Name, testCloud.AuthTypes[0])
		supports = s
		return err
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(supports, jc.IsTrue)
}

// TestCloudSupportsAuthTypeFalse is asserting the happy path that for a valid
// cloud and a non supported auth type we get back false with no errors.
func (s *stateSuite) TestCloudSupportsAuthTypeFalse(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)

	var supports bool
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		s, err := CloudSupportsAuthType(context.Background(), tx, testCloud.Name, cloud.AuthType("no-exist"))
		supports = s
		return err
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Check(supports, jc.IsFalse)
}

// TestCloudSupportsAuthTypeCloudNotFound is checking to that if we ask if a
// cloud supports an auth type and the cloud doesn't exist we get back a
// [clouderrors.NotFound] error.
func (s *stateSuite) TestCloudSupportsAuthTypeCloudNotFound(c *gc.C) {
	var supports bool
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		s, err := CloudSupportsAuthType(context.Background(), tx, "no-exist", cloud.AuthType("no-exist"))
		supports = s
		return err
	})

	c.Assert(err, jc.ErrorIs, clouderrors.NotFound)
	c.Check(supports, jc.IsFalse)
}

func (s *stateSuite) ensureUser(c *gc.C, userUUID, name, createdByUUID string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, userUUID, name, name, false, createdByUUID, time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user_authentication (user_uuid, disabled)
			VALUES (?, ?)
		`, userUUID, false)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) checkPermissionRow(c *gc.C, access permission.Access, expectedGrantTo, expectedGrantON string) {
	db := s.DB()

	row := db.QueryRow(`
SELECT uuid
FROM user
WHERE name = ?
`, expectedGrantTo)
	c.Assert(row.Err(), jc.ErrorIsNil)
	var userUUID string
	err := row.Scan(&userUUID)
	c.Assert(err, jc.ErrorIsNil)

	// Find the permission
	row = db.QueryRow(`
SELECT uuid, access_type, grant_to, grant_on
FROM v_permission
`)
	c.Assert(row.Err(), jc.ErrorIsNil)
	var (
		accessType, userUuid, grantTo, grantOn string
	)
	err = row.Scan(&userUuid, &accessType, &grantTo, &grantOn)
	c.Assert(err, jc.ErrorIsNil)

	// Verify the permission as expected.
	c.Check(userUuid, gc.Not(gc.Equals), "")
	c.Check(accessType, gc.Equals, string(access))
	c.Check(grantTo, gc.Equals, userUUID)
	c.Check(grantOn, gc.Equals, expectedGrantON)
}

func (s *stateSuite) TestGetCloudForNonExistentID(c *gc.C) {
	fakeID := cloudtesting.GenCloudID(c)
	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetCloudForID(context.Background(), fakeID)
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
}

func (s *stateSuite) TestGetCloudForID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)

	db := s.DB()
	var id corecloud.ID
	err := db.QueryRow("SELECT uuid FROM v_cloud where name = ?", testCloud.Name).Scan(&id)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(err, jc.ErrorIsNil)
	cloud, err := st.GetCloudForID(context.Background(), id)
	c.Check(err, jc.ErrorIsNil)
	c.Check(cloud, jc.DeepEquals, testCloud)
}
