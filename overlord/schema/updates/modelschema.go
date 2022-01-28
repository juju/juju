// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package updates

import (
	"context"

	"github.com/juju/errors"
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
	_, err := tx.ExecContext(context.TODO(), `
CREATE TABLE IF NOT EXISTS actions (
	id INTEGER PRIMARY KEY AUTOINCREMENT, 
	receiver TEXT,
	name TEXT,
	parameters_json TEXT,
	operation TEXT
	status TEXT,
	message TEXT,
	enqueued DATETIME,
	started DATETIME,
	completed DATETIME
);
-- The actions logs and results are split into two different tables. This is to 
-- enable the ability to truncate the tables whilst still keeping the actions 
-- intact. 
CREATE TABLE IF NOT EXISTS actions_logs (
	id INTEGER PRIMARY KEY,
	action_id INTEGER,
	output TEXT,
	timestamp DATETIME,
	FOREIGN KEY (action_id)	REFERENCES actions (id)
);
CREATE TABLE IF NOT EXISTS actions_results (
	action_id INTEGER PRIMARY KEY,
	result_json TEXT,
	FOREIGN KEY (action_id)	REFERENCES actions (id)
);
	`,
	)
	return errors.Trace(err)
}
