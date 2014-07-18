// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

import (
	"fmt"
)

// XXX Move this to juju/juju/testing/files.go as FakeFile.
type badReadWriter struct{}

func (rw *badReadWriter) Read([]byte) (int, error) {
	return 0, fmt.Errorf("failed to read")
}

func (rw *badReadWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("failed to write")
}

func (rw *badReadWriter) Close() error {
	return nil
}
