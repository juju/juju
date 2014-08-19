// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
)

var (
	GetMongodumpPath = &getMongodumpPath
	GetFilesToBackup = &getFilesToBackup
	RunCommand       = &runCommand

	Create = create
)

func DumpCreateResult(result *createResult) (io.ReadCloser, int64, string) {
	return result.archiveFile, result.size, result.checksum
}

func NewTestCreateArgs(filesToBackUp []string, db dumper) *createArgs {
	args := createArgs{
		filesToBackUp: filesToBackUp,
		db:            db,
	}
	return &args
}

func NewTestDBDumper() *mongoDumper {
	return &mongoDumper{"localhost:8080", "bogus-user", "boguspassword"}
}
