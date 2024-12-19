// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/schema"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/internal/environschema"
)

const defaultTrustLevel = false

var trustFields = environschema.Fields{
	application.TrustConfigOptionName: {
		Description: "Does this application have access to trusted credentials",
		Type:        environschema.Tbool,
		Group:       environschema.JujuGroup,
	},
}

var trustDefaults = schema.Defaults{
	application.TrustConfigOptionName: defaultTrustLevel,
}
