// Copyright 2012, 2013 Canonical Ltd.

// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	stderrors "errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	corecharm "github.com/juju/charm"
	"github.com/juju/charm/hooks"
	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/fslock"
	proxyutils "github.com/juju/utils/proxy"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/uniter"
	apiwatcher "github.com/juju/juju/state/api/watcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/jujuc"
	"github.com/juju/juju/worker/uniter/relation"
)

var logger = loggo.GetLogger("juju.worker.uniter")

const (
	// These work fine for linux, but should we need to work with windows
	// workloads in the future, we'll need to move these into a file that is
	// compiled conditionally for different targets and use tcp (most likely).
	RunListenerFile = "run.socket"
)

// A UniterExecutionObserver gets the appropriate methods called when a hook
// is executed and either succeeds or fails.  Missing hooks don't get reported
// in this way.
type UniterExecutionObserver interface {
	HookCompleted(hookName string)
	HookFailed(hookName string)
}

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
	uuid          string
	envName       string

	dataDir      string
	baseDir      string
	toolsDir     string
	relationsDir string
	charmPath    string
	deployer     charm.Deployer
	s            *State
	sf           *StateFile
	rand         *rand.Rand
	hookLock     *fslock.Lock
	runListener  *RunListener

	proxy      proxyutils.Settings
	proxyMutex sync.Mutex

	ranConfigChanged bool
	// The execution observer is only used in tests at this stage. Should this
	// need to be extended, perhaps a list of observers would be needed.
	observer UniterExecutionObserver
}

// NewUniter creates a new Uniter which will install, run, and upgrade
// a charm on behalf of the unit with the given unitTag, by executing
// hooks and operations provoked by changes in st.
func NewUniter(st *uniter.State, unitTag string, dataDir string, hookLock *fslock.Lock) *Uniter {
	u := &Uniter{
		st:       st,
		dataDir:  dataDir,
		hookLock: hookLock,
	}
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.loop(unitTag))
	}()
	return u
}

func (u *Uniter) loop(unitTag string) (err error) {
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

func (u *Uniter) init(unitTag string) (err error) {
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil {
		return err
	}
	u.unit, err = u.st.Unit(tag)
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
	u.toolsDir = tools.ToolsDir(u.dataDir, unitTag)
	if err := EnsureJujucSymlinks(u.toolsDir); err != nil {
		return err
	}
	u.baseDir = filepath.Join(u.dataDir, "agents", unitTag)
	u.relationsDir = filepath.Join(u.baseDir, "state", "relations")
	if err := os.MkdirAll(u.relationsDir, 0755); err != nil {
		return err
	}
	serviceTag, err := names.ParseServiceTag(u.unit.ServiceTag())
	if err != nil {
		return err
	}
	u.service, err = u.st.Service(serviceTag)
	if err != nil {
		return err
	}
	var env *uniter.Environment
	env, err = u.st.Environment()
	if err != nil {
		return err
	}
	u.uuid = env.UUID()
	u.envName = env.Name()

	u.relationers = map[int]*Relationer{}
	u.relationHooks = make(chan hook.Info)
	u.charmPath = filepath.Join(u.baseDir, "charm")
	deployerPath := filepath.Join(u.baseDir, "state", "deployer")
	bundles := charm.NewBundlesDir(filepath.Join(u.baseDir, "state", "bundles"))
	u.deployer, err = charm.NewDeployer(u.charmPath, deployerPath, bundles)
	if err != nil {
		return fmt.Errorf("cannot create deployer: %v", err)
	}
	u.sf = NewStateFile(filepath.Join(u.baseDir, "state", "uniter"))
	u.rand = rand.New(rand.NewSource(time.Now().Unix()))

	// If we start trying to listen for juju-run commands before we have valid
	// relation state, surprising things will come to pass.
	if err := u.restoreRelations(); err != nil {
		return err
	}
	runListenerSocketPath := filepath.Join(u.baseDir, RunListenerFile)
	logger.Debugf("starting juju-run listener on unix:%s", runListenerSocketPath)
	u.runListener, err = NewRunListener(u, runListenerSocketPath)
	if err != nil {
		return err
	}
	// The socket needs to have permissions 777 in order for other users to use it.
	return os.Chmod(runListenerSocketPath, 0777)
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

// writeState saves uniter state with the supplied values, and infers the appropriate
// value of Started.
func (u *Uniter) writeState(op Op, step OpStep, hi *hook.Info, url *corecharm.URL) error {
	s := State{
		Started:  op == RunHook && hi.Kind == hooks.Start || u.s != nil && u.s.Started,
		Op:       op,
		OpStep:   step,
		Hook:     hi,
		CharmURL: url,
	}
	if err := u.sf.Write(s.Started, s.Op, s.OpStep, s.Hook, s.CharmURL); err != nil {
		return err
	}
	u.s = &s
	return nil
}

// deploy deploys the supplied charm URL, and sets follow-up hook operation state
// as indicated by reason.
func (u *Uniter) deploy(curl *corecharm.URL, reason Op) error {
	if reason != Install && reason != Upgrade {
		panic(fmt.Errorf("%q is not a deploy operation", reason))
	}
	var hi *hook.Info
	if u.s != nil && (u.s.Op == RunHook || u.s.Op == Upgrade) {
		// If this upgrade interrupts a RunHook, we need to preserve the hook
		// info so that we can return to the appropriate error state. However,
		// if we're resuming (or have force-interrupted) an Upgrade, we also
		// need to preserve whatever hook info was preserved when we initially
		// started upgrading, to ensure we still return to the correct state.
		hi = u.s.Hook
	}
	if u.s == nil || u.s.OpStep != Done {
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
		if err = u.writeState(reason, Pending, hi, curl); err != nil {
			return err
		}
		if err = u.deployer.Deploy(); err != nil {
			return err
		}
		if err = u.writeState(reason, Done, hi, curl); err != nil {
			return err
		}
	}
	logger.Infof("charm %q is deployed", curl)
	status := Queued
	if hi != nil {
		// If a hook operation was interrupted, restore it.
		status = Pending
	} else {
		// Otherwise, queue the relevant post-deploy hook.
		hi = &hook.Info{}
		switch reason {
		case Install:
			hi.Kind = hooks.Install
		case Upgrade:
			hi.Kind = hooks.UpgradeCharm
		}
	}
	return u.writeState(RunHook, status, hi, nil)
}

// errHookFailed indicates that a hook failed to execute, but that the Uniter's
// operation is not affected by the error.
var errHookFailed = stderrors.New("hook execution failed")

func (u *Uniter) getHookContext(hr *HookRunner, hi *hook.Info) (hctx *HookContext, err error) {
	if err := hi.Validate(); err != nil {
		return nil, err
	}

	hookName := string(hi.Kind)
	actionParams := map[string]interface{}{}

	// Get a proper name if we have a Relation or Action.  If we have an
	// Action, make sure its params are valid.
	if hi.Kind.IsRelation() {
		hookName, err = u.relationers[hi.RelationId].PrepareHook(*hi)
		if err != nil {
			return nil, err
		}
	} else if hi.Kind == hooks.ActionRequested {
		action, err := u.st.Action(names.NewActionTag(hi.ActionId))
		if err != nil {
			return nil, err
		}
		actionParams = action.Params()
		hookName = action.Name()

		// TODO(binary132): Validate
	}

	hctxId := fmt.Sprintf("%s:%s:%d", u.unit.Name(), hookName, u.rand.Int63())

	return NewHookContext(hr, hctxId, hi, hookName, actionParams)
}

func (u *Uniter) getHookRunner() (runner *HookRunner, err error) {
	apiAddrs, err := u.st.APIAddresses()
	if err != nil {
		return nil, err
	}
	ownerTag, err := u.service.GetOwnerTag()
	if err != nil {
		return nil, err
	}
	ctxRelations := map[int]*ContextRelation{}
	for id, r := range u.relationers {
		ctxRelations[id] = r.Context()
	}

	u.proxyMutex.Lock()
	defer u.proxyMutex.Unlock()

	// Make a copy of the proxy settings.
	proxySettings := u.proxy
	return NewHookRunner(u.unit, u.uuid, u.envName, ctxRelations,
		apiAddrs, ownerTag, proxySettings, u.charmPath, u.toolsDir)
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

func (u *Uniter) startJujucServer(context *HookContext) (*jujuc.Server, string, error) {
	// Prepare server.
	getCmd := func(ctxId, cmdName string) (cmd.Command, error) {
		// TODO: switch to long-running server with single context;
		// use nonce in place of context id.
		if ctxId != context.id {
			return nil, fmt.Errorf("expected context id %q, got %q", context.id, ctxId)
		}
		return jujuc.NewCommand(context, cmdName)
	}
	socketPath := filepath.Join(u.baseDir, "agent.socket")
	// Use abstract namespace so we don't get stale socket files.
	socketPath = "@" + socketPath
	srv, err := jujuc.NewServer(getCmd, socketPath)
	if err != nil {
		return nil, "", err
	}
	go srv.Run()
	return srv, socketPath, nil
}

// RunCommands executes the supplied commands in a hook context.
func (u *Uniter) RunCommands(commands string) (results *exec.ExecResponse, err error) {
	logger.Tracef("run commands: %s", commands)
	lockMessage := fmt.Sprintf("%s: running commands", u.unit.Name())
	if err = u.acquireHookLock(lockMessage); err != nil {
		return nil, err
	}
	defer u.hookLock.Unlock()

	runner, err := u.getHookRunner()
	if err != nil {
		return nil, err
	}
	hctx, err := u.getHookContext(runner, &hook.Info{
		Kind:       hooks.RunCommands,
		RelationId: -1,
	})
	if err != nil {
		return nil, err
	}
	srv, socketPath, err := u.startJujucServer(hctx)
	if err != nil {
		return nil, err
	}
	defer srv.Close()

	result, err := hctx.RunCommands(commands, socketPath)
	if result != nil {
		logger.Tracef("run commands: rc=%v\nstdout:\n%sstderr:\n%s", result.Code, result.Stdout, result.Stderr)
	}
	return result, err
}

func (u *Uniter) notifyHookInternal(hook string, hctx *HookContext, method func(string)) {
	if r, ok := hctx.HookRelation(); ok {
		remote, _ := hctx.RemoteUnitName()
		if remote != "" {
			remote = " " + remote
		}
		hook = hook + remote + " " + r.FakeId()
	}
	method(hook)
}

func (u *Uniter) notifyHookCompleted(hook string, hctx *HookContext) {
	if u.observer != nil {
		u.notifyHookInternal(hook, hctx, u.observer.HookCompleted)
	}
}

func (u *Uniter) notifyHookFailed(hook string, hctx *HookContext) {
	if u.observer != nil {
		u.notifyHookInternal(hook, hctx, u.observer.HookFailed)
	}
}

// runHook executes the supplied hook.Info in an appropriate hook context. If
// the hook itself fails to execute, it returns errHookFailed.
func (u *Uniter) runHook(hi hook.Info) (err error) {
	hookRunner, err := u.getHookRunner()
	if err != nil {
		return err
	}

	// Prepare context.
	hookContext, err := u.getHookContext(hookRunner, &hi)
	if err != nil {
		return err
	}

	lockMessage := fmt.Sprintf("%s: running hook %q", u.unit.Name(), hookContext.hookName)

	if err = u.acquireHookLock(lockMessage); err != nil {
		return err
	}
	defer u.hookLock.Unlock()

	// WIP

	srv, socketPath, err := u.startJujucServer(hookContext)
	if err != nil {
		return err
	}
	defer srv.Close()

	// Run the hook.
	if err := u.writeState(RunHook, Pending, &hi, nil); err != nil {
		return err
	}
	logger.Infof("running %q hook", hookContext.hookName)

	ranHook := true
	err = hookContext.RunHook(socketPath)

	if IsMissingHookError(err) {
		ranHook = false
	} else if err != nil {
		logger.Errorf("hook failed: %s", err)
		u.notifyHookFailed(hookContext.hookName, hookContext)
		return errHookFailed
	}
	if err := u.writeState(RunHook, Done, &hi, nil); err != nil {
		return err
	}
	if ranHook {
		logger.Infof("ran %q hook", hookContext.hookName)
		u.notifyHookCompleted(hookContext.hookName, hookContext)
	} else {
		logger.Infof("skipped %q hook (missing)", hookContext.hookName)
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
	} else if hi.Kind == hooks.ActionRequested {
		// TODO(binary132): complete Action
	} else if hi.Kind == hooks.ConfigChanged {
		u.ranConfigChanged = true
	}
	if err := u.writeState(Continue, Pending, &hi, nil); err != nil {
		return err
	}
	logger.Infof("committed %q hook", hi.Kind)
	return nil
}

// currentHookName returns the current full hook name.
func (u *Uniter) currentHookName() string {
	hookInfo := u.s.Hook
	hookName := string(hookInfo.Kind)
	if hookInfo.Kind.IsRelation() {
		relationer := u.relationers[hookInfo.RelationId]
		name := relationer.ru.Endpoint().Name
		hookName = fmt.Sprintf("%s-%s", name, hookInfo.Kind)
	} else if hookInfo.Kind == hooks.ActionRequested {
		hookName = fmt.Sprintf("%s-%s", hookName, hookInfo.ActionId)
	}

	// TODO: How do we handle Action hooks?  Getting the Name requires
	// API call.  Elsewhere, the Name of the Hook is considered to be
	// the name of the Action in order to find and run the executable.

	return hookName
}

// getJoinedRelations finds out what relations the unit is *really* part of,
// working around the fact that pre-1.19 (1.18.1?) unit agents don't write a
// state dir for a relation until a remote unit joins.
func (u *Uniter) getJoinedRelations() (map[int]*uniter.Relation, error) {
	var joinedRelationTags []string
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
	knownDirs, err := relation.ReadAllStateDirs(u.relationsDir)
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
		dir, err := relation.ReadStateDir(u.relationsDir, id)
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
		ch, err := corecharm.ReadDir(u.charmPath)
		if err != nil {
			return nil, err
		}
		if ep, err := rel.Endpoint(); err != nil {
			return nil, err
		} else if !ep.ImplementedBy(ch) {
			logger.Warningf("skipping relation with unknown endpoint %q", ep.Name)
			continue
		}
		dir, err := relation.ReadStateDir(u.relationsDir, id)
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
				return watcher.MustErr(w)
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

// updatePackageProxy updates the package proxy settings from the
// environment.
func (u *Uniter) updatePackageProxy(cfg *config.Config) {
	u.proxyMutex.Lock()
	defer u.proxyMutex.Unlock()

	newSettings := cfg.ProxySettings()
	if u.proxy != newSettings {
		u.proxy = newSettings
		logger.Debugf("Updated proxy settings: %#v", u.proxy)
		// Update the environment values used by the process.
		u.proxy.SetEnvironmentValues()
	}
}

// watchForProxyChanges kicks off a go routine to listen to the watcher and
// update the proxy settings.
func (u *Uniter) watchForProxyChanges(environWatcher apiwatcher.NotifyWatcher) {
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
					u.updatePackageProxy(environConfig)
				}
			}
		}
	}()
}
