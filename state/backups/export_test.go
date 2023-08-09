// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"io"

	"github.com/juju/errors"

	corebackups "github.com/juju/juju/core/backups"
)

var (
	Create = create

	TestGetFilesToBackUp = &getFilesToBackUp
	GetDBDumper          = &getDBDumper
	RunCreate            = &runCreate
	FinishMeta           = &finishMeta
	GetMongodumpPath     = &getMongodumpPath
	RunCommand           = &runCommandFn
	AvailableDisk        = &availableDisk
	TotalDisk            = &totalDisk
	DirSize              = &dirSize
)

// ExposeCreateResult extracts the values in a create() result.
func ExposeCreateResult(result *createResult) (io.ReadCloser, int64, string, string) {
	return result.archiveFile, result.size, result.checksum, result.filename
}

// NewTestCreateArgs builds a new args value for create() calls.
func NewTestCreateArgs(backupDir string, filesToBackUp []string, db DBDumper, metar io.Reader) *createArgs {
	args := createArgs{
		destinationDir: backupDir,
		filesToBackUp:  filesToBackUp,
		db:             db,
		metadataReader: metar,
	}
	return &args
}

// ExposeCreateResult extracts the values in a create() args value.
func ExposeCreateArgs(args *createArgs) (string, []string, DBDumper) {
	return args.destinationDir, args.filesToBackUp, args.db
}

// NewTestCreateResult builds a new create() result.
func NewTestCreateResult(file io.ReadCloser, size int64, checksum, filename string) *createResult {
	result := createResult{
		archiveFile: file,
		size:        size,
		checksum:    checksum,
		filename:    filename,
	}
	return &result
}

// NewTestCreate builds a new replacement for create() with the given result.
func NewTestCreate(result *createResult) (*createArgs, func(*createArgs) (*createResult, error)) {
	var received createArgs

	if result == nil {
		archiveFile := io.NopCloser(bytes.NewBufferString("<archive>"))
		result = NewTestCreateResult(archiveFile, 10, "<checksum>", "")
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
func NewTestMetaFinisher(failure string) func(*corebackups.Metadata, *createResult) error {
	return func(*corebackups.Metadata, *createResult) error {
		if failure == "" {
			return nil
		}
		return errors.New(failure)
	}
}
