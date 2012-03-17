// The hook package provides a mechanism by which charm hooks can be executed in
// appropriate environments.
package hook

import (
	"fmt"
	"launchpad.net/juju/go/log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Context is responsible for the state against which a hook tool will execute;
// it implements the core of the various hook tools and is involved in
// constructing a suitable environment in which to execute a hook (which may
// call hook tools that need to call back into the Context).
type Context struct {
	Id             string
	LocalUnitName  string
	RemoteUnitName string
	RelationName   string
}

// Log is the core of the `log` hook command, and is always meaningful in any
// Context.
func (ctx *Context) Log(debug bool, msg string) {
	s := []string{}
	if ctx.LocalUnitName != "" {
		s = append(s, ctx.LocalUnitName)
	}
	if ctx.RelationName != "" {
		s = append(s, ctx.RelationName)
	}
	full := fmt.Sprintf("Context<%s>: %s", strings.Join(s, ", "), msg)
	if debug {
		log.Debugf(full)
	} else {
		log.Printf(full)
	}
}

// vars returns an os.Environ-style list of strings necessary to run a hook in,
// and call back into, ctx.
func vars(ctx *Context, charmDir, socketPath string) []string {
	vars := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=" + os.Getenv("PATH"),
		"CHARM_DIR=" + charmDir,
		"JUJU_CONTEXT_ID=" + ctx.Id,
		"JUJU_AGENT_SOCKET=" + socketPath,
	}
	if ctx.LocalUnitName != "" {
		vars = append(vars, "JUJU_UNIT_NAME="+ctx.LocalUnitName)
	}
	if ctx.RemoteUnitName != "" {
		vars = append(vars, "JUJU_REMOTE_UNIT="+ctx.RemoteUnitName)
	}
	if ctx.RelationName != "" {
		vars = append(vars, "JUJU_RELATION="+ctx.RelationName)
	}
	return vars
}

// Exec executes a hook in an environment which allows it to to call back into
// ctx to execute hook tools.
func Exec(ctx *Context, hookName, charmDir, socketPath string) error {
	ps := exec.Command(filepath.Join(charmDir, "hooks", hookName))
	ps.Env = vars(ctx, charmDir, socketPath)
	ps.Dir = charmDir
	if err := ps.Run(); err != nil {
		if ee, ok := err.(*exec.Error); ok {
			if os.IsNotExist(ee.Err) {
				return nil
			}
		}
		return err
	}
	return nil
}
