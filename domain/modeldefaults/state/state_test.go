// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	corecloud "github.com/juju/juju/core/cloud"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstatetesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite

	modelUUID model.UUID
}

var _ = gc.Suite(&stateSuite{})

func (m *stateSuite) SetUpTest(c *gc.C) {
	m.ControllerSuite.SetUpTest(c)
	m.modelUUID = modelstatetesting.CreateTestModel(c, m.TxnRunnerFactory(), "model-defaults")
}

// TestModelMetadataDefaults is asserting the happy path of model metadata
// defaults.
func (s *stateSuite) TestModelMetadataDefaults(c *gc.C) {
	uuid := modelstatetesting.CreateTestModel(c, s.TxnRunnerFactory(), "test")
	st := NewState(s.TxnRunnerFactory())
	defaults, err := st.ModelMetadataDefaults(context.Background(), uuid)
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, map[string]string{
		config.NameKey: "test",
		config.UUIDKey: uuid.String(),
		config.TypeKey: "ec2",
	})
}

// TestModelMetadataDefaultsNoModel is asserting that if we ask for the model
// metadata defaults for a model that doesn't exist we get back a
// [modelerrors.NotFound] error.
func (s *stateSuite) TestModelMetadataDefaultsNoModel(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	st := NewState(s.TxnRunnerFactory())
	defaults, err := st.ModelMetadataDefaults(context.Background(), uuid)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
	c.Check(len(defaults), gc.Equals, 0)
}

var (
	testCloud = cloud.Cloud{
		Name:      "fluffy",
		Type:      "ec2",
		AuthTypes: []cloud.AuthType{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Endpoint:  "https://endpoint",
		Regions: []cloud.Region{{
			Name: "region1",
		}, {
			Name: "region2",
		}},
	}
)

func (s *stateSuite) TestUpdateCloudDefaults(c *gc.C) {
	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	cloudUUID := corecloud.UUID(uuid.MustNewUUID().String())
	err := cloudSt.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.UpdateCloudDefaults(context.Background(), cloudUUID, map[string]string{
		"foo":        "bar",
		"wallyworld": "peachy",
	})
	c.Assert(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cloudUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, map[string]string{
		"foo":        "bar",
		"wallyworld": "peachy",
	})
}

func (s *stateSuite) TestComplexUpdateCloudDefaults(c *gc.C) {
	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	cloudUUID := corecloud.UUID(uuid.MustNewUUID().String())
	err := cloudSt.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.UpdateCloudDefaults(context.Background(), cloudUUID, map[string]string{
		"foo":        "bar",
		"wallyworld": "peachy",
	})
	c.Assert(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cloudUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, map[string]string{
		"foo":        "bar",
		"wallyworld": "peachy",
	})

	err = st.UpdateCloudDefaults(context.Background(), cloudUUID, map[string]string{
		"wallyworld": "peachy1",
		"foo2":       "bar2",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = st.DeleteCloudDefaults(context.Background(), cloudUUID, []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)

	defaults, err = st.CloudDefaults(context.Background(), cloudUUID)
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
	})
	c.Assert(err, jc.ErrorIs, clouderrors.NotFound)
}

func (s *stateSuite) TestCloudRegionDefaults(c *gc.C) {
	cld := testCloud

	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	cloudUUID := corecloud.UUID(uuid.MustNewUUID().String())
	err := cloudSt.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cloudUUID,
		cld.Regions[0].Name,
		map[string]string{
			"foo":        "bar",
			"wallyworld": "peachy",
		})
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cloudUUID,
		cld.Regions[1].Name,
		map[string]string{
			"foo":        "bar1",
			"wallyworld": "peachy2",
		})
	c.Assert(err, jc.ErrorIsNil)

	regionDefaults, err := st.CloudAllRegionDefaults(context.Background(), cloudUUID)
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

	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	cloudUUID := corecloud.UUID(uuid.MustNewUUID().String())
	err := cloudSt.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), cld)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cloudUUID,
		cld.Regions[0].Name,
		map[string]string{
			"foo":        "bar",
			"wallyworld": "peachy",
		})
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cloudUUID,
		cld.Regions[1].Name,
		map[string]string{
			"foo":        "bar1",
			"wallyworld": "peachy2",
		})
	c.Assert(err, jc.ErrorIsNil)

	regionDefaults, err := st.CloudAllRegionDefaults(context.Background(), cloudUUID)
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
		cloudUUID,
		cld.Regions[1].Name,
		map[string]string{
			"wallyworld": "peachy3",
		})
	c.Assert(err, jc.ErrorIsNil)

	err = st.DeleteCloudRegionDefaults(
		context.Background(),
		cloudUUID,
		cld.Regions[1].Name,
		[]string{"foo"})
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateCloudRegionDefaults(
		context.Background(),
		cloudUUID,
		cld.Regions[0].Name,
		map[string]string{
			"one":   "two",
			"three": "four",
			"five":  "six",
		})
	c.Assert(err, jc.ErrorIsNil)

	regionDefaults, err = st.CloudAllRegionDefaults(context.Background(), cloudUUID)
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
	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	cloudUUID := corecloud.UUID(uuid.MustNewUUID().String())
	err := cloudSt.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.UpdateCloudRegionDefaults(context.Background(), cloudUUID, "noexistregion", map[string]string{
		"foo": "bar",
	})
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	defaults, err := st.CloudAllRegionDefaults(context.Background(), cloudUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(defaults), gc.Equals, 0)
}

func (s *stateSuite) TestCloudDefaultsRemoval(c *gc.C) {
	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	cloudUUID := corecloud.UUID(uuid.MustNewUUID().String())
	err := cloudSt.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.UpdateCloudDefaults(context.Background(), cloudUUID, map[string]string{
		"foo":        "bar",
		"wallyworld": "peachy",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = st.DeleteCloudDefaults(context.Background(), cloudUUID, []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)

	defaults, err := st.CloudDefaults(context.Background(), cloudUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, map[string]string{
		"wallyworld": "peachy",
	})

	err = st.DeleteCloudDefaults(context.Background(), cloudUUID, []string{"noexist"})
	c.Assert(err, jc.ErrorIsNil)

	defaults, err = st.CloudDefaults(context.Background(), cloudUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, map[string]string{
		"wallyworld": "peachy",
	})
}

func (s *stateSuite) TestEmptyCloudDefaults(c *gc.C) {
	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	cloudUUID := corecloud.UUID(uuid.MustNewUUID().String())
	err := cloudSt.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	defaults, err := st.CloudDefaults(context.Background(), cloudUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(defaults), gc.Equals, 0)
}

func (s *stateSuite) TestGetCloudUUID(c *gc.C) {
	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	cloudUUID := corecloud.UUID(uuid.MustNewUUID().String())
	err := cloudSt.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	uuid, err := st.GetCloudUUID(context.Background(), testCloud.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid.String(), gc.Equals, cloudUUID.String())
}

func (s *stateSuite) TestGetModelCloudDetails(c *gc.C) {
	modelUUID := modelstatetesting.CreateTestModel(c, s.TxnRunnerFactory(), "test")
	var cloudUUID string
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(context.Background(), "SELECT uuid FROM cloud WHERE name = ?", "test").Scan(&cloudUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	gotCloudUUID, region, err := st.GetModelCloudDetails(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotCloudUUID.String(), gc.Equals, cloudUUID)
	c.Assert(region, gc.Equals, "test-region")
}

// TestGetModelCloudType asserts that the cloud type for a created model is
// correct.
func (s *stateSuite) TestGetCloudType(c *gc.C) {
	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	cloudUUID := corecloud.UUID(uuid.MustNewUUID().String())
	err := cloudSt.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), testCloud)
	c.Assert(err, jc.ErrorIsNil)

	ct, err := NewState(s.TxnRunnerFactory()).CloudType(
		context.Background(), cloudUUID,
	)
	c.Check(err, jc.ErrorIsNil)
	c.Check(ct, gc.Equals, "ec2")
}

// TestGetModelCloudTypModelNotFound is asserting that when no model exists we
// get back a [modelerrors.NotFound] error when querying for a model's cloud
// type.
func (s *stateSuite) TestGetCloudTypeCloudNotFound(c *gc.C) {
	cloudUUID := corecloud.UUID(uuid.MustNewUUID().String())
	_, err := NewState(s.TxnRunnerFactory()).CloudType(
		context.Background(), cloudUUID,
	)
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
}
