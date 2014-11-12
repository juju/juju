// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/utils/filestorage"
)

var (
	Create = create

	TestGetFilesToBackUp = &getFilesToBackUp
	GetDBDumper          = &getDBDumper
	RunCreate            = &runCreate
	FinishMeta           = &finishMeta
	StoreArchiveRef      = &storeArchive
	GetMongodumpPath     = &getMongodumpPath
	RunCommand           = &runCommand
)

// ExposeCreateResult extracts the values in a create() result.
func ExposeCreateResult(result *createResult) (io.ReadCloser, int64, string) {
	return result.archiveFile, result.size, result.checksum
}

// NewTestCreateArgs builds a new args value for create() calls.
func NewTestCreateArgs(filesToBackUp []string, db DBDumper, mfile io.Reader) *createArgs {
	args := createArgs{
		filesToBackUp: filesToBackUp,
		db:            db,
		metadataFile:  mfile,
	}
	return &args
}

// ExposeCreateResult extracts the values in a create() args value.
func ExposeCreateArgs(args *createArgs) ([]string, DBDumper) {
	return args.filesToBackUp, args.db
}

// NewTestCreateResult builds a new create() result.
func NewTestCreateResult(file io.ReadCloser, size int64, checksum string) *createResult {
	result := createResult{
		archiveFile: file,
		size:        size,
		checksum:    checksum,
	}
	return &result
}

// NewTestCreate builds a new replacement for create() with the given result.
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

// NewTestCreate builds a new replacement for create() with the given failure.
func NewTestCreateFailure(failure string) func(*createArgs) (*createResult, error) {
	return func(*createArgs) (*createResult, error) {
		return nil, errors.New(failure)
	}
}

// NewTestMetaFinisher builds a new replacement for finishMetadata with
// the given failure.
func NewTestMetaFinisher(failure string) func(*Metadata, *createResult) error {
	return func(*Metadata, *createResult) error {
		if failure == "" {
			return nil
		}
		return errors.New(failure)
	}
}

// NewTestArchiveStorer builds a new replacement for StoreArchive with
// the given failure.
func NewTestArchiveStorer(failure string) func(filestorage.FileStorage, *Metadata, io.Reader) error {
	return func(filestorage.FileStorage, *Metadata, io.Reader) error {
		if failure == "" {
			return nil
		}
		return errors.New(failure)
	}
}
