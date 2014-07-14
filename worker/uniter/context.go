// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juju/charm"
	"github.com/juju/charm/hooks"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/uniter"
	unitdebug "github.com/juju/juju/worker/uniter/debug"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/jujuc"
	"github.com/juju/loggo"
	utilexec "github.com/juju/utils/exec"
)

// HookContext contains a handle to the HookRunner and HookInfo.  It uses these
// to implement jujuc.Context.
type HookContext struct {
	runner        *HookRunner
	id            string
	info          *hook.Info
	hookName      string
	hookPath      string
	actionResults map[string]interface{}
	actionParams  map[string]interface{}
}

type missingHookError struct {
	hookName string
}

func (e *missingHookError) Error() string {
	return e.hookName + " does not exist"
}

func IsMissingHookError(err error) bool {
	_, ok := err.(*missingHookError)
	return ok
}

// NewHookContext builds a HookContext for the named hook.  If the Hook is of
// the RunCommands type (a generic context which requires no hook in the charm),
// simply put it together without a hook path.  Otherwise, check the path to
// make sure the hook is implemented before trying to run it.
func NewHookContext(hr *HookRunner, id string, hi *hook.Info, hookName string,
	actionParams map[string]interface{}) (*HookContext, error) {

	newContext := &HookContext{
		runner:       hr,
		id:           id,
		info:         hi,
		hookName:     hookName,
		actionParams: actionParams,
	}
	// for generic commands, no further work is needed for a complete context.
	if hi.Kind == hooks.RunCommands {
		return newContext, nil
	}

	// otherwise, take a look to make sure the hook is there, and set the
	// hookPath for the context.
	hookLocation := "hooks"
	if hi.Kind == hooks.ActionRequested {
		hookLocation = "actions"
	}
	hookPath, err := exec.LookPath(filepath.Join(hr.charmDir, hookLocation, hookName))
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && os.IsNotExist(ee.Err) {
			// Missing hook is perfectly valid, but worth mentioning.
			logger.Infof("hook %q not implemented", hookName)
			return nil, &missingHookError{hookName}
		}
		return nil, err
	}

	// If everything checked out, we have our hook path.
	newContext.hookPath = hookPath

	return newContext, nil
}

func (ctx *HookContext) UnitName() string {
	return ctx.runner.unit.Name()
}

func (ctx *HookContext) PublicAddress() (string, bool) {
	return ctx.runner.publicAddress, ctx.runner.publicAddress != ""
}

func (ctx *HookContext) PrivateAddress() (string, bool) {
	return ctx.runner.privateAddress, ctx.runner.privateAddress != ""
}

func (ctx *HookContext) OpenPort(protocol string, port int) error {
	return ctx.runner.unit.OpenPort(protocol, port)
}

func (ctx *HookContext) ClosePort(protocol string, port int) error {
	return ctx.runner.unit.ClosePort(protocol, port)
}

func (ctx *HookContext) OwnerTag() string {
	return ctx.runner.serviceOwner
}

func (ctx *HookContext) ConfigSettings() (charm.Settings, error) {
	return ctx.runner.ConfigSettings()
}

func (ctx *HookContext) ActionParams() map[string]interface{} {
	return ctx.actionParams
}

func (ctx *HookContext) HookRelation() (jujuc.ContextRelation, bool) {
	if ctx.info.Kind.IsRelation() {
		return ctx.Relation(ctx.info.RelationId)
	}
	return ctx.Relation(-1)
}

func (ctx *HookContext) RemoteUnitName() (string, bool) {
	return ctx.info.RemoteUnit, ctx.info.RemoteUnit != ""
}

func (ctx *HookContext) Relation(id int) (jujuc.ContextRelation, bool) {
	r, found := ctx.runner.relations[id]
	return r, found
}

func (ctx *HookContext) RelationIds() []int {
	ids := []int{}
	for id := range ctx.runner.relations {
		ids = append(ids, id)
	}
	return ids
}

// hookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into ctx.
func (ctx *HookContext) hookVars(socketPath string) []string {
	vars := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=" + ctx.runner.toolsDir + ":" + os.Getenv("PATH"),
		"CHARM_DIR=" + ctx.runner.charmDir,
		"JUJU_CONTEXT_ID=" + ctx.id,
		"JUJU_AGENT_SOCKET=" + socketPath,
		"JUJU_UNIT_NAME=" + ctx.runner.unit.Name(),
		"JUJU_ENV_UUID=" + ctx.runner.uuid,
		"JUJU_ENV_NAME=" + ctx.runner.envName,
		"JUJU_API_ADDRESSES=" + strings.Join(ctx.runner.apiAddrs, " "),
	}
	if r, found := ctx.HookRelation(); found {
		vars = append(vars, "JUJU_RELATION="+r.Name())
		vars = append(vars, "JUJU_RELATION_ID="+r.FakeId())
		name, _ := ctx.RemoteUnitName()
		vars = append(vars, "JUJU_REMOTE_UNIT="+name)
	}
	vars = append(vars, ctx.runner.proxySettings.AsEnvironmentValues()...)
	return vars
}

func (ctx *HookContext) finalizeContext(process string, err error) error {
	writeChanges := err == nil
	for id, rctx := range ctx.runner.relations {
		if writeChanges {
			if e := rctx.WriteSettings(); e != nil {
				e = fmt.Errorf(
					"could not write settings from %q to relation %d: %v",
					process, id, e,
				)
				logger.Errorf("%v", e)
				if err == nil {
					err = e
				}
			}
		}
		rctx.ClearCache()
	}
	return err
}

// RunCommands executes the commands in an environment which allows it to to
// call back into the hook context to execute jujuc tools.
func (ctx *HookContext) RunCommands(commands, socketPath string) (*utilexec.ExecResponse, error) {
	env := ctx.hookVars(socketPath)
	result, err := utilexec.RunCommands(
		utilexec.RunParams{
			Commands:    commands,
			WorkingDir:  ctx.runner.charmDir,
			Environment: env})
	return result, ctx.finalizeContext("run commands", err)
}

func (ctx *HookContext) GetLogger() loggo.Logger {
	return loggo.GetLogger(fmt.Sprintf("unit.%s.%s", ctx.UnitName(), ctx.hookName))
}

// RunHook executes a built-in hook in an environment which allows it to to
// call back into the hook context to execute jujuc tools.
func (ctx *HookContext) RunHook(socketPath string) error {
	var err error
	debugctx := unitdebug.NewHooksContext(ctx.runner.unit.Name())
	if session, _ := debugctx.FindSession(); session != nil && session.MatchHook(ctx.hookName) {
		logger.Infof("executing %s via debug-hooks", ctx.hookName)
		err = session.RunHook(ctx.hookName, ctx.runner.charmDir, ctx.hookVars(socketPath))
	} else {
		err = ctx.runCharmHook(socketPath)
	}
	return ctx.finalizeContext(ctx.hookName, err)
}

func (ctx *HookContext) runCharmHook(socketPath string) error {
	ps := exec.Command(ctx.hookPath)
	ps.Env = ctx.hookVars(socketPath)
	ps.Dir = ctx.runner.charmDir
	outReader, outWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("cannot make logging pipe: %v", err)
	}
	ps.Stdout = outWriter
	ps.Stderr = outWriter
	hookLogger := &hookLogger{
		r:      outReader,
		done:   make(chan struct{}),
		logger: ctx.GetLogger(),
	}
	go hookLogger.run()
	err = ps.Start()
	outWriter.Close()
	if err == nil {
		err = ps.Wait()
	}
	hookLogger.stop()
	return err
}

type hookLogger struct {
	r       io.ReadCloser
	done    chan struct{}
	mu      sync.Mutex
	stopped bool
	logger  loggo.Logger
}

func (l *hookLogger) run() {
	defer close(l.done)
	defer l.r.Close()
	br := bufio.NewReaderSize(l.r, 4096)
	for {
		line, _, err := br.ReadLine()
		if err != nil {
			if err != io.EOF {
				logger.Errorf("cannot read hook output: %v", err)
			}
			break
		}
		l.mu.Lock()
		if l.stopped {
			l.mu.Unlock()
			return
		}
		l.logger.Infof("%s", line)
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

// SettingsMap is a map from unit name to relation settings.
type SettingsMap map[string]params.RelationSettings

// ContextRelation is the implementation of jujuc.ContextRelation.
type ContextRelation struct {
	ru *uniter.RelationUnit

	// members contains settings for known relation members. Nil values
	// indicate members whose settings have not yet been cached.
	members SettingsMap

	// settings allows read and write access to the relation unit settings.
	settings *uniter.Settings

	// cache is a short-term cache that enables consistent access to settings
	// for units that are not currently participating in the relation. Its
	// contents should be cleared whenever a new hook is executed.
	cache SettingsMap
}

// NewContextRelation creates a new context for the given relation unit.
// The unit-name keys of members supplies the initial membership.
func NewContextRelation(ru *uniter.RelationUnit, members map[string]int64) *ContextRelation {
	ctx := &ContextRelation{ru: ru, members: SettingsMap{}}
	for unit := range members {
		ctx.members[unit] = nil
	}
	ctx.ClearCache()
	return ctx
}

// WriteSettings persists all changes made to the unit's relation settings.
func (ctx *ContextRelation) WriteSettings() (err error) {
	if ctx.settings != nil {
		err = ctx.settings.Write()
	}
	return
}

// ClearCache discards all cached settings for units that are not members
// of the relation, and all unwritten changes to the unit's relation settings.
// including any changes to Settings that have not been written.
func (ctx *ContextRelation) ClearCache() {
	ctx.settings = nil
	ctx.cache = make(SettingsMap)
}

// UpdateMembers ensures that the context is aware of every supplied
// member unit. For each supplied member, the cached settings will be
// overwritten.
func (ctx *ContextRelation) UpdateMembers(members SettingsMap) {
	for m, s := range members {
		ctx.members[m] = s
	}
}

// DeleteMember drops the membership and cache of a single remote unit, without
// perturbing settings for the remaining members.
func (ctx *ContextRelation) DeleteMember(unitName string) {
	delete(ctx.members, unitName)
}

func (ctx *ContextRelation) Id() int {
	return ctx.ru.Relation().Id()
}

func (ctx *ContextRelation) Name() string {
	return ctx.ru.Endpoint().Name
}

func (ctx *ContextRelation) FakeId() string {
	return fmt.Sprintf("%s:%d", ctx.Name(), ctx.ru.Relation().Id())
}

func (ctx *ContextRelation) UnitNames() (units []string) {
	for unit := range ctx.members {
		units = append(units, unit)
	}
	sort.Strings(units)
	return units
}

func (ctx *ContextRelation) Settings() (jujuc.Settings, error) {
	if ctx.settings == nil {
		node, err := ctx.ru.Settings()
		if err != nil {
			return nil, err
		}
		ctx.settings = node
	}
	return ctx.settings, nil
}

func (ctx *ContextRelation) ReadSettings(unit string) (settings params.RelationSettings, err error) {
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
