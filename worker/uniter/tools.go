package uniter

import (
	"fmt"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/environs"
	"os"
	"path/filepath"
	"strings"
)

// EnsureTools creates a symbolic link to jujuc for each
// hook command. If the commands already exist, this operation
// does nothing.
func EnsureTools(unitName string) (err error) {
	dir := environs.AgentToolsDir(agentName(unitName))
	for _, name := range server.CommandNames() {
		// The link operation fails when the target already exists,
		// so this is a no-op when the command names already
		// exist.
		err := os.Symlink("./jujuc", filepath.Join(dir, name))
		if err == nil {
			continue
		}
		// TODO(rog) use os.IsExist when fix is released.
		if e, ok := err.(*os.LinkError); !ok || !os.IsExist(e.Err) {
			return fmt.Errorf("cannot initialize hook commands for unit %q: %v", unitName, err)
		}
	}
	return nil
}

func agentName(unitName string) string {
	return "unit-" + unitFsName(unitName)
}

// unitFsName returns a variation on the supplied unit name that can be used in
// a filesystem path.
func unitFsName(unitName string) string {
	return strings.Replace(unitName, "/", "-", 1)
}
