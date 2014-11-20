// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// db is a surrogate for the proverbial DB layer abstraction that we
// wish we had for juju state.  To that end, the package holds the DB
// implementation-specific details and functionality needed for backups.
// Currently that means mongo-specific details.  However, as a stand-in
// for a future DB layer abstraction, the db package does not expose any
// low-level details publicly.  Thus the backups implementation remains
// oblivious to the underlying DB implementation.
package db

import (
	coreutils "github.com/juju/juju/utils"
)

var runCommand = coreutils.RunCommand
