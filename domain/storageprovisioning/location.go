// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

const (
	// DefaultFilesystemAttachmentDir is the default directory where filesystem
	// attachments are mounted if no other location is specified.
	DefaultFilesystemAttachmentDir = "/var/lib/juju/storage"
)

// ProhibitedFilesystemAttachmentLocations returns a list of locations for which
// a filesystem attachment is prohibited from being requested to mount at.
func ProhibitedFilesystemAttachmentLocations() []string {
	return []string{
		"/charm", // Used by Kubernetes provider
		DefaultFilesystemAttachmentDir,
	}
}
