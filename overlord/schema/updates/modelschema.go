// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package updates

import (
	"github.com/juju/juju/overlord/schema"
	"github.com/juju/juju/overlord/state"
)

// ModelSchema returns the current model database schema.
func ModelSchema() *schema.Schema {
	return schema.New(modelUpdates)
}

var modelUpdates = []schema.Update{
	modelUpdateFromV0,
}

func modelUpdateFromV0(tx state.Txn) error {
	return nil
}
