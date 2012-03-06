// The hook package provides a mechanism by which charm hooks can be executed in
// appropriate environments.
package hook

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/juju/go/cmd"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// JUJUC_PATH probably shouldn't just be hardcoded here, but I'm not sure
// where else it ought to come from at this stage...
var JUJUC_PATH = "/usr/bin/jujuc"

// Env holds the values of the environment variables that are always expected
// in a hook execution environment, and a map[string]string for any additional
// vars that may be expected in a particular context (such as JUJU_RELATION).
type Env struct {
	ContextId string
	AgentSock string
	CharmDir  string
	UnitName  string
	Vars      map[string]string
}

// environ returns the environment variables required to execute a hook in a
// context defined by env and cs, expressed as an os.Environ-style []string.
func (env *Env) environ(cs *cmdSet) []string {
	path := fmt.Sprintf("%s:%s", cs.path, os.Getenv("PATH"))
	path = strings.Trim(path, ":")
	vars := map[string]string{
		"DEBIAN_FRONTEND":          "noninteractive",
		"APT_LISTCHANGES_FRONTEND": "none",
		"JUJU_CONTEXT_ID":          env.ContextId,
		"JUJU_AGENT_SOCKET":        env.AgentSock,
		"JUJU_UNIT_NAME":           env.UnitName,
		"CHARM_DIR":                env.CharmDir,
		"PATH":                     path,
	}
	if env.Vars != nil {
		for k, v := range env.Vars {
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

// Context exposes information about a particular hook execution context, and
// what commands it makes available to that hook.
type Context interface {
	Env() *Env
	Commands() []cmd.Command
}

// cmdSet is responsible for generating symlinks to jujuc for use by a
// particular hook, and for deleting them afterwards.
type cmdSet struct {
	path string
}

// newCmdSet creates a temporary directory containing symlinks to jujuc.
func newCmdSet(cmds []cmd.Command) (cs *cmdSet, err error) {
	path, err := ioutil.TempDir("", "juju-cmdset-")
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			os.RemoveAll(path)
		}
	}()
	for _, c := range cmds {
		err = os.Symlink(JUJUC_PATH, filepath.Join(path, c.Info().Name))
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

// Exec executes the named hook in the environment defined by ctx (or silently
// returns if the hook doesn't exist).
func Exec(ctx Context, hookName string) error {
	env := ctx.Env()
	path := filepath.Join(env.CharmDir, "hooks", hookName)
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
	cs, err := newCmdSet(ctx.Commands())
	if err != nil {
		return err
	}
	defer cs.delete()
	ps := exec.Command(path)
	ps.Dir = env.CharmDir
	ps.Env = env.environ(cs)
	return ps.Run()
}
