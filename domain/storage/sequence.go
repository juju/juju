// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	domainsequence "github.com/juju/juju/domain/sequence"
)

// These consts are the sequence namespaces used to generate
// monotonically increasing ints to use for storage entity IDs.
const (
	FilesystemSequenceNamespace      = domainsequence.StaticNamespace("filesystem")
	VolumeSequenceNamespace          = domainsequence.StaticNamespace("volume")
	StorageInstanceSequenceNamespace = domainsequence.StaticNamespace("storage")
)
