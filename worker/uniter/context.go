// Copyright 2012-2014 Canonical Ltd.
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

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	utilexec "github.com/juju/utils/exec"
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/version"
	unitdebug "github.com/juju/juju/worker/uniter/debug"
	"github.com/juju/juju/worker/uniter/jujuc"
)

var windowsSuffixOrder = []string{
	".ps1",
	".cmd",
	".bat",
	".exe",
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

// meterStatus describes the unit's meter status.
type meterStatus struct {
	code string
	info string
}

// HookContext is the implementation of jujuc.Context.
type HookContext struct {
	unit *uniter.Unit

	// state is the handle to the uniter State so that HookContext can make
	// API calls on the stateservice.
	// NOTE: We would like to be rid of the fake-remote-Unit and switch
	// over fully to API calls on State.  This adds that ability, but we're
	// not fully there yet.
	state *uniter.State

	// privateAddress is the cached value of the unit's private
	// address.
	privateAddress string

	// publicAddress is the cached value of the unit's public
	// address.
	publicAddress string

	// configSettings holds the service configuration.
	configSettings charm.Settings

	// id identifies the context.
	id string

	// actionData contains the values relevant to the run of an Action:
	// its tag, its parameters, and its results.
	actionData *actionData

	// uuid is the universally unique identifier of the environment.
	uuid string

	// envName is the human friendly name of the environment.
	envName string

	// relationId identifies the relation for which a relation hook is
	// executing. If it is -1, the context is not running a relation hook;
	// otherwise, its value must be a valid key into the relations map.
	relationId int

	// remoteUnitName identifies the changing unit of the executing relation
	// hook. It will be empty if the context is not running a relation hook,
	// or if it is running a relation-broken hook.
	remoteUnitName string

	// relations contains the context for every relation the unit is a member
	// of, keyed on relation id.
	relations map[int]*ContextRelation

	// apiAddrs contains the API server addresses.
	apiAddrs []string

	// serviceOwner contains the user tag of the service owner.
	serviceOwner names.UserTag

	// proxySettings are the current proxy settings that the uniter knows about.
	proxySettings proxy.Settings

	// metrics are the metrics recorded by calls to add-metric.
	metrics []jujuc.Metric

	// canAddMetrics specifies whether the hook allows recording metrics.
	canAddMetrics bool

	// meterStatus is the status of the unit's metering.
	meterStatus *meterStatus
}

func NewHookContext(
	unit *uniter.Unit,
	state *uniter.State,
	id,
	uuid,
	envName string,
	relationId int,
	remoteUnitName string,
	relations map[int]*ContextRelation,
	apiAddrs []string,
	serviceOwner names.UserTag,
	proxySettings proxy.Settings,
	canAddMetrics bool,
	actionData *actionData,
) (*HookContext, error) {
	ctx := &HookContext{
		unit:           unit,
		state:          state,
		id:             id,
		uuid:           uuid,
		envName:        envName,
		relationId:     relationId,
		remoteUnitName: remoteUnitName,
		relations:      relations,
		apiAddrs:       apiAddrs,
		serviceOwner:   serviceOwner,
		proxySettings:  proxySettings,
		canAddMetrics:  canAddMetrics,
		actionData:     actionData,
	}
	// Get and cache the addresses.
	var err error
	ctx.publicAddress, err = unit.PublicAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return nil, err
	}
	ctx.privateAddress, err = unit.PrivateAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return nil, err
	}

	statusCode, statusInfo, err := unit.MeterStatus()
	if err != nil {
		return nil, errors.Annotate(err, "could not retrieve meter status for unit")
	}
	if statusCode != "" {
		ctx.meterStatus = &meterStatus{
			code: statusCode,
			info: statusInfo,
		}
	}

	return ctx, nil
}

func (ctx *HookContext) UnitName() string {
	return ctx.unit.Name()
}

func (ctx *HookContext) PublicAddress() (string, bool) {
	return ctx.publicAddress, ctx.publicAddress != ""
}

func (ctx *HookContext) PrivateAddress() (string, bool) {
	return ctx.privateAddress, ctx.privateAddress != ""
}

func (ctx *HookContext) OpenPorts(protocol string, fromPort, toPort int) error {
	return ctx.unit.OpenPorts(protocol, fromPort, toPort)
}

func (ctx *HookContext) ClosePorts(protocol string, fromPort, toPort int) error {
	return ctx.unit.ClosePorts(protocol, fromPort, toPort)
}

func (ctx *HookContext) OwnerTag() string {
	return ctx.serviceOwner.String()
}

func (ctx *HookContext) ConfigSettings() (charm.Settings, error) {
	if ctx.configSettings == nil {
		var err error
		ctx.configSettings, err = ctx.unit.ConfigSettings()
		if err != nil {
			return nil, err
		}
	}
	result := charm.Settings{}
	for name, value := range ctx.configSettings {
		result[name] = value
	}
	return result, nil
}

// ActionParams simply returns the arguments to the Action.
func (ctx *HookContext) ActionParams() (map[string]interface{}, error) {
	if ctx.actionData == nil {
		return nil, fmt.Errorf("not running an action")
	}
	return ctx.actionData.ActionParams, nil
}

// SetActionMessage sets a message for the Action, usually an error message.
func (ctx *HookContext) SetActionMessage(message string) error {
	if ctx.actionData == nil {
		return fmt.Errorf("not running an action")
	}
	ctx.actionData.ResultsMessage = message
	return nil
}

// SetActionFailed sets the fail state of the action.
func (ctx *HookContext) SetActionFailed() error {
	if ctx.actionData == nil {
		return fmt.Errorf("not running an action")
	}
	ctx.actionData.ActionFailed = true
	return nil
}

// UpdateActionResults inserts new values for use with action-set and
// action-fail.  The results struct will be delivered to the state server
// upon completion of the Action.  It returns an error if not called on an
// Action-containing HookContext.
func (ctx *HookContext) UpdateActionResults(keys []string, value string) error {
	if ctx.actionData == nil {
		return fmt.Errorf("not running an action")
	}
	addValueToMap(keys, value, ctx.actionData.ResultsMap)
	return nil
}

func (ctx *HookContext) HookRelation() (jujuc.ContextRelation, bool) {
	return ctx.Relation(ctx.relationId)
}

func (ctx *HookContext) RemoteUnitName() (string, bool) {
	return ctx.remoteUnitName, ctx.remoteUnitName != ""
}

func (ctx *HookContext) Relation(id int) (jujuc.ContextRelation, bool) {
	r, found := ctx.relations[id]
	return r, found
}

func (ctx *HookContext) RelationIds() []int {
	ids := []int{}
	for id := range ctx.relations {
		ids = append(ids, id)
	}
	return ids
}

// AddMetrics adds metrics to the hook context.
func (ctx *HookContext) AddMetrics(key, value string, created time.Time) error {
	if !ctx.canAddMetrics {
		return fmt.Errorf("metrics disabled")
	}
	ctx.metrics = append(ctx.metrics, jujuc.Metric{key, value, created})
	return nil
}

// mergeEnvironment takes in a string array representing the desired environment
// and merges it with the current environment. On Windows, clearing the environment,
// or having missing environment variables, may lead to standard go packages not working
// (os.TempDir relies on $env:TEMP), and powershell erroring out
// Currently this function is only used for windows
func mergeEnvironment(env []string) []string {
	if env == nil {
		return nil
	}
	m := map[string]string{}
	var tmpEnv []string
	for _, val := range os.Environ() {
		varSplit := strings.SplitN(val, "=", 2)
		m[varSplit[0]] = varSplit[1]
	}

	for _, val := range env {
		varSplit := strings.SplitN(val, "=", 2)
		m[varSplit[0]] = varSplit[1]
	}

	for key, val := range m {
		tmpEnv = append(tmpEnv, key+"="+val)
	}

	return tmpEnv
}

// windowsEnv adds windows specific environment variables. PSModulePath
// helps hooks use normal imports instead of dot sourcing modules
// its a convenience variable. The PATH variable delimiter is
// a semicolon instead of a colon
func (ctx *HookContext) windowsEnv(charmDir, toolsDir string) []string {
	charmModules := filepath.Join(charmDir, "Modules")
	hookModules := filepath.Join(charmDir, "hooks", "Modules")
	env := []string{
		"Path=" + filepath.FromSlash(toolsDir) + ";" + os.Getenv("Path"),
		"PSModulePath=" + os.Getenv("PSModulePath") + ";" + charmModules + ";" + hookModules,
	}
	return mergeEnvironment(env)
}

func (ctx *HookContext) ubuntuEnv(toolsDir string) []string {
	env := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=" + toolsDir + ":" + os.Getenv("PATH"),
	}
	return env
}

func (ctx *HookContext) osDependentEnvVars(charmDir, toolsDir string) []string {
	switch version.Current.OS {
	case version.Windows:
		return ctx.windowsEnv(charmDir, toolsDir)
	default:
		return ctx.ubuntuEnv(toolsDir)
	}
}

// hookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into ctx.
func (ctx *HookContext) hookVars(charmDir, toolsDir, socketPath string) []string {
	// TODO(binary132): add Action env variables: JUJU_ACTION_NAME,
	// JUJU_ACTION_UUID, ...
	vars := []string{
		"CHARM_DIR=" + charmDir,
		"JUJU_CONTEXT_ID=" + ctx.id,
		"JUJU_AGENT_SOCKET=" + socketPath,
		"JUJU_UNIT_NAME=" + ctx.unit.Name(),
		"JUJU_ENV_UUID=" + ctx.uuid,
		"JUJU_ENV_NAME=" + ctx.envName,
		"JUJU_API_ADDRESSES=" + strings.Join(ctx.apiAddrs, " "),
	}
	osVars := ctx.osDependentEnvVars(charmDir, toolsDir)
	vars = append(vars, osVars...)

	if r, found := ctx.HookRelation(); found {
		vars = append(vars, "JUJU_RELATION="+r.Name())
		vars = append(vars, "JUJU_RELATION_ID="+r.FakeId())
		name, _ := ctx.RemoteUnitName()
		vars = append(vars, "JUJU_REMOTE_UNIT="+name)
	}
	vars = append(vars, ctx.proxySettings.AsEnvironmentValues()...)
	vars = append(vars, ctx.meterStatusEnvVars()...)
	return vars
}

// meterStatusEnvVars returns meter status environment variables if the meter
// status is set.
func (ctx *HookContext) meterStatusEnvVars() []string {
	if ctx.meterStatus != nil {
		return []string{
			fmt.Sprintf("JUJU_METER_STATUS=%s", ctx.meterStatus.code),
			fmt.Sprintf("JUJU_METER_INFO=%s", ctx.meterStatus.info)}
	}
	return nil
}

func (ctx *HookContext) finalizeContext(process string, ctxErr error) (err error) {
	writeChanges := ctxErr == nil

	// In the case of Actions, handle any errors using finalizeAction.
	if ctx.actionData != nil {
		// If we had an error in err at this point, it's part of the
		// normal behavior of an Action.  Errors which happen during
		// the finalize should be handed back to the uniter.  Close
		// over the existing err, clear it, and only return errors
		// which occur during the finalize, e.g. API call errors.
		defer func(ctxErr error) {
			err = ctx.finalizeAction(ctxErr, err)
		}(ctxErr)
		ctxErr = nil
	}

	for id, rctx := range ctx.relations {
		if writeChanges {
			if e := rctx.WriteSettings(); e != nil {
				e = fmt.Errorf(
					"could not write settings from %q to relation %d: %v",
					process, id, e,
				)
				logger.Errorf("%v", e)
				if ctxErr == nil {
					ctxErr = e
				}
			}
		}
		rctx.ClearCache()
	}

	if ctxErr != nil {
		return ctxErr
	}

	// TODO (tasdomas) 2014 09 03: context finalization needs to modified to apply all
	//                             changes in one api call to minimize the risk
	//                             of partial failures.
	if ctx.canAddMetrics && len(ctx.metrics) > 0 {
		if writeChanges {
			metrics := make([]params.Metric, len(ctx.metrics))
			for i, metric := range ctx.metrics {
				metrics[i] = params.Metric{Key: metric.Key, Value: metric.Value, Time: metric.Time}
			}
			if e := ctx.unit.AddMetrics(metrics); e != nil {
				logger.Errorf("%v", e)
				if ctxErr == nil {
					ctxErr = e
				}
			}
		}
		ctx.metrics = nil
	}

	return ctxErr
}

// finalizeAction passes back the final status of an Action hook to state.
// It wraps any errors which occurred in normal behavior of the Action run;
// only errors passed in unhandledErr will be returned.
func (ctx *HookContext) finalizeAction(err, unhandledErr error) error {
	// TODO (binary132): synchronize with gsamfira's reboot logic
	message := ctx.actionData.ResultsMessage
	results := ctx.actionData.ResultsMap
	tag := ctx.actionData.ActionTag
	status := params.ActionCompleted
	if ctx.actionData.ActionFailed {
		status = params.ActionFailed
	}

	// If we had an action error, we'll simply encapsulate it in the response
	// and discard the error state.  Actions should not error the uniter.
	if err != nil {
		message = err.Error()
		if IsMissingHookError(err) {
			message = fmt.Sprintf("action not implemented on unit %q", ctx.UnitName())
		}
		status = params.ActionFailed
	}

	callErr := ctx.state.ActionFinish(tag, status, results, message)
	if callErr != nil {
		unhandledErr = errors.Wrap(unhandledErr, callErr)
	}
	return unhandledErr
}

// RunCommands executes the commands in an environment which allows it to to
// call back into the hook context to execute jujuc tools.
func (ctx *HookContext) RunCommands(commands, charmDir, toolsDir, socketPath string) (*utilexec.ExecResponse, error) {
	env := ctx.hookVars(charmDir, toolsDir, socketPath)
	result, err := utilexec.RunCommands(
		utilexec.RunParams{
			Commands:    commands,
			WorkingDir:  charmDir,
			Environment: env})
	return result, ctx.finalizeContext("run commands", err)
}

func (ctx *HookContext) GetLogger(hookName string) loggo.Logger {
	return loggo.GetLogger(fmt.Sprintf("unit.%s.%s", ctx.UnitName(), hookName))
}

// RunAction executes a hook from the charm's actions in an environment which
// allows it to to call back into the hook context to execute jujuc tools.
func (ctx *HookContext) RunAction(hookName, charmDir, toolsDir, socketPath string) error {
	if ctx.actionData == nil {
		return fmt.Errorf("not running an action")
	}
	// If the action had already failed (i.e. from invalid params), we
	// just want to finalize without running it.
	if ctx.actionData.ActionFailed {
		return ctx.finalizeContext(hookName, nil)
	}
	return ctx.runCharmHookWithLocation(hookName, "actions", charmDir, toolsDir, socketPath)
}

// RunHook executes a built-in hook in an environment which allows it to to
// call back into the hook context to execute jujuc tools.
func (ctx *HookContext) RunHook(hookName, charmDir, toolsDir, socketPath string) error {
	return ctx.runCharmHookWithLocation(hookName, "hooks", charmDir, toolsDir, socketPath)
}

func (ctx *HookContext) runCharmHookWithLocation(hookName, charmLocation, charmDir, toolsDir, socketPath string) error {
	var err error
	env := ctx.hookVars(charmDir, toolsDir, socketPath)
	debugctx := unitdebug.NewHooksContext(ctx.unit.Name())
	if session, _ := debugctx.FindSession(); session != nil && session.MatchHook(hookName) {
		logger.Infof("executing %s via debug-hooks", hookName)
		err = session.RunHook(hookName, charmDir, env)
	} else {
		err = ctx.runCharmHook(hookName, charmDir, env, charmLocation)
	}
	return ctx.finalizeContext(hookName, err)
}

func lookPath(hook string) (string, error) {
	hookFile, err := exec.LookPath(hook)
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && os.IsNotExist(ee.Err) {
			return "", &missingHookError{hook}
		}
		return "", err
	}
	return hookFile, nil
}

// searchHook will search, in order, hooks suffixed with extensions
// in windowsSuffixOrder. As windows cares about extensions to determine
// how to execute a file, we will allow several suffixes, with powershell
// being default.
func searchHook(charmDir, hook string) (string, error) {
	hookFile := filepath.Join(charmDir, hook)
	if version.Current.OS != version.Windows {
		// we are not running on windows,
		// there is no need to look for suffixed hooks
		return lookPath(hookFile)
	}
	for _, val := range windowsSuffixOrder {
		file := fmt.Sprintf("%s%s", hookFile, val)
		foundHook, err := lookPath(file)
		if err != nil {
			if IsMissingHookError(err) {
				// look for next suffix
				continue
			}
			return "", err
		}
		return foundHook, nil
	}
	return "", &missingHookError{hook}
}

// hookCommand constructs an appropriate command to be passed to
// exec.Command(). The exec package uses cmd.exe as default on windows
// cmd.exe does not know how to execute ps1 files by default, and
// powershell needs a few flags to allow execution (-ExecutionPolicy)
// and propagate error levels (-File). .cmd and .bat files can be run directly
func hookCommand(hook string) []string {
	if version.Current.OS != version.Windows {
		// we are not running on windows,
		// just return the hook name
		return []string{hook}
	}
	if strings.HasSuffix(hook, ".ps1") {
		return []string{
			"powershell.exe",
			"-NonInteractive",
			"-ExecutionPolicy",
			"RemoteSigned",
			"-File",
			hook,
		}
	}
	return []string{hook}
}

func (ctx *HookContext) runCharmHook(hookName, charmDir string, env []string, charmLocation string) error {
	hook, err := searchHook(charmDir, filepath.Join(charmLocation, hookName))
	if err != nil {
		if IsMissingHookError(err) {
			// Missing hook is perfectly valid, but worth mentioning.
			logger.Infof("skipped %q hook (not implemented)", hookName)
		}
		return err
	}
	hookCmd := hookCommand(hook)
	ps := exec.Command(hookCmd[0], hookCmd[1:]...)
	ps.Env = env
	ps.Dir = charmDir
	outReader, outWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("cannot make logging pipe: %v", err)
	}
	ps.Stdout = outWriter
	ps.Stderr = outWriter
	hookLogger := &hookLogger{
		r:      outReader,
		done:   make(chan struct{}),
		logger: ctx.GetLogger(hookName),
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

// actionData contains the tag, parameters, and results of an Action.
type actionData struct {
	ActionTag      names.ActionTag
	ActionParams   map[string]interface{}
	ActionFailed   bool
	ResultsMessage string
	ResultsMap     map[string]interface{}
}

// newActionData builds a suitable actionData struct with no nil members.
// this should only be called in the event that an Action hook is being requested.
func newActionData(tag *names.ActionTag, params map[string]interface{}) *actionData {
	return &actionData{
		ActionTag:    *tag,
		ActionParams: params,
		ResultsMap:   map[string]interface{}{},
	}
}

// actionStatus messages define the possible states of a completed Action.
const (
	actionStatusInit   = "init"
	actionStatusFailed = "fail"
)

// addValueToMap adds the given value to the map on which the method is run.
// This allows us to merge maps such as {foo: {bar: baz}} and {foo: {baz: faz}}
// into {foo: {bar: baz, baz: faz}}.
func addValueToMap(keys []string, value string, target map[string]interface{}) {
	next := target

	for i := range keys {
		// if we are on last key set the value.
		// shouldn't be a problem.  overwrites existing vals.
		if i == len(keys)-1 {
			next[keys[i]] = value
			break
		}

		if iface, ok := next[keys[i]]; ok {
			switch typed := iface.(type) {
			case map[string]interface{}:
				// If we already had a map inside, keep
				// stepping through.
				next = typed
			default:
				// If we didn't, then overwrite value
				// with a map and iterate with that.
				m := map[string]interface{}{}
				next[keys[i]] = m
				next = m
			}
			continue
		}

		// Otherwise, it wasn't present, so make it and step
		// into.
		m := map[string]interface{}{}
		next[keys[i]] = m
		next = m
	}
}
