// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

// ObjectMetadataPath pairs a logical object path with its blob hash.
type ObjectMetadataPath struct {
	Path   string
	SHA384 string
}

// ObjectInventory describes the files a recovered namespace needs.
type ObjectInventory struct {
	Namespace string
	Paths     []ObjectMetadataPath
}
