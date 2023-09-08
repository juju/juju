// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	ctx "context"
	"database/sql"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

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
		CACertificates: []string{"cert1", "cert2"},
		SkipTLSVerify:  true,
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
		CACertificates: []string{"cert1", "cert3"},
		SkipTLSVerify:  false,
	}
)

func (s *stateSuite) assertCloud(c *gc.C, cloud cloud.Cloud) string {
	db := s.DB()

	// Check the cloud record.
	row := db.QueryRow("SELECT uuid, name, cloud_type_id, endpoint, identity_endpoint, storage_endpoint, skip_tls_verify FROM cloud WHERE name = ?", "fluffy")
	c.Assert(row.Err(), jc.ErrorIsNil)

	var dbCloud Cloud
	err := row.Scan(&dbCloud.ID, &dbCloud.Name, &dbCloud.TypeID, &dbCloud.Endpoint, &dbCloud.IdentityEndpoint, &dbCloud.StorageEndpoint, &dbCloud.SkipTLSVerify)
	c.Assert(err, jc.ErrorIsNil)
	if !utils.IsValidUUIDString(dbCloud.ID) {
		c.Fatalf("invalid cloud uuid %q", dbCloud.ID)
	}
	c.Check(dbCloud.Name, gc.Equals, cloud.Name)
	c.Check(dbCloud.TypeID, gc.Equals, 5) // 5 is "ec2"
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
	err := st.UpsertCloud(ctx.Background(), cloud)
	c.Assert(err, jc.ErrorIsNil)

	cloudUUID := s.assertCloud(c, cloud)
	return cloudUUID
}

func (s *stateSuite) TestUpsertCloudNew(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)
}

func (s *stateSuite) TestUpsertCloudNewNoRegions(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	cld := testCloud
	cld.Regions = nil
	s.assertInsertCloud(c, st, cld)
}

func (s *stateSuite) TestUpsertCloudNewNoCertificates(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	cld := testCloud
	cld.CACertificates = nil
	s.assertInsertCloud(c, st, cld)
}

func (s *stateSuite) TestUpsertCloudUpdateExisting(c *gc.C) {
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
		CACertificates: []string{"cert1", "cert3"},
		SkipTLSVerify:  false,
	}

	err := st.UpsertCloud(ctx.Background(), cld)
	c.Assert(err, jc.ErrorIsNil)

	cloudUUID := s.assertCloud(c, cld)
	c.Assert(originalUUID, gc.Equals, cloudUUID)
}

func (s *stateSuite) TestUpsertCloudInvalidType(c *gc.C) {
	cld := testCloud
	cld.Type = "mycloud"

	st := NewState(s.TxnRunnerFactory())
	err := st.UpsertCloud(ctx.Background(), cld)
	c.Assert(err, gc.ErrorMatches, `.* cloud type "mycloud" not valid`)
}

func (s *stateSuite) TestCloudWithEmptyNameFails(c *gc.C) {
	cld := testCloud
	cld.Name = ""

	st := NewState(s.TxnRunnerFactory())
	err := st.UpsertCloud(ctx.Background(), cld)
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue)
}

func (s *stateSuite) TestUpdateCloudDefaults(c *gc.C) {
	cld := testCloud

	st := NewState(s.TxnRunnerFactory())
	err := st.UpsertCloud(ctx.Background(), cld)
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
	err := st.UpsertCloud(ctx.Background(), cld)
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
	c.Assert(errors.Is(err, errors.NotFound), jc.IsTrue)
}

func (s *stateSuite) TestCloudRegionDefaults(c *gc.C) {
	cld := testCloud

	st := NewState(s.TxnRunnerFactory())
	err := st.UpsertCloud(ctx.Background(), cld)
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
	err := st.UpsertCloud(ctx.Background(), cld)
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
	err := st.UpsertCloud(context.Background(), cld)
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudRegionDefaults(context.Background(), cld.Name, "noexistregion", map[string]string{
		"foo": "bar",
	}, nil)
	c.Assert(errors.Is(err, errors.NotFound), jc.IsTrue)

	defaults, err := st.CloudAllRegionDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(defaults), gc.Equals, 0)
}

func (s *stateSuite) TestCloudDefaultsRemoval(c *gc.C) {
	cld := testCloud

	st := NewState(s.TxnRunnerFactory())
	err := st.UpsertCloud(ctx.Background(), cld)
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
	err := st.UpsertCloud(ctx.Background(), cld)
	c.Assert(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cld.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(defaults), gc.Equals, 0)
}

func (s *stateSuite) TestNonFoundCloudDefaults(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	defaults, err := st.CloudDefaults(context.Background(), "notfound")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(defaults), gc.Equals, 0)
}

func (s *stateSuite) TestUpsertCloudInvalidAuthType(c *gc.C) {
	cld := testCloud
	cld.AuthTypes = []cloud.AuthType{"myauth"}

	st := NewState(s.TxnRunnerFactory())
	err := st.UpsertCloud(ctx.Background(), cld)
	c.Assert(err, gc.ErrorMatches, `.* auth type "myauth" not valid`)
}

func (s *stateSuite) TestListClouds(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.UpsertCloud(ctx.Background(), testCloud)
	c.Assert(err, jc.ErrorIsNil)
	err = st.UpsertCloud(ctx.Background(), testCloud2)
	c.Assert(err, jc.ErrorIsNil)

	clouds, err := st.ListClouds(ctx.Background(), "")
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

func (s *stateSuite) TestListCloudsFilter(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.UpsertCloud(ctx.Background(), testCloud)
	c.Assert(err, jc.ErrorIsNil)
	err = st.UpsertCloud(ctx.Background(), testCloud2)
	c.Assert(err, jc.ErrorIsNil)

	clouds, err := st.ListClouds(ctx.Background(), "fluffy3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 0)

	clouds, err = st.ListClouds(ctx.Background(), "fluffy2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, jc.DeepEquals, []cloud.Cloud{testCloud2})
}

func (s *stateSuite) TestDeleteCloud(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)

	err := st.DeleteCloud(ctx.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)

	clouds, err := st.ListClouds(ctx.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 0)
}

func (s *stateSuite) TestDeleteCloudInUse(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	s.assertInsertCloud(c, st, testCloud)

	cred_uuid := utils.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := fmt.Sprintf(`
INSERT INTO cloud_credential (uuid, name, cloud_uuid, auth_type_id, owner_uuid)
SELECT '%s', 'default', uuid, 1, 'fred' FROM cloud
WHERE cloud.name = ?
`, cred_uuid)
		result, err := tx.ExecContext(ctx, stmt, "fluffy")
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

	err = st.DeleteCloud(ctx.Background(), "fluffy")
	c.Assert(err, gc.ErrorMatches, "cannot delete cloud as it is still in use")

	clouds, err := st.ListClouds(ctx.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 1)
	c.Assert(clouds[0].Name, gc.Equals, "fluffy")
}
