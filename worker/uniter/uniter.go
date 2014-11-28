// Copyright 2012-2014 Canonical Ltd.

// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
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
	"github.com/juju/juju/worker/uniter/context/jujuc"
	"github.com/juju/juju/worker/uniter/filter"
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

// CollectMetricsSignal is the signature of the function used to generate a collect-metrics
// signal.
type CollectMetricsSignal func(now, lastSignal time.Time, interval time.Duration) <-chan time.Time

// collectMetricsTimer returns a channel that will signal the collect metrics hook
// as close to metricsPollInterval after the last run as possible.
func collectMetricsTimer(now, lastRun time.Time, interval time.Duration) <-chan time.Time {
	waitDuration := interval - now.Sub(lastRun)
	logger.Debugf("waiting for %v", waitDuration)
	return time.After(waitDuration)
}

// activeMetricsTimer is the timer function used to generate metrics collections
// signals for metrics-enabled charms.
var activeMetricsTimer CollectMetricsSignal = collectMetricsTimer

// inactiveMetricsTimer is the default metrics signal generation function, that returns no signal.
// It will be used in charms that do not declare metrics.
func inactiveMetricsTimer(_, _ time.Time, _ time.Duration) <-chan time.Time {
	return nil
}

// deployerProxy exists because we're not yet comfortable that we can safely
// drop support for charm.gitDeployer. If we can, then the uniter doesn't
// need a deployer reference at all: and we can drop fixDeployer, and even
// the Notify* methods on the Deployer interface, and simply hand the
// deployer we create over to the operationFactory at creation and forget
// about it.
//
// We will never be *completely* certain that gitDeployer can be dropped,
// because it's not done as an upgrade step (because we can't replace the
// deployer while conflicted, and upgrades are not gated on no-conflicts);
// and so long as there's a reasonable possibility that someone *might* have
// been running a pre-1.19.1 environment, and have either upgraded directly
// in a conflict state *or* have upgraded stepwise without fixing a conflict
// state, we should keep this complexity.
//
// In practice, that possibility is growing ever more remote, but we're not
// ready to pull the trigger yet.
type deployerProxy struct {
	charm.Deployer
}

// Uniter implements the capabilities of the unit agent. It is not intended to
// implement the actual *behaviour* of the unit agent; that responsibility is
// delegated to Mode values, which are expected to react to events and direct
// the uniter's responses to them.
type Uniter struct {
	tomb          tomb.Tomb
	st            *uniter.State
	paths         Paths
	f             filter.Filter
	unit          *uniter.Unit
	service       *uniter.Service
	relationers   map[int]*Relationer
	relationHooks chan hook.Info

	deployer          *deployerProxy
	operationFactory  operation.Factory
	operationExecutor operation.Executor

	hookLock    *fslock.Lock
	runListener *RunListener

	ranConfigChanged bool

	// The execution observer is only used in tests at this stage. Should this
	// need to be extended, perhaps a list of observers would be needed.
	observer UniterExecutionObserver

	// collectMetricsAt defines a function that will be used to generate signals
	// for the collect-metrics hook.
	collectMetricsAt CollectMetricsSignal
}

// NewUniter creates a new Uniter which will install, run, and upgrade
// a charm on behalf of the unit with the given unitTag, by executing
// hooks and operations provoked by changes in st.
func NewUniter(st *uniter.State, unitTag names.UnitTag, dataDir string, hookLock *fslock.Lock) *Uniter {
	u := &Uniter{
		st:               st,
		paths:            NewPaths(dataDir, unitTag),
		hookLock:         hookLock,
		collectMetricsAt: inactiveMetricsTimer,
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
	u.f, err = filter.NewFilter(u.st, unitTag)
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
			switch cause := errors.Cause(err); cause {
			case operation.ErrHookFailed:
				mode, err = ModeHookError, nil
			case operation.ErrNeedsReboot:
				err = worker.ErrRebootMachine
			case tomb.ErrDying, worker.ErrTerminateAgent:
				err = cause
			}
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
	if err := jujuc.EnsureSymlinks(u.paths.ToolsDir); err != nil {
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
	if err := u.restoreRelations(); err != nil {
		return err
	}

	deployer, err := charm.NewDeployer(
		u.paths.State.CharmDir,
		u.paths.State.DeployerDir,
		charm.NewBundlesDir(u.paths.State.BundlesDir),
	)
	if err != nil {
		return fmt.Errorf("cannot create deployer: %v", err)
	}
	u.deployer = &deployerProxy{deployer}
	contextFactory, err := context.NewFactory(
		u.st, unitTag, u.getRelationInfos, u.getCharm,
	)
	if err != nil {
		return err
	}
	u.operationFactory = operation.NewFactory(
		u.deployer,
		contextFactory,
		&operationCallbacks{u},
		u.tomb.Dying(),
	)

	operationExecutor, err := operation.NewExecutor(
		u.paths.State.OperationsFile, u.getServiceCharmURL,
	)
	if err != nil {
		return err
	}
	u.operationExecutor = operationExecutor

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

func (u *Uniter) getRelationInfos() map[int]*context.RelationInfo {
	relationInfos := map[int]*context.RelationInfo{}
	for id, r := range u.relationers {
		relationInfos[id] = r.ContextInfo()
	}
	return relationInfos
}

func (u *Uniter) getCharm() (corecharm.Charm, error) {
	ch, err := corecharm.ReadCharm(u.paths.State.CharmDir)
	if err != nil {
		return nil, err
	}
	return ch, nil
}

func (u *Uniter) getServiceCharmURL() (*corecharm.URL, error) {
	charmURL, _, err := u.service.CharmURL()
	return charmURL, err
}

func (u *Uniter) operationState() operation.State {
	return u.operationExecutor.State()
}

// deploy deploys the supplied charm URL, and sets follow-up hook operation state
// as indicated by reason.
func (u *Uniter) deploy(curl *corecharm.URL, reason operation.Kind) error {
	op, err := u.operationFactory.NewDeploy(curl, reason)
	if err != nil {
		return err
	}
	err = u.operationExecutor.Run(op)
	if err != nil {
		return err
	}

	// The new charm may have declared metrics where the old one had none
	// (or vice versa), so reset the metrics collection policy according
	// to current state.
	// TODO(fwereade): maybe this should be in operation.deploy.Commit()?
	return u.initializeMetricsCollector()
}

// initializeMetricsCollector enables the periodic collect-metrics hook
// for charms that declare metrics.
func (u *Uniter) initializeMetricsCollector() error {
	charm, err := u.getCharm()
	if err != nil {
		return err
	}
	if metrics := charm.Metrics(); metrics != nil && len(metrics.Metrics) > 0 {
		u.collectMetricsAt = activeMetricsTimer
	}
	return nil
}

// RunCommands executes the supplied commands in a hook context.
func (u *Uniter) RunCommands(args RunCommandsArgs) (results *exec.ExecResponse, err error) {
	// TODO(fwereade): this is *still* all sorts of messed-up and not remotely
	// goroutine-safe, but that's not what I'm fixing at the moment. We'll deal
	// with that when we get a sane ops queue and are no longer depending on the
	// uniter mode funcs for all the rest of our scheduling.
	// In particular, we should probably defer InferRemoteUnit until much later;
	// it's currently quite plausible that the relation state could change a fair
	// amount between entering this method and actually acquiring the execution
	// lock (and it could change more after that, too, but at least that window
	// is reasonably bounded in a way that this one is not).
	logger.Tracef("run commands: %s", args.Commands)

	remoteUnitName, err := InferRemoteUnit(u.relationers, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	type responseInfo struct {
		response *exec.ExecResponse
		err      error
	}
	responseChan := make(chan responseInfo, 1)
	sendResponse := func(response *exec.ExecResponse, err error) {
		responseChan <- responseInfo{response, err}
	}

	op, err := u.operationFactory.NewCommands(args.Commands, args.RelationId, remoteUnitName, sendResponse)
	if err != nil {
		return nil, err
	}
	err = u.operationExecutor.Run(op)
	if err == nil {
		select {
		case response := <-responseChan:
			results, err = response.response, response.err
		default:
			err = errors.New("command response never sent")
		}
	}
	if errors.Cause(err) == operation.ErrNeedsReboot {
		u.tomb.Kill(worker.ErrRebootMachine)
		err = nil
	}
	if err != nil {
		u.tomb.Kill(err)
	}
	return results, err
}

// runAction executes the supplied hook.Info as an Action.
func (u *Uniter) runAction(actionId string) (err error) {
	op, err := u.operationFactory.NewAction(actionId)
	if err != nil {
		return err
	}
	return u.operationExecutor.Run(op)
}

// runHook executes the supplied hook.Info in an appropriate hook context. If
// the hook itself fails to execute, it returns errHookFailed.
func (u *Uniter) runHook(hi hook.Info) (err error) {
	if hi.Kind == hooks.Action {
		return u.runAction(hi.ActionId)
	}
	op, err := u.operationFactory.NewHook(hi)
	if err != nil {
		return err
	}
	return u.operationExecutor.Run(op)
}

func (u *Uniter) skipHook(hi hook.Info) (err error) {
	op, err := u.operationFactory.NewHook(hi)
	if err != nil {
		return err
	}
	return u.operationExecutor.Skip(op)
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
	if err := charm.FixDeployer(&u.deployer.Deployer); err != nil {
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

// InferRemoteUnit attempts to infer the remoteUnit for a given relationId. If the
// remoteUnit is present in the RunCommandArgs, that is used and no attempt to infer
// the remoteUnit happens. If no remoteUnit or more than one remoteUnit is found for
// a given relationId an error is returned for display to the user.
func InferRemoteUnit(relationers map[int]*Relationer, args RunCommandsArgs) (string, error) {
	if args.RelationId == -1 {
		if len(args.RemoteUnitName) > 0 {
			return "", errors.Errorf("remote unit: %s, provided without a relation", args.RemoteUnitName)
		}
		return "", nil
	}

	remoteUnit := args.RemoteUnitName
	noRemoteUnit := len(remoteUnit) == 0

	relationer, found := relationers[args.RelationId]
	if !found {
		return "", errors.Errorf("unable to find relation id: %d", args.RelationId)
	}

	remoteUnits := relationer.ContextInfo().MemberNames
	numRemoteUnits := len(remoteUnits)

	if !args.ForceRemoteUnit {
		if noRemoteUnit {
			var err error
			switch numRemoteUnits {
			case 0:
				err = errors.Errorf("no remote unit found for relation id: %d, override to execute commands", args.RelationId)
			case 1:
				remoteUnit = remoteUnits[0]
			default:
				err = errors.Errorf("unable to determine remote-unit, please disambiguate: %+v", remoteUnits)
			}

			if err != nil {
				return "", errors.Trace(err)
			}
		} else {
			found := false
			for _, value := range remoteUnits {
				if value == remoteUnit {
					found = true
					break
				}
			}
			if !found {
				return "", errors.Errorf("no remote unit found: %s, override to execute command", remoteUnit)
			}
		}
	}

	if noRemoteUnit && args.ForceRemoteUnit {
		return remoteUnit, nil
	}

	if !names.IsValidUnit(remoteUnit) {
		return "", errors.Errorf(`"%s" is not a valid remote unit name`, remoteUnit)
	}

	unitTag := names.NewUnitTag(remoteUnit)
	return unitTag.Id(), nil
}
