package uniter

import (
	"fmt"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/environs"
	"os"
	"path/filepath"
)

// EnsureJujucSymlinks creates a symbolic link to jujuc in dataDir for each
// hook command. If the commands already exist, this operation
// does nothing.
func EnsureJujucSymlinks(dataDir, agentName string) (err error) {
	dir := environs.AgentToolsDir(dataDir, agentName)
	for _, name := range server.CommandNames() {
		// The link operation fails when the target already exists,
		// so this is a no-op when the command names already
		// exist.
		err := os.Symlink("./jujuc", filepath.Join(dir, name))
		if err == nil {
			continue
		}
		// TODO(rog) drop LinkError check when fix is released (see http://codereview.appspot.com/6442080/)
		if e, ok := err.(*os.LinkError); !ok || !os.IsExist(e.Err) {
			return fmt.Errorf("cannot initialize hook commands for %q: %v", agentName, err)
		}
	}
	return nil
}
