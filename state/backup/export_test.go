// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

var (
	TarFiles         = tarFiles
	GetMongodumpPath = &getMongodumpPath
	GetFilesToBackup = &getFilesToBackup
	DoBackup         = &runCommand

	ParseJSONError = parseJSONError

	DefaultFilename = defaultFilename
)
