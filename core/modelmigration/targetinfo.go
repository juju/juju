// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import "github.com/juju/names"

// TargetInfo holds the details required to connect to a
// migration's target controller.
//
// TODO(mjs) - Note the similarity to api.Info. It would be nice
// to be able to use api.Info here but state can't import api and
// moving api.Info to live under the core package is too big a project
// to be done right now.
type TargetInfo struct {
	// ControllerTag holds tag for the target controller.
	ControllerTag names.EnvironTag

	// Addrs holds the addresses and ports of the target controller's
	// API servers.
	Addrs []string

	// CACert holds the CA certificate that will be used to validate
	// the target API server's certificate, in PEM format.
	CACert string

	// EntityTag holds the entity to authenticate with to the target
	// controller.
	EntityTag names.Tag

	// Password holds the password to use with TargetEntityTag.
	Password string
}
