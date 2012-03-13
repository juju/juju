// The hook package provides a mechanism by which charm hooks can be executed in
// appropriate environments.
package hook

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Env exposes the values of the environment variables that are always expected
// in a hook execution environment, and a map[string]string for any additional
// vars that may be expected in a particular context (such as JUJU_RELATION).
type Env interface {
	ContextId() string
	AgentSock() string
	UnitName() string
	CharmDir() string
	Vars() map[string]string
}

// vars returns the environment variables required to execute a hook, expressed
// as an os.Environ-style []string.
func vars(env Env) []string {
	vars := map[string]string{
		"CHARM_DIR":                env.CharmDir(),
		"JUJU_CONTEXT_ID":          env.ContextId(),
		"JUJU_AGENT_SOCKET":        env.AgentSock(),
		"JUJU_UNIT_NAME":           env.UnitName(),
		"APT_LISTCHANGES_FRONTEND": "none",
		"DEBIAN_FRONTEND":          "noninteractive",
		"PATH":                     os.Getenv("PATH"),
	}
	extra := env.Vars()
	if extra != nil {
		for k, v := range extra {
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
func Exec(env Env, hookName string) error {
	dir := env.CharmDir()
	path := filepath.Join(dir, "hooks", hookName)
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
	ps.Dir = dir
	ps.Env = vars(env)
	return ps.Run()
}
