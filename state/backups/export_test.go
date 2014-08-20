// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
)

var (
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

type testDBDumper struct {
	DumpDir string
}

func (d *testDBDumper) Dump(dumpDir string) error {
	d.DumpDir = dumpDir
	return nil
}

func NewTestDBDumper() *testDBDumper {
	return &testDBDumper{}
}
