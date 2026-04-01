// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"time"

	"github.com/juju/juju/core/user"
)

// ExternalUserImport holds the data needed to recreate an external user during
// model migration import. Permission granting is handled separately by the
// migration operation.
type ExternalUserImport struct {
	Name        user.Name
	DisplayName string
	DateCreated time.Time
}
