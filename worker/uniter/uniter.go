// Copyright 2012-2014 Canonical Ltd.

// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	stderrors "errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/fslock"
	corecharm "gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/uniter"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/relation"
)

var logger = loggo.GetLogger("juju.worker.uniter")

const (
	// interval at which the unit's metrics should be collected
	metricsPollInterval = 5 * time.Minute
)

// A UniterExecutionObserver gets the appropriate methods called when a hook
// is executed and either succeeds or fails.  Missing hooks don't get reported
// in this way.
type UniterExecutionObserver interface {
	HookCompleted(hookName string)
	HookFailed(hookName string)
}

// collectMetricsTimer returns a channel that will signal the collect metrics hook
// as close to metricsPollInterval after the last run as possible.
func collectMetricsTimer(now, lastRun time.Time, interval time.Duration) <-chan time.Time {
	waitDuration := interval - now.Sub(lastRun)
	logger.Debugf("waiting for %v", waitDuration)
	return time.After(waitDuration)
}

// collectMetricsAt defines a function that will be used to generate signals
// for the collect-metrics hook. It will be replaced in tests.
var collectMetricsAt func(now, lastSignal time.Time, interval time.Duration) <-chan time.Time = collectMetricsTimer

// Uniter implements the capabilities of the unit agent. It is not intended to
// implement the actual *behaviour* of the unit agent; that responsibility is
// delegated to Mode values, which are expected to react to events and direct
// the uniter's responses to them.
type Uniter struct {
	tomb          tomb.Tomb
	st            *uniter.State
	f             *filter
	unit          *uniter.Unit
	service       *uniter.Service
	relationers   map[int]*Relationer
	relationHooks chan hook.Info

	paths              Paths
	deployer           charm.Deployer
	operationState     *operation.State
	operationStateFile *operation.StateFile
	contextFactory     context.Factory
	hookLock           *fslock.Lock
	runListener        *RunListener

	ranConfigChanged bool

	// The execution observer is only used in tests at this stage. Should this
	// need to be extended, perhaps a list of observers would be needed.
	observer UniterExecutionObserver
}

// NewUniter creates a new Uniter which will install, run, and upgrade
// a charm on behalf of the unit with the given unitTag, by executing
// hooks and operations provoked by changes in st.
func NewUniter(st *uniter.State, unitTag names.UnitTag, dataDir string, hookLock *fslock.Lock) *Uniter {
	u := &Uniter{
		st:       st,
		paths:    NewPaths(dataDir, unitTag),
		hookLock: hookLock,
	}
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.loop(unitTag))
	}()
	return u
}

func (u *Uniter) loop(unitTag names.UnitTag) (err error) {
	if err := u.init(unitTag); err != nil {
		if err == worker.ErrTerminateAgent {
			return err
		}
		return fmt.Errorf("failed to initialize uniter for %q: %v", unitTag, err)
	}
	defer u.runListener.Close()
	logger.Infof("unit %q started", u.unit)

	environWatcher, err := u.st.WatchForEnvironConfigChanges()
	if err != nil {
		return err
	}
	defer watcher.Stop(environWatcher, &u.tomb)
	u.watchForProxyChanges(environWatcher)

	// Start filtering state change events for consumption by modes.
	u.f, err = newFilter(u.st, unitTag)
	if err != nil {
		return err
	}
	defer watcher.Stop(u.f, &u.tomb)
	go func() {
		u.tomb.Kill(u.f.Wait())
	}()

	// Run modes until we encounter an error.
	mode := ModeContinue
	for err == nil {
		select {
		case <-u.tomb.Dying():
			err = tomb.ErrDying
		default:
			mode, err = mode(u)
		}
	}
	logger.Infof("unit %q shutting down: %s", u.unit, err)
	return err
}

func (u *Uniter) setupLocks() (err error) {
	if message := u.hookLock.Message(); u.hookLock.IsLocked() && message != "" {
		// Look to see if it was us that held the lock before.  If it was, we
		// should be safe enough to break it, as it is likely that we died
		// before unlocking, and have been restarted by upstart.
		parts := strings.SplitN(message, ":", 2)
		if len(parts) > 1 && parts[0] == u.unit.Name() {
			if err := u.hookLock.BreakLock(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (u *Uniter) init(unitTag names.UnitTag) (err error) {
	u.unit, err = u.st.Unit(unitTag)
	if err != nil {
		return err
	}
	if u.unit.Life() == params.Dead {
		// If we started up already dead, we should not progress further. If we
		// become Dead immediately after starting up, we may well complete any
		// operations in progress before detecting it; but that race is fundamental
		// and inescapable, whereas this one is not.
		return worker.ErrTerminateAgent
	}
	if err = u.setupLocks(); err != nil {
		return err
	}
	if err := EnsureJujucSymlinks(u.paths.ToolsDir); err != nil {
		return err
	}
	if err := os.MkdirAll(u.paths.State.RelationsDir, 0755); err != nil {
		return err
	}
	u.service, err = u.st.Service(u.unit.ServiceTag())
	if err != nil {
		return err
	}

	u.relationers = map[int]*Relationer{}
	u.relationHooks = make(chan hook.Info)
	u.deployer, err = charm.NewDeployer(
		u.paths.State.CharmDir,
		u.paths.State.DeployerDir,
		charm.NewBundlesDir(u.paths.State.BundlesDir),
	)
	if err != nil {
		return fmt.Errorf("cannot create deployer: %v", err)
	}
	u.operationStateFile = operation.NewStateFile(u.paths.State.OperationsFile)

	// If we start trying to listen for juju-run commands before we have valid
	// relation state, surprising things will come to pass.
	if err := u.restoreRelations(); err != nil {
		return err
	}

	u.contextFactory, err = context.NewFactory(u.st, unitTag, u.getRelationInfos)
	if err != nil {
		return err
	}

	logger.Debugf("starting juju-run listener on unix:%s", u.paths.Runtime.JujuRunSocket)
	u.runListener, err = NewRunListener(u, u.paths.Runtime.JujuRunSocket)
	if err != nil {
		return err
	}
	// The socket needs to have permissions 777 in order for other users to use it.
	if version.Current.OS != version.Windows {
		return os.Chmod(u.paths.Runtime.JujuRunSocket, 0777)
	}
	return nil
}

func (u *Uniter) Kill() {
	u.tomb.Kill(nil)
}

func (u *Uniter) Wait() error {
	return u.tomb.Wait()
}

func (u *Uniter) Stop() error {
	u.tomb.Kill(nil)
	return u.Wait()
}

func (u *Uniter) Dead() <-chan struct{} {
	return u.tomb.Dead()
}

// writeOperationState saves uniter state with the supplied values, inferring
// the appropriate values of Started and CollectMetricsTime.
func (u *Uniter) writeOperationState(kind operation.Kind, step operation.Step, hi *hook.Info, url *corecharm.URL) error {
	var collectMetricsTime int64 = 0
	if hi != nil && hi.Kind == hooks.CollectMetrics && step == operation.Done {
		// update collectMetricsTime if the collect-metrics hook was run
		collectMetricsTime = time.Now().Unix()
	} else if u.operationState != nil {
		// or preserve existing value
		collectMetricsTime = u.operationState.CollectMetricsTime
	}

	reachedStartHook := false
	if kind == operation.RunHook && hi.Kind == hooks.Start {
		reachedStartHook = true
	} else if u.operationState != nil && u.operationState.Started {
		reachedStartHook = true
	}
	operationState := operation.State{
		Started:            reachedStartHook,
		Kind:               kind,
		Step:               step,
		Hook:               hi,
		CharmURL:           url,
		CollectMetricsTime: collectMetricsTime,
	}
	if err := u.operationStateFile.Write(
		operationState.Started,
		operationState.Kind,
		operationState.Step,
		operationState.Hook,
		operationState.CharmURL,
		operationState.CollectMetricsTime,
	); err != nil {
		return err
	}
	u.operationState = &operationState
	return nil
}

// deploy deploys the supplied charm URL, and sets follow-up hook operation state
// as indicated by reason.
func (u *Uniter) deploy(curl *corecharm.URL, reason operation.Kind) error {
	if reason != operation.Install && reason != operation.Upgrade {
		panic(fmt.Errorf("%q is not a deploy operation", reason))
	}
	var hi *hook.Info
	if u.operationState != nil {
		// If this upgrade interrupts a RunHook, we need to preserve the hook
		// info so that we can return to the appropriate error state. However,
		// if we're resuming (or have force-interrupted) an Upgrade, we also
		// need to preserve whatever hook info was preserved when we initially
		// started upgrading, to ensure we still return to the correct state.
		kind := u.operationState.Kind
		if kind == operation.RunHook || kind == operation.Upgrade {
			hi = u.operationState.Hook
		}
	}
	if u.operationState == nil || u.operationState.Step != operation.Done {
		// Get the new charm bundle before announcing intention to use it.
		logger.Infof("fetching charm %q", curl)
		sch, err := u.st.Charm(curl)
		if err != nil {
			return err
		}
		if err = u.deployer.Stage(sch, u.tomb.Dying()); err != nil {
			return err
		}

		// Set the new charm URL - this returns when the operation is complete,
		// at which point we can refresh the local copy of the unit to get a
		// version with the correct charm URL, and can go ahead and deploy
		// the charm proper.
		if err := u.f.SetCharm(curl); err != nil {
			return err
		}
		if err := u.unit.Refresh(); err != nil {
			return err
		}
		logger.Infof("deploying charm %q", curl)
		if err = u.writeOperationState(reason, operation.Pending, hi, curl); err != nil {
			return err
		}
		if err = u.deployer.Deploy(); err != nil {
			return err
		}
		if err = u.writeOperationState(reason, operation.Done, hi, curl); err != nil {
			return err
		}
	}
	logger.Infof("charm %q is deployed", curl)
	status := operation.Queued
	if hi != nil {
		// If a hook operation was interrupted, restore it.
		status = operation.Pending
	} else {
		// Otherwise, queue the relevant post-deploy hook.
		hi = &hook.Info{}
		switch reason {
		case operation.Install:
			hi.Kind = hooks.Install
		case operation.Upgrade:
			hi.Kind = hooks.UpgradeCharm
		}
	}
	return u.writeOperationState(operation.RunHook, status, hi, nil)
}

// errHookFailed indicates that a hook failed to execute, but that the Uniter's
// operation is not affected by the error.
var errHookFailed = stderrors.New("hook execution failed")

func (u *Uniter) getRelationInfos() map[int]*context.RelationInfo {
	relationInfos := map[int]*context.RelationInfo{}
	for id, r := range u.relationers {
		relationInfos[id] = r.ContextInfo()
	}
	return relationInfos
}

func (u *Uniter) acquireHookLock(message string) (err error) {
	// We want to make sure we don't block forever when locking, but take the
	// tomb into account.
	checkTomb := func() error {
		select {
		case <-u.tomb.Dying():
			return tomb.ErrDying
		default:
			// no-op to fall through to return.
		}
		return nil
	}
	if err = u.hookLock.LockWithFunc(message, checkTomb); err != nil {
		return err
	}
	return nil
}

// RunCommands executes the supplied commands in a hook context.
func (u *Uniter) RunCommands(commands string) (results *exec.ExecResponse, err error) {
	logger.Tracef("run commands: %s", commands)
	lockMessage := fmt.Sprintf("%s: running commands", u.unit.Name())
	if err = u.acquireHookLock(lockMessage); err != nil {
		return nil, err
	}
	defer u.hookLock.Unlock()

	hctx, err := u.contextFactory.NewRunContext()
	if err != nil {
		return nil, err
	}
	result, err := context.NewRunner(hctx, u.paths).RunCommands(commands)
	if result != nil {
		logger.Tracef("run commands: rc=%v\nstdout:\n%sstderr:\n%s", result.Code, result.Stdout, result.Stderr)
	}
	return result, err
}

func (u *Uniter) notifyHookInternal(hook string, hctx *context.HookContext, method func(string)) {
	if r, ok := hctx.HookRelation(); ok {
		remote, _ := hctx.RemoteUnitName()
		if remote != "" {
			remote = " " + remote
		}
		hook = hook + remote + " " + r.FakeId()
	}
	method(hook)
}

func (u *Uniter) notifyHookCompleted(hook string, hctx *context.HookContext) {
	if u.observer != nil {
		u.notifyHookInternal(hook, hctx, u.observer.HookCompleted)
	}
}

func (u *Uniter) notifyHookFailed(hook string, hctx *context.HookContext) {
	if u.observer != nil {
		u.notifyHookInternal(hook, hctx, u.observer.HookFailed)
	}
}

// validateAction validates the given Action params against the spec defined
// for the charm.
func (u *Uniter) validateAction(name string, params map[string]interface{}) (bool, error) {
	ch, err := corecharm.ReadCharm(u.paths.State.CharmDir)
	if err != nil {
		return false, err
	}

	// Note that ch.Actions() will never be nil, rather an empty struct.
	actionSpecs := ch.Actions()

	spec, ok := actionSpecs.ActionSpecs[name]
	if !ok {
		return false, fmt.Errorf("no spec was defined for action %q", name)
	}

	return spec.ValidateParams(params)
}

// runAction executes the supplied hook.Info as an Action.
func (u *Uniter) runAction(hi hook.Info) (err error) {
	if err = hi.Validate(); err != nil {
		return err
	}

	tag := names.NewActionTag(hi.ActionId)
	action, err := u.st.Action(tag)
	if err != nil {
		return err
	}

	actionParams := action.Params()
	actionName := action.Name()
	_, actionParamsErr := u.validateAction(actionName, actionParams)
	if actionParamsErr != nil {
		actionParamsErr = errors.Annotatef(actionParamsErr, "action %q param validation failed", actionName)
	}

	lockMessage := fmt.Sprintf("%s: running hook %q", u.unit.Name(), actionName)
	if err = u.acquireHookLock(lockMessage); err != nil {
		return err
	}
	defer u.hookLock.Unlock()

	hctx, err := u.contextFactory.NewActionContext(tag, actionName, actionParams)
	if err != nil {
		return err
	}

	if actionParamsErr != nil {
		// If errors come back here, we have a problem; this should
		// never happen, since errors will only occur if the context
		// had a nil actionData, and actionData != nil runs this
		// method.
		err = hctx.SetActionMessage(actionParamsErr.Error())
		if err != nil {
			return err
		}
		err = hctx.SetActionFailed()
		if err != nil {
			return err
		}
	}

	// err will be any unhandled error from finalizeContext.
	err = context.NewRunner(hctx, u.paths).RunAction(actionName)
	if err != nil {
		err = errors.Annotatef(err, "action %q had unexpected failure", actionName)
		logger.Errorf("action failed: %s", err.Error())
		u.notifyHookFailed(actionName, hctx)
		return err
	}
	if err := u.writeOperationState(operation.RunHook, operation.Done, &hi, nil); err != nil {
		return err
	}
	message, err := hctx.ActionMessage()
	if err != nil {
		return err
	}
	logger.Infof(message)
	u.notifyHookCompleted(actionName, hctx)
	return u.commitHook(hi)
}

// runHook executes the supplied hook.Info in an appropriate hook context. If
// the hook itself fails to execute, it returns errHookFailed.
func (u *Uniter) runHook(hi hook.Info) (err error) {
	if hi.Kind == hooks.Action {
		return u.runAction(hi)
	}

	if err = hi.Validate(); err != nil {
		return err
	}

	// If it wasn't an Action, continue as normal.
	relationId := -1
	hookName := string(hi.Kind)

	if hi.Kind.IsRelation() {
		relationId = hi.RelationId
		if hookName, err = u.relationers[relationId].PrepareHook(hi); err != nil {
			return err
		}
	}
	lockMessage := fmt.Sprintf("%s: running hook %q", u.unit.Name(), hookName)
	if err = u.acquireHookLock(lockMessage); err != nil {
		return err
	}
	defer u.hookLock.Unlock()

	hctx, err := u.contextFactory.NewHookContext(hi)
	if err != nil {
		return err
	}

	// Run the hook.
	if err := u.writeOperationState(operation.RunHook, operation.Pending, &hi, nil); err != nil {
		return err
	}
	logger.Infof("running %q hook", hookName)

	ranHook := true
	err = context.NewRunner(hctx, u.paths).RunHook(hookName)
	if context.IsMissingHookError(err) {
		ranHook = false
	} else if err != nil {
		logger.Errorf("hook %q failed: %s", hookName, err)
		u.notifyHookFailed(hookName, hctx)
		return errHookFailed
	}
	if err := u.writeOperationState(operation.RunHook, operation.Done, &hi, nil); err != nil {
		return err
	}
	if ranHook {
		logger.Infof("ran %q hook", hookName)
		u.notifyHookCompleted(hookName, hctx)
	} else {
		logger.Infof("skipped %q hook (missing)", hookName)
	}
	return u.commitHook(hi)
}

// commitHook ensures that state is consistent with the supplied hook, and
// that the fact of the hook's completion is persisted.
func (u *Uniter) commitHook(hi hook.Info) error {
	logger.Infof("committing %q hook", hi.Kind)
	if hi.Kind.IsRelation() {
		if err := u.relationers[hi.RelationId].CommitHook(hi); err != nil {
			return err
		}
		if hi.Kind == hooks.RelationBroken {
			delete(u.relationers, hi.RelationId)
		}
	}
	if hi.Kind == hooks.ConfigChanged {
		u.ranConfigChanged = true
	}
	if err := u.writeOperationState(operation.Continue, operation.Pending, &hi, nil); err != nil {
		return err
	}
	logger.Infof("committed %q hook", hi.Kind)
	return nil
}

// currentHookName returns the current full hook name.
func (u *Uniter) currentHookName() string {
	hookInfo := u.operationState.Hook
	hookName := string(hookInfo.Kind)
	if hookInfo.Kind.IsRelation() {
		relationer := u.relationers[hookInfo.RelationId]
		name := relationer.ru.Endpoint().Name
		hookName = fmt.Sprintf("%s-%s", name, hookInfo.Kind)
	} else if hookInfo.Kind == hooks.Action {
		hookName = fmt.Sprintf("%s-%s", hookName, hookInfo.ActionId)
	}
	return hookName
}

// getJoinedRelations finds out what relations the unit is *really* part of,
// working around the fact that pre-1.19 (1.18.1?) unit agents don't write a
// state dir for a relation until a remote unit joins.
func (u *Uniter) getJoinedRelations() (map[int]*uniter.Relation, error) {
	var joinedRelationTags []names.RelationTag
	for {
		var err error
		joinedRelationTags, err = u.unit.JoinedRelations()
		if err == nil {
			break
		}
		if params.IsCodeNotImplemented(err) {
			logger.Infof("waiting for state server to be upgraded")
			select {
			case <-u.tomb.Dying():
				return nil, tomb.ErrDying
			case <-time.After(15 * time.Second):
				continue
			}
		}
		return nil, err
	}
	joinedRelations := make(map[int]*uniter.Relation)
	for _, tag := range joinedRelationTags {
		relation, err := u.st.Relation(tag)
		if err != nil {
			return nil, err
		}
		joinedRelations[relation.Id()] = relation
	}
	return joinedRelations, nil
}

// restoreRelations reconciles the local relation state dirs with the
// remote state of the corresponding relations.
func (u *Uniter) restoreRelations() error {
	joinedRelations, err := u.getJoinedRelations()
	if err != nil {
		return err
	}
	knownDirs, err := relation.ReadAllStateDirs(u.paths.State.RelationsDir)
	if err != nil {
		return err
	}
	for id, dir := range knownDirs {
		if rel, ok := joinedRelations[id]; ok {
			if err := u.addRelation(rel, dir); err != nil {
				return err
			}
		} else if err := dir.Remove(); err != nil {
			return err
		}
	}
	for id, rel := range joinedRelations {
		if _, ok := knownDirs[id]; ok {
			continue
		}
		dir, err := relation.ReadStateDir(u.paths.State.RelationsDir, id)
		if err != nil {
			return err
		}
		if err := u.addRelation(rel, dir); err != nil {
			return err
		}
	}
	return nil
}

// updateRelations responds to changes in the life states of the relations
// with the supplied ids. If any id corresponds to an alive relation not
// known to the unit, the uniter will join that relation and return its
// relationer in the added list.
func (u *Uniter) updateRelations(ids []int) (added []*Relationer, err error) {
	for _, id := range ids {
		if r, found := u.relationers[id]; found {
			rel := r.ru.Relation()
			if err := rel.Refresh(); err != nil {
				return nil, fmt.Errorf("cannot update relation %q: %v", rel, err)
			}
			if rel.Life() == params.Dying {
				if err := r.SetDying(); err != nil {
					return nil, err
				} else if r.IsImplicit() {
					delete(u.relationers, id)
				}
			}
			continue
		}
		// Relations that are not alive are simply skipped, because they
		// were not previously known anyway.
		rel, err := u.st.RelationById(id)
		if err != nil {
			if params.IsCodeNotFoundOrCodeUnauthorized(err) {
				continue
			}
			return nil, err
		}
		if rel.Life() != params.Alive {
			continue
		}
		// Make sure we ignore relations not implemented by the unit's charm.
		ch, err := corecharm.ReadCharmDir(u.paths.State.CharmDir)
		if err != nil {
			return nil, err
		}
		if ep, err := rel.Endpoint(); err != nil {
			return nil, err
		} else if !ep.ImplementedBy(ch) {
			logger.Warningf("skipping relation with unknown endpoint %q", ep.Name)
			continue
		}
		dir, err := relation.ReadStateDir(u.paths.State.RelationsDir, id)
		if err != nil {
			return nil, err
		}
		err = u.addRelation(rel, dir)
		if err == nil {
			added = append(added, u.relationers[id])
			continue
		}
		e := dir.Remove()
		if !params.IsCodeCannotEnterScope(err) {
			return nil, err
		}
		if e != nil {
			return nil, e
		}
	}
	if ok, err := u.unit.IsPrincipal(); err != nil {
		return nil, err
	} else if ok {
		return added, nil
	}
	// If no Alive relations remain between a subordinate unit's service
	// and its principal's service, the subordinate must become Dying.
	keepAlive := false
	for _, r := range u.relationers {
		scope := r.ru.Endpoint().Scope
		if scope == corecharm.ScopeContainer && !r.dying {
			keepAlive = true
			break
		}
	}
	if !keepAlive {
		if err := u.unit.Destroy(); err != nil {
			return nil, err
		}
	}
	return added, nil
}

// addRelation causes the unit agent to join the supplied relation, and to
// store persistent state in the supplied dir.
func (u *Uniter) addRelation(rel *uniter.Relation, dir *relation.StateDir) error {
	logger.Infof("joining relation %q", rel)
	ru, err := rel.Unit(u.unit)
	if err != nil {
		return err
	}
	r := NewRelationer(ru, dir, u.relationHooks)
	w, err := u.unit.Watch()
	if err != nil {
		return err
	}
	defer watcher.Stop(w, &u.tomb)
	for {
		select {
		case <-u.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-w.Changes():
			if !ok {
				return watcher.EnsureErr(w)
			}
			err := r.Join()
			if params.IsCodeCannotEnterScopeYet(err) {
				logger.Infof("cannot enter scope for relation %q; waiting for subordinate to be removed", rel)
				continue
			} else if err != nil {
				return err
			}
			logger.Infof("joined relation %q", rel)
			u.relationers[rel.Id()] = r
			return nil
		}
	}
}

// fixDeployer replaces the uniter's git-based charm deployer with a manifest-
// based one, if necessary. It should not be called unless the existing charm
// deployment is known to be in a stable state.
func (u *Uniter) fixDeployer() error {
	if err := charm.FixDeployer(&u.deployer); err != nil {
		return fmt.Errorf("cannot convert git deployment to manifest deployment: %v", err)
	}
	return nil
}

// watchForProxyChanges kicks off a go routine to listen to the watcher and
// update the proxy settings.
func (u *Uniter) watchForProxyChanges(environWatcher apiwatcher.NotifyWatcher) {
	// TODO(fwereade) 23-10-2014 bug 1384565
	// Uniter shouldn't be responsible for this at all: we should rename
	// MachineEnvironmentWorker and run one of those (that eschews rewriting
	// system files).
	go func() {
		for {
			select {
			case <-u.tomb.Dying():
				return
			case _, ok := <-environWatcher.Changes():
				logger.Debugf("new environment change")
				if !ok {
					return
				}
				environConfig, err := u.st.EnvironConfig()
				if err != nil {
					logger.Errorf("cannot load environment configuration: %v", err)
				} else {
					proxySettings := environConfig.ProxySettings()
					logger.Debugf("Updating proxy settings: %#v", proxySettings)
					proxySettings.SetEnvironmentValues()
				}
			}
		}
	}()
}
