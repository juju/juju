// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package download

import (
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
)

const (
	// ErrUnexpectedHash is returned when the hash of the downloaded resource
	// does not match the expected hash.
	ErrUnexpectedHash = errors.ConstError("downloaded resource has unexpected hash")

	// ErrUnexpectedSize is returned when the size of the downloaded resources
	// does not match the expected size.
	ErrUnexpectedSize = errors.ConstError("downloaded resource has unexpected size")
)

// FileSystem defines a file system for modifying files on a users system.
type FileSystem interface {
	// CreateTemp creates a new temporary file in the directory dir,
	// opens the file for reading and writing, and returns the resulting file.
	CreateTemp(dir, pattern string) (*os.File, error)

	// Open opens the named file for reading. If successful, methods on
	// the returned file can be used for reading; the associated file
	Open(name string) (*os.File, error)

	// Remove removes the named file or (empty) directory.
	Remove(name string) error
}

// Downloader validates a resource blob.
type Downloader struct {
	logger     logger.Logger
	fileSystem FileSystem
}

// NewDownloader returns a new resource blob validator.
func NewDownloader(logger logger.Logger, fileSystem FileSystem) *Downloader {
	return &Downloader{
		logger:     logger,
		fileSystem: fileSystem,
	}
}

// Download takes a request body ReadCloser containing a resource blob and
// checks that the size and hash match the expected values. It downloads the
// blob to a temporary file and returns a ReadCloser that deletes the
// temporary file on closure.
//
// Returns [ErrUnexpectedHash] if the hash of the downloaded resource does not
// match the expected hash.
// Returns [ErrUnexpectedSize] if the size of the downloaded resource does not
// match the expected size.
func (v *Downloader) Download(
	reader io.ReadCloser,
	expectedSHA384 string,
	expectedSize int64,
) (_ io.ReadCloser, err error) {
	defer func() {
		_ = reader.Close()
	}()

	tmpFile, err := v.fileSystem.CreateTemp("", "resource-")
	if err != nil {
		return nil, errors.Capture(err)
	}
	defer func() {
		// Always close the file, we no longer require to have it open for
		// this purpose. Another process/method can take over the file.
		cErr := tmpFile.Close()

		// If the download was successful, we don't need to remove the file.
		// It is the responsibility of the caller to remove the file.
		if err == nil {
			if cErr != nil {
				v.logger.Warningf("closing temporary file: %v", cErr)
			}
			return
		}

		// Remove the temporary file if the download failed. If we can't
		// remove the file, log a warning, so the operator can clean it up
		// manually.
		removeErr := v.fileSystem.Remove(tmpFile.Name())
		if removeErr != nil {
			v.logger.Warningf("failed to remove temporary file %q: %v", tmpFile.Name(), removeErr)
		}
	}()

	hasher384 := sha512.New384()

	size, err := io.Copy(tmpFile, io.TeeReader(reader, hasher384))
	if err != nil {
		return nil, errors.Capture(err)
	}

	sha384 := hex.EncodeToString(hasher384.Sum(nil))

	if sha384 != expectedSHA384 {
		return nil, errors.Errorf(
			"%w: got %q, expected %q", ErrUnexpectedHash, sha384, expectedSHA384,
		)
	}
	if size != expectedSize {
		return nil, errors.Errorf(
			"%w: got %q, expected %q", ErrUnexpectedSize, size, expectedSize,
		)
	}

	// Create a reader for the temporary file containing the resource.
	tmpFileReader, err := v.newTmpFileReader(tmpFile.Name())
	if err != nil {
		return nil, errors.Errorf("opening downloaded resource: %w", err)
	}

	return tmpFileReader, nil
}

func (v *Downloader) newTmpFileReader(file string) (*tmpFileReader, error) {
	f, err := v.fileSystem.Open(file)
	if err != nil {
		return nil, err
	}
	return &tmpFileReader{
		File:       f,
		logger:     v.logger,
		filesystem: v.fileSystem,
	}, nil
}

// tmpFileReader wraps an *os.File and deletes it when closed.
type tmpFileReader struct {
	*os.File
	filesystem FileSystem
	logger     logger.Logger
}

// Close closes the temporary file and removes it. If the file cannot be
// removed, an error is logged.
func (f *tmpFileReader) Close() (err error) {
	defer func() {
		removeErr := f.filesystem.Remove(f.Name())
		if err == nil {
			err = removeErr
		} else if removeErr != nil {
			f.logger.Warningf("failed to remove temporary file %q: %v", f.Name(), removeErr)
		}
	}()

	return f.File.Close()
}

type fileSystem struct{}

// CreateTemp creates a new temporary file in the directory dir,
// opens the file for reading and writing, and returns the resulting file.
func (fileSystem) CreateTemp(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}

// Open opens the named file for reading. If successful, methods on
// the returned file can be used for reading; the associated file
func (fileSystem) Open(name string) (*os.File, error) {
	return os.Open(name)
}

// Remove removes the named file or (empty) directory.
func (fileSystem) Remove(name string) error {
	return os.Remove(name)
}

// DefaultFileSystem returns the default file system.
func DefaultFileSystem() FileSystem {
	return fileSystem{}
}
