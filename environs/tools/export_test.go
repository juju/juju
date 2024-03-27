// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

var (
	FindExecutable                = findExecutable
	CheckToolsReleases            = checkToolsReleases
	ArchiveAndSHA256              = archiveAndSHA256
	WriteMetadataFiles            = &writeMetadataFiles
	CurrentStreamsVersion         = currentStreamsVersion
	MarshalToolsMetadataIndexJSON = marshalToolsMetadataIndexJSON
)

func VersionsMatchingHash(v *Versions, h string) []string {
	return v.versionsMatchingHash(h)
}
