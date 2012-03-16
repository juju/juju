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

// ExecInfo is responsible for constructing those parts of a hook execution
// environment which cannot be inferred from the Context itself.
type ExecInfo struct {
	ContextId   string
	AgentSocket string
	CharmDir    string
	RemoteUnit  string
}

// Vars returns an os.Environ-style list of strings.
func (info *ExecInfo) Vars() []string {
	vars := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=" + os.Getenv("PATH"),
		"CHARM_DIR=" + info.CharmDir,
		"JUJU_CONTEXT_ID=" + info.ContextId,
		"JUJU_AGENT_SOCKET=" + info.AgentSocket,
	}
	if info.RemoteUnit != "" {
		vars = append(vars, "JUJU_REMOTE_UNIT="+info.RemoteUnit)
	}
	return vars
}

// Context represents the environment in which a hook (and therefore any hook
// commands called by that hook) will execute.
// It implements the core functionality of the various commands, and runs hooks
// in appropriately-configured environments.
type Context struct {
	Local    string // Name of the local unit
	Relation string // Name of the relation
}

// Exec executes a hook in the environment defined by ctx and info.
func (ctx *Context) Exec(hookName string, info *ExecInfo) error {
	vars := info.Vars()
	if ctx.Local != "" {
		vars = append(vars, "JUJU_UNIT_NAME="+ctx.Local)
	}
	if ctx.Relation != "" {
		vars = append(vars, "JUJU_RELATION="+ctx.Relation)
	}
	ps := exec.Command(filepath.Join(info.CharmDir, "hooks", hookName))
	ps.Dir = info.CharmDir
	ps.Env = vars
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

// Log is the core of the `log` hook command, and is always meaningful in any
// Context.
func (ctx *Context) Log(debug bool, msg string) {
	s := []string{}
	if ctx.Local != "" {
		s = append(s, ctx.Local)
	}
	if ctx.Relation != "" {
		s = append(s, ctx.Relation)
	}
	full := fmt.Sprintf("Context<%s>: %s", strings.Join(s, ", "), msg)
	if debug {
		log.Debugf(full)
	} else {
		log.Printf(full)
	}
}
