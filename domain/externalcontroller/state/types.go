// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
)

// ExternalController represents a single row from the database when
// external_controller is joined with external_controller_address.
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
}

// MigrationControllerInfo holds the details required to connect to a controller.
type MigrationControllerInfo struct {
	// ControllerTag holds tag for the controller.
	ID string `db:"uuid"`

	// Alias holds a (human friendly) alias for the controller.
	Alias sql.NullString `db:"alias"`

	// Addr holds a single host:port value for
	// the external controller's API server.
	Addr sql.NullString `db:"address"`

	// CACert holds the CA certificate that will be used to validate
	// the API server's certificate, in PEM format.
	CACert string `db:"ca_cert"`

	// ModelUUID holds a modelUUID value.
	ModelUUID sql.NullString `db:"model"`
}
