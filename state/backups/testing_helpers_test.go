// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"

	"github.com/juju/juju/state/backups/db"
)

func ExposeCreateResult(result *createResult) (io.ReadCloser, int64, string) {
	return result.archiveFile, result.size, result.checksum
}

func NewTestCreateArgs(filesToBackUp []string, db db.Dumper) *createArgs {
	args := createArgs{
		filesToBackUp: filesToBackUp,
		db:            db,
	}
	return &args
}
