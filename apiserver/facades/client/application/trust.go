// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/schema"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/internal/configschema"
)

const defaultTrustLevel = false

var trustFields = configschema.Fields{
	application.TrustConfigOptionName: {
		Description: "Does this application have access to trusted credentials",
		Type:        configschema.Tbool,
		Group:       configschema.JujuGroup,
	},
}

var trustDefaults = schema.Defaults{
	application.TrustConfigOptionName: defaultTrustLevel,
}
