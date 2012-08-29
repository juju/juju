package uniter

import (
	"fmt"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/environs"
	"os"
	"path/filepath"
	"strings"
)

// EnsureJujucSymlinks creates a symbolic link to jujuc for each
// hook command. If the commands already exist, this operation
// does nothing.
func EnsureJujucSymlinks(unitName string) (err error) {
	agentName := "unit-" + strings.Replace(unitName, "/", "-", 1)
	log.Printf("VarDir: %q", environs.VarDir)
	dir := environs.AgentToolsDir(agentName)
	log.Printf("toolsDir(%q): %q", agentName, dir)
	for _, name := range server.CommandNames() {
		// The link operation fails when the target already exists,
		// so this is a no-op when the command names already
		// exist.
		err := os.Symlink("./jujuc", filepath.Join(dir, name))
		if err == nil {
			continue
		}
		_, err1 := os.Stat(dir)
		log.Printf("after symlink failure, stat %q -> %v", dir, err1)
		// TODO(rog) drop LinkError check when fix is released (see http://codereview.appspot.com/6442080/)
		if e, ok := err.(*os.LinkError); !ok || !os.IsExist(e.Err) {
			return fmt.Errorf("cannot initialize hook commands for unit %q: %v", unitName, err)
		}
	}
	log.Printf("made symlinks ok")
	return nil
}
