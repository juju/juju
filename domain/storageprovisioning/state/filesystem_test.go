// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/tc"
)

// filesystemSuite provides a test suite for testing the filesystem watcher
// provisioning interfact of state.
type filesystemWatcherSuite struct {
	schematesting.ModelSuite
}

func TestFilesystemWatcherSuite(t *testing.T) {
	tc.Run(t, &filesystemWatcherSuite{})
}
