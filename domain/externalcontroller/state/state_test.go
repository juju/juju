// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/core/crossmodel"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestRetrieveExternalController(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", "my-controller", "test-cert")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfo, err := st.Controller(c.Context(), "ctrl1")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(controllerInfo.ControllerUUID, tc.Equals, "ctrl1")
	c.Assert(controllerInfo.Alias, tc.Equals, "my-controller")
	c.Assert(controllerInfo.CACert, tc.Equals, "test-cert")
	c.Assert(controllerInfo.Addrs, tc.SameContents, []string{"192.168.1.1", "10.0.0.1"})
}

func (s *stateSuite) TestRetrieveExternalControllerWithoutAddresses(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", "my-controller", "test-cert")`)
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfo, err := st.Controller(c.Context(), "ctrl1")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(controllerInfo.ControllerUUID, tc.Equals, "ctrl1")
	c.Assert(controllerInfo.Alias, tc.Equals, "my-controller")
	c.Assert(controllerInfo.CACert, tc.Equals, "test-cert")
	c.Assert(controllerInfo.Addrs, tc.HasLen, 0)
}

func (s *stateSuite) TestRetrieveExternalControllerWithoutAlias(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller(uuid,ca_cert) VALUES	
("ctrl1", "test-cert")`)
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfo, err := st.Controller(c.Context(), "ctrl1")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(controllerInfo.ControllerUUID, tc.Equals, "ctrl1")
	// Empty Alias => zero value
	c.Assert(controllerInfo.Alias, tc.Equals, "")
	c.Assert(controllerInfo.CACert, tc.Equals, "test-cert")
	c.Assert(controllerInfo.Addrs, tc.HasLen, 0)
}

func (s *stateSuite) TestRetrieveExternalControllerNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Retrieve a not-existent controller.
	_, err := st.Controller(c.Context(), "ctrl1")
	c.Assert(err, tc.ErrorMatches, `external controller "ctrl1" not found`)
}

func (s *stateSuite) TestRetrieveExternalControllerForModel(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", "my-controller", "test-cert")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, tc.ErrorIsNil)
	// Insert a model corresponding to that controller.
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfos, err := st.ControllersForModels(c.Context(), "model1")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(controllerInfos[0].ControllerUUID, tc.Equals, "ctrl1")
	c.Assert(controllerInfos[0].Alias, tc.Equals, "my-controller")
	c.Assert(controllerInfos[0].CACert, tc.Equals, "test-cert")
	c.Assert(controllerInfos[0].Addrs, tc.SameContents, []string{"192.168.1.1", "10.0.0.1"})
}

func (s *stateSuite) TestRetrieveExternalControllerForModelWithoutAddresses(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", "my-controller", "test-cert")`)
	c.Assert(err, tc.ErrorIsNil)
	// Insert a model corresponding to that controller.
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfos, err := st.ControllersForModels(c.Context(), "model1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerInfos, tc.HasLen, 1)

	c.Assert(controllerInfos[0].ControllerUUID, tc.Equals, "ctrl1")
	c.Assert(controllerInfos[0].Alias, tc.Equals, "my-controller")
	c.Assert(controllerInfos[0].CACert, tc.Equals, "test-cert")
	c.Assert(controllerInfos[0].Addrs, tc.HasLen, 0)
}

func (s *stateSuite) TestRetrieveExternalControllerForModelWithoutAlias(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller(uuid,ca_cert) VALUES	
("ctrl1", "test-cert")`)
	c.Assert(err, tc.ErrorIsNil)
	// Insert a model corresponding to that controller.
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)

	// Retrieve the created external controller.
	controllerInfos, err := st.ControllersForModels(c.Context(), "model1")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(controllerInfos[0].ControllerUUID, tc.Equals, "ctrl1")
	// Empty Alias => zero value
	c.Assert(controllerInfos[0].Alias, tc.Equals, "")
	c.Assert(controllerInfos[0].CACert, tc.Equals, "test-cert")
	c.Assert(controllerInfos[0].Addrs, tc.HasLen, 0)
}

func (s *stateSuite) TestUpdateExternalControllerNewData(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ecUUID := uuid.MustNewUUID().String()
	m1 := uuid.MustNewUUID().String()
	ec := crossmodel.ControllerInfo{
		ControllerUUID: ecUUID,
		Alias:          "new-external-controller",
		Addrs:          []string{"10.10.10.10", "192.168.0.9"},
		CACert:         "random-cert-string",
		ModelUUIDs:     []string{m1},
	}

	err := st.UpdateExternalController(c.Context(), ec)
	c.Assert(err, tc.ErrorIsNil)

	db := s.DB()

	// Check the controller record.
	row := db.QueryRow("SELECT alias, ca_cert FROM external_controller WHERE uuid = ?", ecUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var alias, cert string
	err = row.Scan(&alias, &cert)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(alias, tc.Equals, "new-external-controller")
	c.Check(cert, tc.Equals, "random-cert-string")

	// Check the addresses.
	rows, err := db.Query("SELECT address FROM external_controller_address WHERE controller_uuid = ?", ecUUID)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	addrs := set.NewStrings()
	for rows.Next() {
		var addr string
		err := rows.Scan(&addr)
		c.Assert(err, tc.ErrorIsNil)
		addrs.Add(addr)
	}
	c.Check(addrs.Values(), tc.HasLen, 2)
	c.Check(addrs.Contains("10.10.10.10"), tc.IsTrue)
	c.Check(addrs.Contains("192.168.0.9"), tc.IsTrue)

	// Check for the model.
	row = db.QueryRow("SELECT controller_uuid FROM external_model WHERE uuid = ?", m1)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var mc string
	err = row.Scan(&mc)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mc, tc.Equals, ecUUID)
}

func (s *stateSuite) TestUpdateExternalControllerUpsertAndReplace(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	ecUUID := uuid.MustNewUUID().String()
	ec := crossmodel.ControllerInfo{
		ControllerUUID: ecUUID,
		Alias:          "new-external-controller",
		Addrs:          []string{"10.10.10.10", "192.168.0.9"},
		CACert:         "random-cert-string",
	}

	// Initial values.
	err := st.UpdateExternalController(c.Context(), ec)
	c.Assert(err, tc.ErrorIsNil)

	// Now with different alias and addresses.
	ec.Alias = "updated-external-controller"
	ec.Addrs = []string{"10.10.10.10", "192.168.0.10"}

	err = st.UpdateExternalController(c.Context(), ec)
	c.Assert(err, tc.ErrorIsNil)

	db := s.DB()

	// Check the controller record.
	row := db.QueryRow("SELECT alias FROM external_controller WHERE uuid = ?", ecUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var alias string
	err = row.Scan(&alias)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(alias, tc.Equals, "updated-external-controller")

	// Addresses should have one preserved and one replaced.
	rows, err := db.Query("SELECT address FROM external_controller_address WHERE controller_uuid = ?", ecUUID)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	addrs := set.NewStrings()
	for rows.Next() {
		var addr string
		err := rows.Scan(&addr)
		c.Assert(err, tc.ErrorIsNil)
		addrs.Add(addr)
	}
	c.Check(addrs.Values(), tc.HasLen, 2)
	c.Check(addrs.Contains("10.10.10.10"), tc.IsTrue)
	c.Check(addrs.Contains("192.168.0.10"), tc.IsTrue)
}

func (s *stateSuite) TestUpdateExternalControllerUpdateModel(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	m1 := uuid.MustNewUUID().String()
	// This is an existing controller with a model reference.
	ec := crossmodel.ControllerInfo{
		ControllerUUID: uuid.MustNewUUID().String(),
		Alias:          "existing-external-controller",
		CACert:         "random-cert-string",
		ModelUUIDs:     []string{m1},
	}

	err := st.UpdateExternalController(c.Context(), ec)
	c.Assert(err, tc.ErrorIsNil)

	// Now upload a new controller with the same model
	ecUUID := uuid.MustNewUUID().String()
	ec = crossmodel.ControllerInfo{
		ControllerUUID: ecUUID,
		Alias:          "new-external-controller",
		CACert:         "another-random-cert-string",
		ModelUUIDs:     []string{m1},
	}

	err = st.UpdateExternalController(c.Context(), ec)
	c.Assert(err, tc.ErrorIsNil)

	// Check that the model is indicated as being on the new controller.
	row := s.DB().QueryRow("SELECT controller_uuid FROM external_model WHERE uuid = ?", m1)
	c.Assert(row.Err(), tc.ErrorIsNil)

	var mc string
	err = row.Scan(&mc)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mc, tc.Equals, ecUUID)
}

func (s *stateSuite) TestModelsForController(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert a single external controller.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", "my-controller", "test-cert")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, tc.ErrorIsNil)
	// Insert a model corresponding to that controller.
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)

	models, err := st.ModelsForController(c.Context(), "ctrl1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.SameContents, []string{"model1"})
}

func (s *stateSuite) TestModelsForControllerNoRows(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	models, err := st.ModelsForController(c.Context(), "ctrl1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.HasLen, 0)
}

func (s *stateSuite) TestControllersForModels(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert an external controller with one model.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", NULL, "test-cert1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr3", "ctrl1", "10.0.0.2")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)
	// Insert another external controller with two models.
	_, err = db.Exec(`INSERT INTO external_controller VALUES
("ctrl2", "my-controller2", "test-cert2")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr4", "ctrl2", "10.0.0.1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model2", "ctrl2")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model3", "ctrl2")`)
	c.Assert(err, tc.ErrorIsNil)

	controllers, err := st.ControllersForModels(c.Context(), "model1", "model2", "model3", "model2", "model3")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllers, tc.HasLen, 2)

	expectedControllers := []crossmodel.ControllerInfo{
		{
			ControllerUUID: "ctrl1",
			CACert:         "test-cert1",
			Addrs:          []string{"10.0.0.1", "10.0.0.2", "192.168.1.1"},
			ModelUUIDs:     []string{"model1"},
		},
		{
			ControllerUUID: "ctrl2",
			Alias:          "my-controller2",
			CACert:         "test-cert2",
			Addrs:          []string{"10.0.0.1"},
			ModelUUIDs:     []string{"model2", "model3"},
		},
	}
	// Sort the returning controllers which are not order-guaranteed before
	// deep equals assert
	sort.Slice(controllers, func(i, j int) bool { return controllers[i].ControllerUUID < controllers[j].ControllerUUID })
	// Also sort addresses.
	sort.Slice(controllers[0].Addrs, func(i, j int) bool { return controllers[0].Addrs[i] < controllers[0].Addrs[j] })
	// Also sort models.
	sort.Slice(controllers[1].ModelUUIDs, func(i, j int) bool { return controllers[1].ModelUUIDs[i] < controllers[1].ModelUUIDs[j] })
	c.Assert(controllers, tc.DeepEquals, expectedControllers)
}

func (s *stateSuite) TestControllersForModelsOneSingleModel(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert an external controller with one model.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", NULL, "test-cert1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr3", "ctrl1", "10.0.0.2")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model2", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model3", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)

	controllers, err := st.ControllersForModels(c.Context(), "model1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllers[0].Addrs, tc.SameContents, []string{"192.168.1.1", "10.0.0.1", "10.0.0.2"})
	c.Assert(controllers[0].ModelUUIDs, tc.SameContents, []string{"model1", "model2", "model3"})
}

func (s *stateSuite) TestControllersForModelsWithoutModelUUIDs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	db := s.DB()

	// Insert an external controller with one model.
	_, err := db.Exec(`INSERT INTO external_controller VALUES
("ctrl1", NULL, "test-cert1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr1", "ctrl1", "192.168.1.1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr2", "ctrl1", "10.0.0.1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_controller_address VALUES
("addr3", "ctrl1", "10.0.0.2")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model1", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model2", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO external_model VALUES
("model3", "ctrl1")`)
	c.Assert(err, tc.ErrorIsNil)

	controllers, err := st.ControllersForModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllers, tc.HasLen, 0)
}
