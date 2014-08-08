// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"

	"github.com/juju/juju/state/backups/db"
)

var (
	Create = create

	GetFilesToBackUp = &getFilesToBackUp
	GetDBDumper      = &getDBDumper
	RunCreate        = &runCreate
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

func ExposeCreateArgs(args *createArgs) ([]string, db.Dumper) {
	return args.filesToBackUp, args.db
}

func NewTestCreateResult(file io.ReadCloser, size int64, checksum string) *createResult {
	result := createResult{
		archiveFile: file,
		size:        size,
		checksum:    checksum,
	}
	return &result
}

func NewTestCreate(result *createResult, err error) (*createArgs, func(*createArgs) (*createResult, error)) {
	var received createArgs

	testCreate := func(args *createArgs) (*createResult, error) {
		received = *args
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	return &received, testCreate
}
