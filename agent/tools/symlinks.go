// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/symlink"

	"github.com/juju/juju/juju/names"
)

// EnsureSymlinks creates a symbolic link to jujud within dir for each
// command. If the commands already exist, this operation does nothing.
// If dir is a symbolic link, it will be dereferenced first.
func EnsureSymlinks(jujuDir, dir string, commands []string) (err error) {
	logger.Infof("ensure jujuc symlinks in %s", dir)
	defer func() {
		if err != nil {
			err = errors.Annotatef(err, "cannot initialize commands in %q", dir)
		}
	}()
	isSymlink, err := symlink.IsSymlink(jujuDir)
	if err != nil {
		return err
	}
	if isSymlink {
		link, err := symlink.Read(jujuDir)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(link) {
			logger.Infof("%s is relative", link)
			link = filepath.Join(filepath.Dir(dir), link)
		}
		jujuDir = link
		logger.Infof("was a symlink, now looking at %s", jujuDir)
	}

	jujudPath := filepath.Join(jujuDir, names.Jujud)
	logger.Debugf("jujud path %s", jujudPath)
	for _, name := range commands {
		// The link operation fails when the target already exists,
		// so this is a no-op when the command names already
		// exist.
		err := symlink.New(jujudPath, filepath.Join(dir, name))
		if err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}
