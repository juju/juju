// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/utils/filestorage"

	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backups/metadata"
)

var (
	Create = create

	GetFilesToBackUp = &getFilesToBackUp
	GetDBDumper      = &getDBDumper
	RunCreate        = &runCreate
	FinishMeta       = &finishMeta
	StoreArchive     = &storeArchive
)

func ExposeCreateResult(result *createResult) (io.ReadCloser, int64, string) {
	return result.archiveFile, result.size, result.checksum
}

func NewTestCreateArgs(filesToBackUp []string, db db.Dumper, mfile io.Reader) *createArgs {
	args := createArgs{
		filesToBackUp: filesToBackUp,
		db:            db,
		metadataFile:  mfile,
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

func NewTestCreate(result *createResult) (*createArgs, func(*createArgs) (*createResult, error)) {
	var received createArgs

	if result == nil {
		archiveFile := ioutil.NopCloser(bytes.NewBufferString("<archive>"))
		result = NewTestCreateResult(archiveFile, 10, "<checksum>")
	}

	testCreate := func(args *createArgs) (*createResult, error) {
		received = *args
		return result, nil
	}

	return &received, testCreate
}

func NewTestCreateFailure(failure string) func(*createArgs) (*createResult, error) {
	return func(*createArgs) (*createResult, error) {
		return nil, errors.New(failure)
	}
}

func NewTestMetaFinisher(failure string) func(*metadata.Metadata, *createResult) error {
	return func(*metadata.Metadata, *createResult) error {
		if failure == "" {
			return nil
		}
		return errors.New(failure)
	}
}

func NewTestArchiveStorer(failure string) func(filestorage.FileStorage, *metadata.Metadata, io.Reader) error {
	return func(filestorage.FileStorage, *metadata.Metadata, io.Reader) error {
		if failure == "" {
			return nil
		}
		return errors.New(failure)
	}
}
