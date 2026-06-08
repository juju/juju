// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/tc"

	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/internal/uuid"
)

// TestGetOfferUUIDsEmpty verifies that a model with no offers returns an empty
// slice and no error.
func (s *migrationSuite) TestGetOfferUUIDsEmpty(c *tc.C) {
	st := New(s.TxnRunnerFactory(), s.modelUUID)

	uuids, err := st.GetOfferUUIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuids, tc.HasLen, 0)
}

// TestGetOfferUUIDs verifies all hosted offer UUIDs are returned.
func (s *migrationSuite) TestGetOfferUUIDs(c *tc.C) {
	st := New(s.TxnRunnerFactory(), s.modelUUID)
	db := s.DB()

	offer1 := uuid.MustNewUUID().String()
	offer2 := uuid.MustNewUUID().String()
	for _, o := range []string{offer1, offer2} {
		_, err := db.ExecContext(c.Context(), `INSERT INTO offer (uuid, name) VALUES (?, ?)`, o, "offer-"+o[:8])
		c.Assert(err, tc.ErrorIsNil)
	}

	uuids, err := st.GetOfferUUIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuids, tc.SameContents, []string{offer1, offer2})
}

// TestGetOffererModelsEmpty verifies that a model with no remote applications
// returns an empty slice and no error.
func (s *migrationSuite) TestGetOffererModelsEmpty(c *tc.C) {
	st := New(s.TxnRunnerFactory(), s.modelUUID)

	models, err := st.GetOffererModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(models, tc.HasLen, 0)
}

// TestGetOffererModels verifies non-null offerer controller/model pairs are
// returned once, even when multiple remote applications reference the same
// third-party offerer model.
func (s *migrationSuite) TestGetOffererModels(c *tc.C) {
	st := New(s.TxnRunnerFactory(), s.modelUUID)
	db := s.DB()

	charmUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		charmUUID, "remote")
	c.Assert(err, tc.ErrorIsNil)

	controllerUUID := uuid.MustNewUUID().String()
	modelUUID := uuid.MustNewUUID().String()
	otherControllerUUID := uuid.MustNewUUID().String()
	otherModelUUID := uuid.MustNewUUID().String()

	addRemoteOfferer := func(name string, controller any, model string) {
		appUUID := uuid.MustNewUUID().String()
		_, err := db.ExecContext(c.Context(),
			"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)",
			appUUID, name, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
		c.Assert(err, tc.ErrorIsNil)
		_, err = db.ExecContext(c.Context(), `
INSERT INTO application_remote_offerer (
    uuid, life_id, application_uuid, offer_uuid, offer_url,
    offerer_controller_uuid, offerer_model_uuid, macaroon
) VALUES (?, 0, ?, ?, ?, ?, ?, 'macaroon')`,
			uuid.MustNewUUID().String(),
			appUUID,
			uuid.MustNewUUID().String(),
			"admin/"+name+".remote",
			controller,
			model,
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	addRemoteOfferer("remote-a", controllerUUID, modelUUID)
	addRemoteOfferer("remote-b", controllerUUID, modelUUID)
	addRemoteOfferer("remote-c", otherControllerUUID, otherModelUUID)
	addRemoteOfferer("remote-local", nil, uuid.MustNewUUID().String())

	models, err := st.GetOffererModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(models, tc.SameContents, []modelmigrationinternal.OffererModel{
		{ControllerUUID: controllerUUID, ModelUUID: modelUUID},
		{ControllerUUID: otherControllerUUID, ModelUUID: otherModelUUID},
	})
}
