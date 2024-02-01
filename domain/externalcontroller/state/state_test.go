// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	ctx "context"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/crossmodel"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestRetrieveExternalController(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", "my-controller", "test-cert")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfo, err := st.Controller(ctx.Background(), "ctrl1")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(controllerInfo.ControllerTag.Id(), gc.Equals, "ctrl1")
	c.Assert(controllerInfo.Alias, gc.Equals, "my-controller")
	c.Assert(controllerInfo.CACert, gc.Equals, "test-cert")
	c.Assert(controllerInfo.Addrs, jc.SameContents, []string{"192.168.1.1", "10.0.0.1"})
}

func (s *stateSuite) TestRetrieveExternalControllerWithoutAddresses(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", "my-controller", "test-cert")`)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfo, err := st.Controller(ctx.Background(), "ctrl1")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(controllerInfo.ControllerTag.Id(), gc.Equals, "ctrl1")
	c.Assert(controllerInfo.Alias, gc.Equals, "my-controller")
	c.Assert(controllerInfo.CACert, gc.Equals, "test-cert")
	c.Assert(controllerInfo.Addrs, gc.HasLen, 0)
}

func (s *stateSuite) TestRetrieveExternalControllerWithoutAlias(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller(uuid,ca_cert) VALUES	
("ctrl1", "test-cert")`)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfo, err := st.Controller(ctx.Background(), "ctrl1")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(controllerInfo.ControllerTag.Id(), gc.Equals, "ctrl1")
	// Empty Alias => zero value
	c.Assert(controllerInfo.Alias, gc.Equals, "")
	c.Assert(controllerInfo.CACert, gc.Equals, "test-cert")
	c.Assert(controllerInfo.Addrs, gc.HasLen, 0)
}

func (s *stateSuite) TestRetrieveExternalControllerNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Retrieve a not-existent controller.
	_, err := st.Controller(ctx.Background(), "ctrl1")
	c.Assert(err, gc.ErrorMatches, `external controller "ctrl1" not found`)
}

func (s *stateSuite) TestRetrieveExternalControllerForModel(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", "my-controller", "test-cert")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, jc.ErrorIsNil)
	// Insert a model corresponding to that controller.
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfos, err := st.ControllersForModels(ctx.Background(), "model1")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(controllerInfos[0].ControllerTag.Id(), gc.Equals, "ctrl1")
	c.Assert(controllerInfos[0].Alias, gc.Equals, "my-controller")
	c.Assert(controllerInfos[0].CACert, gc.Equals, "test-cert")
	c.Assert(controllerInfos[0].Addrs, jc.SameContents, []string{"192.168.1.1", "10.0.0.1"})
}

func (s *stateSuite) TestRetrieveExternalControllerForModelWithoutAddresses(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", "my-controller", "test-cert")`)
	c.Assert(err, jc.ErrorIsNil)
	// Insert a model corresponding to that controller.
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfos, err := st.ControllersForModels(ctx.Background(), "model1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerInfos, gc.HasLen, 1)

	c.Assert(controllerInfos[0].ControllerTag.Id(), gc.Equals, "ctrl1")
	c.Assert(controllerInfos[0].Alias, gc.Equals, "my-controller")
	c.Assert(controllerInfos[0].CACert, gc.Equals, "test-cert")
	c.Assert(controllerInfos[0].Addrs, gc.HasLen, 0)
}

func (s *stateSuite) TestRetrieveExternalControllerForModelWithoutAlias(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller(uuid,ca_cert) VALUES	
("ctrl1", "test-cert")`)
	c.Assert(err, jc.ErrorIsNil)
	// Insert a model corresponding to that controller.
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfos, err := st.ControllersForModels(ctx.Background(), "model1")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(controllerInfos[0].ControllerTag.Id(), gc.Equals, "ctrl1")
	// Empty Alias => zero value
	c.Assert(controllerInfos[0].Alias, gc.Equals, "")
	c.Assert(controllerInfos[0].CACert, gc.Equals, "test-cert")
	c.Assert(controllerInfos[0].Addrs, gc.HasLen, 0)
}

func (s *stateSuite) TestUpdateExternalControllerNewData(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	ecUUID := utils.MustNewUUID().String()
	m1 := utils.MustNewUUID().String()
	ec := crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(ecUUID),
		Alias:         "new-external-controller",
		Addrs:         []string{"10.10.10.10", "192.168.0.9"},
		CACert:        "random-cert-string",
		ModelUUIDs:    []string{m1},
	}

	err := st.UpdateExternalController(ctx.Background(), ec)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()

	// Check the controller record.
	row := db.QueryRow("SELECT alias, ca_cert FROM external_controller WHERE uuid = ?", ecUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var alias, cert string
	err = row.Scan(&alias, &cert)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(alias, gc.Equals, "new-external-controller")
	c.Check(cert, gc.Equals, "random-cert-string")

	// Check the addresses.
	rows, err := db.Query("SELECT address FROM external_controller_address WHERE controller_uuid = ?", ecUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	addrs := set.NewStrings()
	for rows.Next() {
		var addr string
		err := rows.Scan(&addr)
		c.Assert(err, jc.ErrorIsNil)
		addrs.Add(addr)
	}
	c.Check(addrs.Values(), gc.HasLen, 2)
	c.Check(addrs.Contains("10.10.10.10"), jc.IsTrue)
	c.Check(addrs.Contains("192.168.0.9"), jc.IsTrue)

	// Check for the model.
	row = db.QueryRow("SELECT controller_uuid FROM external_model WHERE uuid = ?", m1)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var mc string
	err = row.Scan(&mc)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mc, gc.Equals, ecUUID)
}

func (s *stateSuite) TestUpdateExternalControllerUpsertAndReplace(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	ecUUID := utils.MustNewUUID().String()
	ec := crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(ecUUID),
		Alias:         "new-external-controller",
		Addrs:         []string{"10.10.10.10", "192.168.0.9"},
		CACert:        "random-cert-string",
	}

	// Initial values.
	err := st.UpdateExternalController(ctx.Background(), ec)
	c.Assert(err, jc.ErrorIsNil)

	// Now with different alias and addresses.
	ec.Alias = "updated-external-controller"
	ec.Addrs = []string{"10.10.10.10", "192.168.0.10"}

	err = st.UpdateExternalController(ctx.Background(), ec)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()

	// Check the controller record.
	row := db.QueryRow("SELECT alias FROM external_controller WHERE uuid = ?", ecUUID)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var alias string
	err = row.Scan(&alias)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(alias, gc.Equals, "updated-external-controller")

	// Addresses should have one preserved and one replaced.
	rows, err := db.Query("SELECT address FROM external_controller_address WHERE controller_uuid = ?", ecUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	addrs := set.NewStrings()
	for rows.Next() {
		var addr string
		err := rows.Scan(&addr)
		c.Assert(err, jc.ErrorIsNil)
		addrs.Add(addr)
	}
	c.Check(addrs.Values(), gc.HasLen, 2)
	c.Check(addrs.Contains("10.10.10.10"), jc.IsTrue)
	c.Check(addrs.Contains("192.168.0.10"), jc.IsTrue)
}

func (s *stateSuite) TestUpdateExternalControllerUpdateModel(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	m1 := utils.MustNewUUID().String()
	// This is an existing controller with a model reference.
	ec := crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(utils.MustNewUUID().String()),
		Alias:         "existing-external-controller",
		CACert:        "random-cert-string",
		ModelUUIDs:    []string{m1},
	}

	err := st.UpdateExternalController(ctx.Background(), ec)
	c.Assert(err, jc.ErrorIsNil)

	// Now upload a new controller with the same model
	ecUUID := utils.MustNewUUID().String()
	ec = crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(ecUUID),
		Alias:         "new-external-controller",
		CACert:        "another-random-cert-string",
		ModelUUIDs:    []string{m1},
	}

	err = st.UpdateExternalController(ctx.Background(), ec)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the model is indicated as being on the new controller.
	row := s.DB().QueryRow("SELECT controller_uuid FROM external_model WHERE uuid = ?", m1)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var mc string
	err = row.Scan(&mc)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mc, gc.Equals, ecUUID)
}

func (s *stateSuite) TestModelsForController(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", "my-controller", "test-cert")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, jc.ErrorIsNil)
	// Insert a model corresponding to that controller.
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)

	models, err := st.ModelsForController(ctx.Background(), "ctrl1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.SameContents, []string{"model1"})
}

func (s *stateSuite) TestControllersForModels(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert an external controller with one model.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", NULL, "test-cert1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr3", "ctrl1", "10.0.0.2")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)
	// Insert another external controller with two models.
	_, err = db.Exec(`INSERT INTO external_controller VALUES
("ctrl2", "my-controller2", "test-cert2")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr4", "ctrl2", "10.0.0.1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model2", "ctrl2")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model3", "ctrl2")`)
	c.Assert(err, jc.ErrorIsNil)

	controllers, err := st.ControllersForModels(ctx.Background(), "model1", "model2", "model3", "model2", "model3")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllers, gc.HasLen, 2)

	expectedControllers := []crossmodel.ControllerInfo{
		{
			ControllerTag: names.NewControllerTag("ctrl1"),
			CACert:        "test-cert1",
			Addrs:         []string{"10.0.0.1", "10.0.0.2", "192.168.1.1"},
			ModelUUIDs:    []string{"model1"},
		},
		{
			ControllerTag: names.NewControllerTag("ctrl2"),
			Alias:         "my-controller2",
			CACert:        "test-cert2",
			Addrs:         []string{"10.0.0.1"},
			ModelUUIDs:    []string{"model2", "model3"},
		},
	}
	// Sort the returning controllers which are not order-guaranteed before
	// deep equals assert
	sort.Slice(controllers, func(i, j int) bool { return controllers[i].ControllerTag.Id() < controllers[j].ControllerTag.Id() })
	// Also sort addresses.
	sort.Slice(controllers[0].Addrs, func(i, j int) bool { return controllers[0].Addrs[i] < controllers[0].Addrs[j] })
	// Also sort models.
	sort.Slice(controllers[1].ModelUUIDs, func(i, j int) bool { return controllers[1].ModelUUIDs[i] < controllers[1].ModelUUIDs[j] })
	c.Assert(controllers, gc.DeepEquals, expectedControllers)
}

func (s *stateSuite) TestControllersForModelsOneSingleModel(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert an external controller with one model.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", NULL, "test-cert1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr3", "ctrl1", "10.0.0.2")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model2", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model3", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)

	controllers, err := st.ControllersForModels(ctx.Background(), "model1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllers[0].Addrs, jc.SameContents, []string{"192.168.1.1", "10.0.0.1", "10.0.0.2"})
	c.Assert(controllers[0].ModelUUIDs, jc.SameContents, []string{"model1", "model2", "model3"})
}

func (s *stateSuite) TestControllersForModelsWithoutModelUUIDs(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert an external controller with one model.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", NULL, "test-cert1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr3", "ctrl1", "10.0.0.2")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model2", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model3", "ctrl1")`)
	c.Assert(err, jc.ErrorIsNil)

	controllers, err := st.ControllersForModels(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllers, gc.HasLen, 0)
}
