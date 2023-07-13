// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"github.com/juju/juju/core/database/schema"
)

// ModelDDL is used to create model databases.
func ModelDDL() *schema.Schema {
	patches := []func() schema.Patch{
		changeLogSchema,
	}

	schema := schema.New()
	for _, fn := range patches {
		schema.Add(fn())
	}
	return schema
}
