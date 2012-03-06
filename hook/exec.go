// The hook package provides a mechanism by which charm hooks can be executed in
// appropriate environments.
package hook

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

// JUJUC_PATH probably shouldn't just be hardcoded here, but I wasn't sure
// where else it ought to come from at this stage...
var JUJUC_PATH = "/usr/bin/jujuc"

// Info holds the values of the environment variables that are always expected
// in a hook execution environment, and a map[string]string for any additional
// vars that may be expected (for example, JUJU_RELATION).
type Info struct {
	ClientId  string
	AgentSock string
	CharmDir  string
	UnitName  string
	Vars      map[string]string
}

// Context exposes information about a particular hook execution context, and
// what Commands it makes available to that hook.
type Context interface {
	Info() *Info
	Interface() []string
}

// cmdSet is responsible for generating symlinks to jujuc for use by a
// particular hook, and for deleting them afterwards.
type cmdSet struct {
	path string
}

// newCmdSet creates a temporary directory containing symlinks to jujuc.
func newCmdSet(names []string) (cs *cmdSet, err error) {
	path, err := ioutil.TempDir("", "juju-cmdset-")
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			os.RemoveAll(path)
		}
	}()
	for _, name := range names {
		err = os.Symlink(JUJUC_PATH, filepath.Join(path, name))
		if err != nil {
			return
		}
	}
	return &cmdSet{path}, nil
}

// delete deletes the cmdSet's symlinks dir.
func (cs *cmdSet) delete() {
	os.RemoveAll(cs.path)
}

// environ returns the environment variables required to execute the hook
// defined by info and cs, expressed as an os.Environ-style []string.
func environ(info *Info, cs *cmdSet) []string {
	path := os.Getenv("PATH")
	if cs.path != "" {
		path = fmt.Sprintf("%s:%s", cs.path, path)
	}
	vars := map[string]string{
		"DEBIAN_FRONTEND":          "noninteractive",
		"APT_LISTCHANGES_FRONTEND": "none",
		"JUJU_CLIENT_ID":           info.ClientId,
		"JUJU_AGENT_SOCKET":        info.AgentSock,
		"JUJU_UNIT_NAME":           info.UnitName,
		"CHARM_DIR":                info.CharmDir,
		"PATH":                     path,
	}
	if info.Vars != nil {
		for k, v := range info.Vars {
			vars[k] = v
		}
	}
	i := 0
	result := make([]string, len(vars))
	for k, v := range vars {
		result[i] = fmt.Sprintf("%s=%s", k, v)
		i++
	}
	return result
}

// Exec executes the named hook in the environment defined by ctx (or silently
// returns if the hook doesn't exist).
func Exec(ctx Context, hookName string) error {
	info := ctx.Info()
	path := filepath.Join(info.CharmDir, "hooks", hookName)
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Yes, yes, that's fine, we "executed the hook". It's cool.
			return nil
		}
		return err
	}
	if stat.Mode()&0500 != 0500 {
		return fmt.Errorf("hook is not executable: %s", path)
	}
	cs, err := newCmdSet(ctx.Interface())
	if err != nil {
		return err
	}
	defer cs.delete()
	ps := exec.Command(path)
	ps.Dir = info.CharmDir
	ps.Env = environ(info, cs)
	return ps.Run()
}
