// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import "github.com/juju/juju/core/database/schema"

func lifeSchema() schema.Patch {
	return schema.MakePatch(`
CREATE TABLE life (
    id    INT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO life VALUES
    (0, 'alive'), 
    (1, 'dying'),
    (2, 'dead');
`)
}
