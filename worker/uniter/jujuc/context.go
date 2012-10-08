package jujuc

import (
	"bufio"
	"fmt"
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
	"sync"
	"time"
)

type HookContext struct {
	Service *state.Service
	Unit    *state.Unit

	// Id identifies the context.
	Id string

	// RelationId_ identifies the relation for which a relation hook is
	// executing. If it is -1, the context is not running a relation hook;
	// otherwise, its value must be a valid key into the Relations map.
	RelationId_ int

	// RemoteUnitName_ identifies the changing unit of the executing relation
	// hook. It will be empty if the context is not running a relation hook,
	// or if it is running a relation-broken hook.
	RemoteUnitName_ string

	// Relations contains the context for every relation the unit is a member
	// of, keyed on relation id.
	Relations map[int]*RelationContext
}

func (ctx *HookContext) UnitName() string {
	return ctx.Unit.Name()
}

func (ctx *HookContext) PublicAddress() (string, error) {
	return ctx.Unit.PublicAddress()
}

func (ctx *HookContext) PrivateAddress() (string, error) {
	return ctx.Unit.PrivateAddress()
}

func (ctx *HookContext) OpenPort(protocol string, port int) error {
	return ctx.Unit.OpenPort(protocol, port)
}

func (ctx *HookContext) ClosePort(protocol string, port int) error {
	return ctx.Unit.ClosePort(protocol, port)
}

func (ctx *HookContext) Config() (map[string]interface{}, error) {
	node, err := ctx.Service.Config()
	if err != nil {
		return nil, err
	}
	charm, _, err := ctx.Service.Charm()
	if err != nil {
		return nil, err
	}
	// TODO Remove this once state is fixed to report default values.
	cfg, err := charm.Config().Validate(nil)
	if err != nil {
		return nil, err
	}
	return merge(node.Map(), cfg), nil
}

func merge(a, b map[string]interface{}) map[string]interface{} {
	for k, v := range b {
		if _, ok := a[k]; !ok {
			a[k] = v
		}
	}
	return a
}

func (ctx *HookContext) RelationId() int {
	return ctx.RelationId_
}

func (ctx *HookContext) RemoteUnitName() string {
	return ctx.RemoteUnitName_
}

func (ctx *HookContext) RelationIds() []int {
	ids := []int{}
	for id := range ctx.Relations {
		ids = append(ids, id)
	}
	return ids
}

func (ctx *HookContext) Relation(id int) (ContextRelation, error) {
	r, found := ctx.Relations[id]
	if !found {
		return nil, ErrNoRelation
	}
	return r, nil
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
func (ctx *HookContext) hookVars(charmDir, toolsDir, socketPath string) []string {
	vars := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=" + toolsDir + ":" + os.Getenv("PATH"),
		"CHARM_DIR=" + charmDir,
		"JUJU_CONTEXT_ID=" + ctx.Id,
		"JUJU_AGENT_SOCKET=" + socketPath,
		"JUJU_UNIT_NAME=" + ctx.Unit.Name(),
	}
	if ctx.RelationId_ != -1 {
		vars = append(vars, "JUJU_RELATION="+ctx.envRelation())
		vars = append(vars, "JUJU_RELATION_ID="+ctx.envRelationId())
		if ctx.RemoteUnitName_ != "" {
			vars = append(vars, "JUJU_REMOTE_UNIT="+ctx.RemoteUnitName_)
		}
	}
	return vars
}

// RunHook executes a hook in an environment which allows it to to call back
// into ctx to execute jujuc tools.
func (ctx *HookContext) RunHook(hookName, charmDir, toolsDir, socketPath string) error {
	ps := exec.Command(filepath.Join(charmDir, "hooks", hookName))
	ps.Env = ctx.hookVars(charmDir, toolsDir, socketPath)
	ps.Dir = charmDir
	outReader, err := ps.StdoutPipe()
	if err != nil {
		return err
	}
	ps.Stderr = ps.Stdout
	logger := &hookLogger{
		r:    outReader,
		done: make(chan struct{}),
	}
	go logger.run()
	err = ps.Run()
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
	r       io.ReadCloser
	done    chan struct{}
	mu      sync.Mutex
	stopped bool
}

func (l *hookLogger) run() {
	defer close(l.done)
	defer l.r.Close()
	br := bufio.NewReaderSize(l.r, 4096)
	for {
		line, _, err := br.ReadLine()
		if err != nil {
			if err != io.EOF {
				log.Printf("cannot read hook output: %v", err)
			}
			break
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
	// We can see the process exit before the logger has processed
	// all its output, so allow a moment for the data buffered
	// in the pipe to be processed. We don't wait indefinitely though,
	// because the hook may have started a background process
	// that keeps the pipe open.
	select {
	case <-l.done:
	case <-time.After(100 * time.Millisecond):
	}
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
	if ctx.RelationId_ == -1 {
		return ""
	}
	return ctx.Relations[ctx.RelationId_].Name()
}

// envRelationId returns the relation id exposed to hooks as JUJU_RELATION_ID.
// If the context does not have a relation, it will return an empty string.
// Otherwise, it will panic if RelationId is not a key in the Relations map.
func (ctx *HookContext) envRelationId() string {
	if ctx.RelationId_ == -1 {
		return ""
	}
	return ctx.Relations[ctx.RelationId_].FakeId()
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
func NewRelationContext(ru *state.RelationUnit, members map[string]int64) *RelationContext {
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

func (ctx *RelationContext) Name() string {
	return ctx.ru.Endpoint().RelationName
}

func (ctx *RelationContext) FakeId() string {
	return fmt.Sprintf("%s:%d", ctx.Name(), ctx.ru.Relation().Id())
}

func (ctx *RelationContext) UnitNames() (units []string) {
	for unit := range ctx.members {
		units = append(units, unit)
	}
	sort.Strings(units)
	return units
}

func (ctx *RelationContext) Settings() (Settings, error) {
	if ctx.settings == nil {
		node, err := ctx.ru.Settings()
		if err != nil {
			return nil, err
		}
		ctx.settings = node
	}
	return ctx.settings, nil
}

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
	*result = ctx.RelationId_
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
