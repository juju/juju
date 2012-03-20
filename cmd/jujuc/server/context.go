// The cmd/jujuc/server package implements the server side of the jujuc proxy
// tool, which forwards command invocations to the unit agent process so that
// they can be executed against specific state.
package server

import (
	"fmt"
	"launchpad.net/juju/go/log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Context is responsible for the state against which a jujuc-forwarded command
// will execute; it implements the core of the various jujuc tools, and is
// involved in constructing a suitable environment in which to execute a hook
// (which is likely to call jujuc tools that need this specific Context).
type Context struct {
	Id             string
	LocalUnitName  string
	RemoteUnitName string
	RelationName   string
}

// Log is the core of the `juju-log` hook command, and is always meaningful in
// any Context.
func (ctx *Context) Log(debug bool, msg string) {
	s := []string{}
	if ctx.LocalUnitName != "" {
		s = append(s, ctx.LocalUnitName)
	}
	if ctx.RelationName != "" {
		s = append(s, ctx.RelationName)
	}
	if len(s) > 0 {
		msg = fmt.Sprintf("%s: ", strings.Join(s, " ")) + msg
	}
	if debug {
		log.Debugf(msg)
	} else {
		log.Printf(msg)
	}
}

// hookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into ctx.
func (ctx *Context) hookVars(charmDir, socketPath string) []string {
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

// RunHook executes a hook in an environment which allows it to to call back
// into ctx to execute hook tools.
func (ctx *Context) RunHook(hookName, charmDir, socketPath string) error {
	ps := exec.Command(filepath.Join(charmDir, "hooks", hookName))
	ps.Env = ctx.hookVars(charmDir, socketPath)
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
