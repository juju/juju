// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"fmt"
	"os"
)

var (
	GetMongodumpPath = &getMongodumpPath
	GetFilesToBackup = &getFilesToBackup
	DoBackup         = &runCommand

	DefaultFilename = defaultFilename
)

func TarFiles(fileList []string, targetPath, strip string, compress bool) (shaSum string, err error) {
	ar := archive{fileList, strip}

	// Create the file.
	f, err := os.Create(targetPath)
	if err != nil {
		return "", fmt.Errorf("cannot create backup file %q", targetPath)
	}

	// Write out the archive.
	if compress {
		return writeTarball(&ar, f)
	} else {
		proxy := newSHA1Proxy(f)
		err = ar.Write(proxy)
		if err != nil {
			return "", err
		}
		return proxy.Hash(), nil
	}
}
