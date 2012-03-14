// The hook package provides a mechanism by which charm hooks can be executed in
// appropriate environments.
package hook

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ExecContext exposes the parts of Context that are necessary to integrate
// with Exec. This is useful partly because Context is not yet implemented, and
// partly for ease of testing.
type ExecContext interface {
	Vars() []string
	Flush() error
}

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

// Exec executes the named hook in the environment defined by ctx and info (or
// silently returns if the hook doesn't exist).
func Exec(hookName string, info *ExecInfo, ctx ExecContext) error {
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
	ps := exec.Command(path)
	ps.Dir = info.CharmDir
	ps.Env = append(info.Vars(), ctx.Vars()...)
	err = ps.Run()
	if err != nil {
		return err
	}
	return ctx.Flush()
}
