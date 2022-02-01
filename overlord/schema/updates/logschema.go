// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package updates

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/overlord/schema"
	"github.com/juju/juju/overlord/state"
)

// LogSchema returns the current log database schema.
func LogSchema() *schema.Schema {
	return schema.New(logUpdates)
}

var logUpdates = []schema.Update{
	logUpdateFromV0,
}

func logUpdateFromV0(tx state.Txn) error {
	_, err := tx.ExecContext(context.TODO(), `
CREATE TABLE IF NOT EXISTS logs(
	ts DATETIME,
	entity TEXT,
	version TEXT,
	module TEXT,
	location TEXT,
	level INTEGER,
	message TEXT,
	labels TEXT
);
`,
	)
	return errors.Trace(err)
}
