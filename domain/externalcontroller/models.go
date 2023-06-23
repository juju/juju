// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontroller

import "github.com/juju/names/v4"

// MigrationControllerInfo holds the details required to connect to a controller.
type MigrationControllerInfo struct {
	// ControllerTag holds tag for the controller.
	ControllerTag names.ControllerTag

	// Alias holds a (human friendly) alias for the controller.
	Alias string

	// Addrs holds the addresses and ports of the controller's API servers.
	Addrs []string

	// CACert holds the CA certificate that will be used to validate
	// the API server's certificate, in PEM format.
	CACert string

	// ModelUUIDs holds the UUIDs of the models hosted on this controller.
	ModelUUIDs []string
}
