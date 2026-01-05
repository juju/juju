// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io"
	"os"

	"github.com/juju/juju/internal/errors"
)

// cleanupReadCloser provides a wrapped implementation of [io.Closer] ensuring that
// the provided cleanup func is run when [cleanupReadCloser.Close] is called.
type cleanupReader struct {
	io.Reader
	closeFunc func() error
}

// strictLimitedReader is a special [io.LimitedReader] that returns
// [ErrorReaderDesync] to the caller of Read when the number of the bytes in
// the stream to not meet the expectation.
type strictLimitedReader io.LimitedReader

const (
	// ErrorReaderDesync is an error that occurs when a given [io.Reader]
	// contains either an under or overflow condition and does not match the
	// expected number of bytes.
	ErrorReaderDesync = errors.ConstError("reader is not synchronized with expected size")
)

// Close will call the provided [cleanupReadCloser.CloseFunc] when called. Implements
// [io.Closer] interface.
func (c cleanupReader) Close() error {
	return c.closeFunc()
}

// newStrictCacher creates a new [io.ReaderCloser] over the supplied reader
// caching the readers contents to a local temporary file. When the returned
// closer is closed the temporary file will be removed.
//
// Should the provided reader not contain the expected number of bytes Read
// will return an error satisfying [ErrorReaderDesync].
func newStrictCacher(
	r io.Reader, size int64,
) (io.ReadCloser, error) {
	tmpFile, err := os.CreateTemp("", "jujutools*")
	if err != nil {
		return nil, errors.Errorf(
			"creating temporary cache file for reader: %w", err,
		)
	}
	closeFunc := func() error {
		errClose := tmpFile.Close()
		errCleanup := os.Remove(tmpFile.Name())
		return errors.Join(errClose, errCleanup)
	}

	r = &strictLimitedReader{R: r, N: size}
	if _, err := io.Copy(tmpFile, r); err != nil {
		_ = closeFunc()
		return nil, err
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		_ = closeFunc()
		return nil, errors.Errorf(
			"seeking cached file back to the start: %w", err,
		)
	}

	return cleanupReader{
		Reader:    &strictLimitedReader{R: tmpFile, N: size},
		closeFunc: closeFunc,
	}, nil
}

func (s *strictLimitedReader) Read(p []byte) (int, error) {
	n, err := (*io.LimitedReader)(s).Read(p)

	if err != nil && errors.Is(err, io.EOF) && s.N < 0 {
		return -1, errors.Errorf(
			"reader does not meet byte expectation of having %d bytes", n,
		).Add(ErrorReaderDesync)
	}
	return n, err
}
