// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

var (
	GetMongodumpPath = &getMongodumpPath
	GetFilesToBackup = &getFilesToBackup
	DoBackup         = &runCommand

	DefaultFilename = defaultFilename
)
