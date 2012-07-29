// The cmd/jujuc/server package implements the server side of the jujuc proxy
// tool, which forwards command invocations to the unit agent process so that
// they can be executed against specific state.
package server

import (
	"fmt"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/state"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// ClientContext is responsible for the state against which a jujuc-forwarded
// command will execute; it implements the core of the various jujuc tools, and
// is involved in constructing a suitable environment in which to execute a hook
// (which is likely to call jujuc tools that need this specific ClientContext).
type ClientContext struct {
	Id             string
	State          *state.State
	LocalUnitName  string
	RemoteUnitName string
	RelationName   string
}

// checkUnitState returns an error if ctx has nil State or LocalUnitName fields.
func (ctx *ClientContext) check() error {
	if ctx.State == nil {
		return fmt.Errorf("context %s cannot access state", ctx.Id)
	}
	if ctx.LocalUnitName == "" {
		return fmt.Errorf("context %s is not attached to a unit", ctx.Id)
	}
	return nil
}

// newCommands maps Command names to initializers.
var newCommands = map[string]func(*ClientContext) (cmd.Command, error){
	"close-port": NewClosePortCommand,
	"config-get": NewConfigGetCommand,
	"juju-log":   NewJujuLogCommand,
	"open-port":  NewOpenPortCommand,
	"unit-get":   NewUnitGetCommand,
}

// NewCommand returns an instance of the named Command, initialized to execute
// against this ClientContext.
func (ctx *ClientContext) NewCommand(name string) (cmd.Command, error) {
	f := newCommands[name]
	if f == nil {
		return nil, fmt.Errorf("unknown command: %s", name)
	}
	return f(ctx)
}

// hookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into ctx.
func (ctx *ClientContext) hookVars(charmDir, socketPath string) []string {
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
// into ctx to execute jujuc tools.
func (ctx *ClientContext) RunHook(hookName, charmDir, socketPath string) error {
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

// settingsMap is a map from unit name to relation settings.
type settingsMap map[string]map[string]interface{}

// RelationContext exposes relation membership and unit settings information.
type RelationContext struct {
	st *state.State
	ru *state.RelationUnit

	// members contains settings for known relation members. Nil values
	// indicate members whose settings have not yet been cached.
	members settingsMap

	// settings allows read and write access to the relation unit settings.
	settings *state.ConfigNode

	// units is a sorted list of unit names reflecting the contents of members.
	units []string

	// cache is a short-term cache that enables consistent access to settings
	// for units that are not currently participating in the relation. Its
	// contents should be cleared whenever a new hook is executed.
	cache settingsMap
}

// NewRelationContext returns a RelationContext that provides access to
// information about the supplied relation unit. Initial membership is
// determined by the keys in members.
func NewRelationContext(st *state.State, ru *state.RelationUnit, members map[string]int) *RelationContext {
	ctx := &RelationContext{st: st, ru: ru}
	m := settingsMap{}
	for unit := range members {
		m[unit] = nil
	}
	ctx.Update(m)
	ctx.Flush(false)
	return ctx
}

// Flush clears the cached data for non-member units, making the context
// suitable for use in the execution of a fresh hook. If write is true,
// any changes made to Settings will be persisted; otherwise, they will
// be discarded.
func (ctx *RelationContext) Flush(write bool) error {
	if write && ctx.settings != nil {
		_, err := ctx.settings.Write()
		return err
	}
	ctx.settings = nil
	ctx.cache = make(settingsMap)
	return nil
}

// Update completely replaces the context's membership data.
func (ctx *RelationContext) Update(members settingsMap) {
	ctx.members = members
	ctx.units = nil
	for unit := range ctx.members {
		ctx.units = append(ctx.units, unit)
	}
	sort.Strings(ctx.units)
}

// Settings returns a ConfigNode that gives read and write access to the
// unit's relation settings.
func (ctx *RelationContext) Settings() (*state.ConfigNode, error) {
	if ctx.settings == nil {
		node, err := ctx.ru.Settings()
		if err != nil {
			return nil, err
		}
		ctx.settings = node
	}
	return ctx.settings, nil
}

// Units returns the names of the units that are present in the relation.
func (ctx *RelationContext) Units() (units []string) {
	return ctx.units
}

// ReadSettings returns the settings of a unit that is now, or was once,
// participating in the relation, and caches the result.
func (ctx *RelationContext) ReadSettings(unit string) (settings map[string]interface{}, err error) {
	settings, member := ctx.members[unit]
	if settings == nil {
		settings, err = ctx.ru.ReadSettings(unit)
		if err != nil {
			return nil, err
		}
	}
	if member {
		ctx.members[unit] = settings
	} else {
		ctx.cache[unit] = settings
	}
	return settings, nil
}
