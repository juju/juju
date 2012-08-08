package uniter

import (
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/environs"
	"os"
	"path/filepath"
	"strings"
)

// EnsureTools returns an error if the juju tools directory for this agent
// does not contain jujuc symlinks for each hook command, and they cannot be
// created.
func EnsureTools(unitName string) (err error) {
	defer errorContextf(&err, "cannot initialize hook commands for unit %q", unitName)
	unitFsName := unitFsName(unitName)
	dir := environs.AgentToolsDir(unitFsName)
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		return err
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return err
	}
	jujuc := filepath.Join(dir, "jujuc")
	if _, err = os.Stat(jujuc); err != nil {
		return err
	}
	for _, name := range server.CommandNames() {
		tool := filepath.Join(dir, name)
		fi, err := os.Lstat(tool)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if fi != nil && fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			if dst, err := os.Readlink(tool); err != nil {
				return err
			} else if dst == jujuc {
				continue
			}
		}
		if err = os.Symlink(jujuc, tool); err != nil {
			return err
		}
	}
	return nil
}

// unitFsName returns a variation on the supplied unit name that can be used in
// a filesystem path.
func unitFsName(unitName string) string {
	return strings.Replace(unitName, "/", "-", 1)
}
