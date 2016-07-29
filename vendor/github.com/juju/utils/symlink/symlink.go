// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package symlink

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/utils"
)

// Replace will do an atomic replacement of a symlink to a new path
func Replace(link, newpath string) error {
	dstDir := filepath.Dir(link)
	uuid, err := utils.NewUUID()
	if err != nil {
		return err
	}
	randStr := uuid.String()
	tmpFile := filepath.Join(dstDir, "tmpfile"+randStr)
	// Create the new symlink before removing the old one. This way, if New()
	// fails, we still have a link to the old tools.
	err = New(newpath, tmpFile)
	if err != nil {
		return fmt.Errorf("cannot create symlink: %s", err)
	}
	// On Windows, symlinks may not be overwritten. We remove it first,
	// and then rename tmpFile
	if _, err := os.Stat(link); err == nil {
		err = os.RemoveAll(link)
		if err != nil {
			return err
		}
	}
	err = os.Rename(tmpFile, link)
	if err != nil {
		return fmt.Errorf("cannot update tools symlink: %v", err)
	}
	return nil
}
