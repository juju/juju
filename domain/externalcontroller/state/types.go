// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/crossmodel"
)

// Controller represents a single row from the database when
// external_controller is joined with external_controller_address and
// external_model.
type Controller struct {
	// ID is the controller UUID.
	ID string `db:"uuid"`

	// Alias holds is a human-friendly name for the controller.
	Alias sql.NullString `db:"alias"`

	// CACert holds the certificate to validate the external
	// controller's API server TLS certificate.
	CACert string `db:"ca_cert"`

	// Addr holds a single host:port value for
	// the external controller's API server.
	Addr sql.NullString `db:"address"`

	// ModelUUID holds a modelUUID value.
	ModelUUID sql.NullString `db:"model"`
}

// Address represents a single row from external_controller_address.
type Address struct {
	// ID is the address UUID.
	ID string `db:"uuid"`

	// Addr holds a single host:port value for
	// the external controller's API server.
	Addr string `db:"address"`

	// ControllerUUID holds the controller uuid.
	ControllerUUID string `db:"controller_uuid"`
}

// Model represents a single row from external_model.
type Model struct {
	// ID is the model UUID.
	ID string `db:"uuid"`

	// ControllerUUID holds the controller uuid.
	ControllerUUID string `db:"controller_uuid"`
}

type Controllers []Controller

// uuids is a slice of controller uuids from the database.
type uuids []string

// updateStatements contains the prepared statements used by
// updateExternalControllerTx.
type updateStatements struct {
	// upsertController upserts the controller alias and certificate in
	// external_controller.
	upsertController *sqlair.Statement
	// deleteUnusedAddresses removes addresses not passed in the uuids slice
	// from external_controller_addresses.
	deleteUnusedAddresses *sqlair.Statement
	// insertNewAddresses adds new addresses to external_controller_addresses,
	// leaving existing ones untouched.
	insertNewAddresses *sqlair.Statement
	// upsertModel upserts the model uuids associated with the controller in
	// external_model.
	upsertModel *sqlair.Statement
}

// NewUpdateStatements prepares the SQLair statements used by
// updateExternalControllerTx.
func NewUpdateStatements() (*updateStatements, error) {
	upsertControllerQuery := `
INSERT INTO external_controller (uuid, alias, ca_cert)
VALUES ($Controller.uuid, $Controller.alias, $Controller.ca_cert)
  ON CONFLICT(uuid) DO UPDATE SET alias=excluded.alias, ca_cert=excluded.ca_cert
`
	upsertControllerStmt, err := sqlair.Prepare(upsertControllerQuery, Controller{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q:", upsertControllerQuery)
	}

	deleteUnusedAddressesQuery := `
DELETE FROM external_controller_address
WHERE  controller_uuid = $Controller.uuid
AND    address NOT IN ($uuids[:])
`
	deleteUnusedAddressesStmt, err := sqlair.Prepare(deleteUnusedAddressesQuery, Controller{}, uuids{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q:", deleteUnusedAddressesQuery)
	}

	insertNewAddressesQuery := `
INSERT INTO external_controller_address (uuid, controller_uuid, address)
VALUES ($Address.uuid, $Address.controller_uuid, $Address.address)
  ON CONFLICT(controller_uuid, address) DO NOTHING
`
	insertNewAddressesStmt, err := sqlair.Prepare(insertNewAddressesQuery, Address{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q:", insertNewAddressesQuery)
	}

	// TODO (manadart 2023-05-13): Check current implementation and see if
	// we need to delete models as we do for addresses, or whether this
	// (additive) approach is what we have now.
	upsertModelQuery := `
INSERT INTO external_model (uuid, controller_uuid)
VALUES ($Model.uuid, $Model.controller_uuid)
  ON CONFLICT(uuid) DO UPDATE SET controller_uuid=excluded.controller_uuid
`
	upsertModelStmt, err := sqlair.Prepare(upsertModelQuery, Model{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q:", upsertModelQuery)
	}

	return &updateStatements{
		upsertController:      upsertControllerStmt,
		deleteUnusedAddresses: deleteUnusedAddressesStmt,
		insertNewAddresses:    insertNewAddressesStmt,
		upsertModel:           upsertModelStmt,
	}, nil
}

// ToControllerInfo Controllers to a slice of crossmodel.ControllerInfo
// structs. This method makes sure only unique models and addresses are mapped
// and flattens them into each controller.
// Order of addresses, models and the resulting crossmodel.ControllerInfo
// elements are not guaranteed, no sorting is applied.
func (e Controllers) ToControllerInfo() []crossmodel.ControllerInfo {
	var resultControllers []crossmodel.ControllerInfo
	// Prepare structs for unique models and addresses for each
	// controller.
	uniqueModelUUIDs := make(map[string]map[string]string)
	uniqueAddresses := make(map[string]map[string]string)
	uniqueControllers := make(map[string]crossmodel.ControllerInfo)

	for _, controller := range e {
		uniqueControllers[controller.ID] = crossmodel.ControllerInfo{
			ControllerTag: names.NewControllerTag(controller.ID),
			CACert:        controller.CACert,
			Alias:         controller.Alias.String,
		}

		// Each row contains only one address, so it's safe
		// to access the only possible (nullable) value.
		if controller.Addr.Valid {
			if _, ok := uniqueAddresses[controller.ID]; !ok {
				uniqueAddresses[controller.ID] = make(map[string]string)
			}
			uniqueAddresses[controller.ID][controller.Addr.String] = controller.Addr.String
		}
		// Each row contains only one model, so it's safe
		// to access the only possible (nullable) value.
		if controller.ModelUUID.Valid {
			if _, ok := uniqueModelUUIDs[controller.ID]; !ok {
				uniqueModelUUIDs[controller.ID] = make(map[string]string)
			}
			uniqueModelUUIDs[controller.ID][controller.ModelUUID.String] = controller.ModelUUID.String
		}
	}

	// Iterate through every controller and flatten its models and
	// addresses.
	for controllerID, controller := range uniqueControllers {
		var modelUUIDs []string
		for _, modelUUID := range uniqueModelUUIDs[controllerID] {
			modelUUIDs = append(modelUUIDs, modelUUID)
		}
		controller.ModelUUIDs = modelUUIDs

		var addresses []string
		for _, modelUUID := range uniqueAddresses[controllerID] {
			addresses = append(addresses, modelUUID)
		}
		controller.Addrs = addresses

		resultControllers = append(resultControllers, controller)
	}

	return resultControllers
}
