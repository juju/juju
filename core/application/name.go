// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strings"

	"github.com/juju/juju/internal/uuid"
)

// IsRemoteApplication returns true if the application name indicates
// that it is a remote application. This is determined by checking if the
// application name is of the form remote-<uuid> (where <uuid> is a valid UUID
// without dashes).
func IsRemoteApplication(appName string) bool {
	parts := strings.SplitN(appName, "-", 2)
	// Check that the application name is of the form remote-<uuid>.
	if len(parts) != 2 || parts[0] != "remote" {
		return false
	}

	// Ensure the second part is a valid UUID without dashes.
	if len(parts[1]) != 32 {
		return false
	}

	_, err := uuid.UUIDFromEncodedString(parts[1])
	return err == nil
}
