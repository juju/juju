// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/names/v5"

	"github.com/juju/juju/core/crossmodel"
)

// ExternalController represents a single row from the database when
// external_controller is joined with external_controller_address and
// external_model.
type ExternalController struct {
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

type ExternalControllers []ExternalController

// ToControllerInfo ExternalControllers to a slice of crossmodel.ControllerInfo
// structs. This method makes sure only unique models and addresses are mapped
// and flattens them into each controller.
// Order of addresses, models and the resulting crossmodel.ControllerInfo
// elements are not guaranteed, no sorting is applied.
func (e ExternalControllers) ToControllerInfo() []crossmodel.ControllerInfo {
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
