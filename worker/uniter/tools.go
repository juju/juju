// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/worker/uniter/jujuc"
)

// EnsureJujucSymlinks creates a symbolic link to jujuc within dir for each
// hook command. If the commands already exist, this operation does nothing.
func EnsureJujucSymlinks(dir string) (err error) {
	for _, name := range jujuc.CommandNames() {
		// The link operation fails when the target already exists,
		// so this is a no-op when the command names already
		// exist.
		err := os.Symlink("./jujud", filepath.Join(dir, name))
		if err == nil {
			continue
		}
		// TODO(rog) drop LinkError check when fix is released (see http://codereview.appspot.com/6442080/)
		if e, ok := err.(*os.LinkError); !ok || !os.IsExist(e.Err) {
			return fmt.Errorf("cannot initialize hook commands in %q: %v", dir, err)
		}
	}
	return nil
}
