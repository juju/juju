// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package backupsweeper provides a worker that periodically removes
// one-shot backup download archives that have outlived their retention
// window. Archives are staged by the Backups facade under server-minted
// ids in the one-shot download directory of the backup dir; they are
// normally removed when downloaded, and the sweeper is the backstop that
// removes them when a download never happens or never completes.
package backupsweeper
