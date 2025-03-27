// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"github.com/juju/juju/core/objectstore"
)

// Metadata holds the metadata of the agent binary to be stored into the database.
type Metadata struct {
	// Version is the version of the agent binary.
	Version string
	// Arch is the architecture of the agent binary.
	Arch string
	// ObjectStoreUUID is the UUID primary key of the object store record where the agent binary is stored.
	ObjectStoreUUID objectstore.UUID
}
