// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/schema"
)

type fieldsFunc func() (schema.Fields, schema.Defaults)
