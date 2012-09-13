// The cmd/jujuc/server package implements the server side of the jujuc proxy
// tool, which forwards command invocations to the unit agent process so that
// they can be executed against specific state.
package server

import (
	"fmt"
	"bufio"
	"io"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"sync"
)

// HookContext is responsible for the state against which a jujuc-forwarded
// command will execute within a unit agent; it implements the core of the
// various jujuc tools, and is involved in constructing a suitable shell
// environment in which to execute a hook (which is likely to call jujuc
// tools that need this specific HookContext).
type HookContext struct {
	Service *state.Service
	Unit    *state.Unit

	// Id identifies the context.
	Id string

	// RelationId identifies the relation for which a relation hook is
	// executing. If it is -1, the context is not running a relation hook;
	// otherwise, its value must be a valid key into the Relations map.
	RelationId int

	// RemoteUnitName identifies the changing unit of the executing relation
	// hook. It will be empty if the context is not running a relation hook,
	// or if it is running a relation-broken hook.
	RemoteUnitName string

	// Relations contains the context for every relation the unit is a member
	// of, keyed on relation id.
	Relations map[int]*RelationContext
}

// newCommands maps Command names to initializers.
var newCommands = map[string]func(*HookContext) (cmd.Command, error){
	"close-port":    NewClosePortCommand,
	"config-get":    NewConfigGetCommand,
	"juju-log":      NewJujuLogCommand,
	"open-port":     NewOpenPortCommand,
	"relation-get":  NewRelationGetCommand,
	"relation-ids":  NewRelationIdsCommand,
	"relation-list": NewRelationListCommand,
	"relation-set":  NewRelationSetCommand,
	"unit-get":      NewUnitGetCommand,
}

// CommandNames returns the names of all jujuc commands.
func CommandNames() (names []string) {
	for name := range newCommands {
		names = append(names, name)
	}
	sort.Strings(names)
	return
}

// NewCommand returns an instance of the named Command, initialized to execute
// against this HookContext.
func (ctx *HookContext) NewCommand(name string) (cmd.Command, error) {
	f := newCommands[name]
	if f == nil {
		return nil, fmt.Errorf("unknown command: %s", name)
	}
	return f(ctx)
}

// hookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into ctx.
func (ctx *HookContext) hookVars(charmDir, socketPath string) []string {
	vars := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=" + os.Getenv("PATH"),
		"CHARM_DIR=" + charmDir,
		"JUJU_CONTEXT_ID=" + ctx.Id,
		"JUJU_AGENT_SOCKET=" + socketPath,
		"JUJU_UNIT_NAME=" + ctx.Unit.Name(),
	}
	if ctx.RelationId != -1 {
		vars = append(vars, "JUJU_RELATION="+ctx.envRelation())
		vars = append(vars, "JUJU_RELATION_ID="+ctx.envRelationId())
		if ctx.RemoteUnitName != "" {
			vars = append(vars, "JUJU_REMOTE_UNIT="+ctx.RemoteUnitName)
		}
	}
	return vars
}

// RunHook executes a hook in an environment which allows it to to call back
// into ctx to execute jujuc tools.
func (ctx *HookContext) RunHook(hookName, charmDir, socketPath string) error {
	ps := exec.Command(filepath.Join(charmDir, "hooks", hookName))
	ps.Env = ctx.hookVars(charmDir, socketPath)
	ps.Dir = charmDir
	outReader, err := ps.StdoutPipe()
	if err != nil {
		return err
	}
	ps.Stderr = ps.Stdout
	logger := &hookLogger{
		r: outReader,
		done: make(chan struct{}),
	}
	go logger.run()
	err = ps.Run()
	// We can see the process exit before the logger has processed
	// all its output, so allow a moment for the data buffered
	// in the pipe to be processed. We don't wait indefinitely though,
	// because the hook may have started a background process
	// that keeps the pipe open.
	select {
	case <-logger.done:
	case <-time.After(50 * time.Millisecond):
	}
	logger.stop()
	if ee, ok := err.(*exec.Error); ok && err != nil {
		if os.IsNotExist(ee.Err) {
			// Missing hook is perfectly valid.
			return nil
		}
	}
	write := err == nil
	for id, rctx := range ctx.Relations {
		if write {
			if e := rctx.WriteSettings(); e != nil {
				e = fmt.Errorf(
					"could not write settings from %q to relation %d: %v",
					hookName, id, e,
				)
				log.Printf("%v", e)
				if err == nil {
					err = e
				}
			}
		}
		rctx.ClearCache()
	}
	return err
}

type hookLogger struct {
	r io.ReadCloser
	done chan struct{}
	mu sync.Mutex
	stopped bool
}

func (l *hookLogger) run() {
	defer close(l.done)
	defer l.r.Close()
	br := bufio.NewReader(l.r)
	
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			break
		}
		if line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		l.mu.Lock()
		if l.stopped {
			l.mu.Unlock()
			return
		}
		log.Printf("HOOK %s", line)
		l.mu.Unlock()
	}
}

func (l *hookLogger) stop() {
	// We can't close the pipe asynchronously, so just
	// stifle output instead.
	l.mu.Lock()
	l.stopped = true
	l.mu.Unlock()
}	

// envRelation returns the relation name exposed to hooks as JUJU_RELATION.
// If the context does not have a relation, it will return an empty string.
// Otherwise, it will panic if RelationId is not a key in the Relations map.
func (ctx *HookContext) envRelation() string {
	if ctx.RelationId == -1 {
		return ""
	}
	return ctx.Relations[ctx.RelationId].ru.Endpoint().RelationName
}

// envRelationId returns the relation id exposed to hooks as JUJU_RELATION_ID.
// If the context does not have a relation, it will return an empty string.
// Otherwise, it will panic if RelationId is not a key in the Relations map.
func (ctx *HookContext) envRelationId() string {
	if ctx.RelationId == -1 {
		return ""
	}
	return fmt.Sprintf("%s:%d", ctx.envRelation(), ctx.RelationId)
}

// SettingsMap is a map from unit name to relation settings.
type SettingsMap map[string]map[string]interface{}

// RelationContext exposes relation membership and unit settings information.
type RelationContext struct {
	ru *state.RelationUnit

	// members contains settings for known relation members. Nil values
	// indicate members whose settings have not yet been cached.
	members SettingsMap

	// settings allows read and write access to the relation unit settings.
	settings *state.ConfigNode

	// cache is a short-term cache that enables consistent access to settings
	// for units that are not currently participating in the relation. Its
	// contents should be cleared whenever a new hook is executed.
	cache SettingsMap
}

// NewRelationContext creates a new context for the given relation unit.
// The unit-name keys of members supplies the initial membership.
func NewRelationContext(ru *state.RelationUnit, members map[string]int) *RelationContext {
	ctx := &RelationContext{ru: ru, members: SettingsMap{}}
	for unit := range members {
		ctx.members[unit] = nil
	}
	ctx.ClearCache()
	return ctx
}

// WriteSettings persists all changes made to the unit's relation settings.
func (ctx *RelationContext) WriteSettings() (err error) {
	if ctx.settings != nil {
		_, err = ctx.settings.Write()
	}
	return
}

// ClearCache discards all cached settings for units that are not members
// of the relation, and all unwritten changes to the unit's relation settings.
// including any changes to Settings that have not been written.
func (ctx *RelationContext) ClearCache() {
	ctx.settings = nil
	ctx.cache = make(SettingsMap)
}

// UpdateMembers ensures that the context is aware of every supplied member
// unit. For each supplied member that has non-nil settings, the cached
// settings will be overwritten; but nil settings will not overwrite cached
// ones.
func (ctx *RelationContext) UpdateMembers(members SettingsMap) {
	for m, s := range members {
		_, found := ctx.members[m]
		if !found || s != nil {
			ctx.members[m] = s
		}
	}
}

// DeleteMember drops the membership and cache of a single remote unit, without
// perturbing settings for the remaining members.
func (ctx *RelationContext) DeleteMember(unitName string) {
	delete(ctx.members, unitName)
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

// Units returns the names of the units that are present in the relation, in
// alphabetical order.
func (ctx *RelationContext) Units() (units []string) {
	for unit := range ctx.members {
		units = append(units, unit)
	}
	sort.Strings(units)
	return units
}

// ReadSettings returns the settings of a unit that is now, or was once,
// participating in the relation.
func (ctx *RelationContext) ReadSettings(unit string) (settings map[string]interface{}, err error) {
	settings, member := ctx.members[unit]
	if settings == nil {
		if settings = ctx.cache[unit]; settings == nil {
			settings, err = ctx.ru.ReadSettings(unit)
			if err != nil {
				return nil, err
			}
		}
	}
	if member {
		ctx.members[unit] = settings
	} else {
		ctx.cache[unit] = settings
	}
	return settings, nil
}

// relationIdValue returns a gnuflag.Value for convenient parsing of relation
// ids in context.
func (ctx *HookContext) relationIdValue(result *int) *relationIdValue {
	*result = ctx.RelationId
	return &relationIdValue{result: result, ctx: ctx, value: ctx.envRelationId()}
}

// relationIdValue implements gnuflag.Value for use in relation hook commands.
type relationIdValue struct {
	result *int
	ctx    *HookContext
	value  string
}

// String returns the current value.
func (v *relationIdValue) String() string {
	return v.value
}

// Set interprets value as a relation id, if possible, and returns an error
// if it is not known to the system. The parsed relation id will be written
// to v.result.
func (v *relationIdValue) Set(value string) error {
	trim := value
	if idx := strings.LastIndex(trim, ":"); idx != -1 {
		trim = trim[idx+1:]
	}
	id, err := strconv.Atoi(trim)
	if err != nil {
		return fmt.Errorf("invalid relation id")
	}
	if _, found := v.ctx.Relations[id]; !found {
		return fmt.Errorf("unknown relation id")
	}
	*v.result = id
	v.value = value
	return nil
}
