// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/symlink"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	jujunames "github.com/juju/juju/juju/names"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/caasoperator/remotestate"
	"github.com/juju/juju/worker/uniter"
	jujucharm "github.com/juju/juju/worker/uniter/charm"
)

var logger = loggo.GetLogger("juju.worker.caasoperator")

// caasOperator implements the capabilities of the caasoperator agent. It is not intended to
// implement the actual *behaviour* of the caasoperator agent; that responsibility is
// delegated to Mode values, which are expected to react to events and direct
// the caasoperator's responses to them.
type caasOperator struct {
	catacomb  catacomb.Catacomb
	config    Config
	paths     Paths
	runner    *worker.Runner
	deployer  jujucharm.Deployer
	stateFile *StateFile
}

// Config hold the configuration for a caasoperator worker.
type Config struct {
	// ModelUUID is the UUID of the model.
	ModelUUID string

	// ModelName is the name of the model.
	ModelName string

	// Application holds the name of the application that
	// this CAAS operator manages.
	Application string

	// CharmGetter is an interface used for getting the
	// application's charm URL and SHA256 hash.
	CharmGetter CharmGetter

	// Clock holds the clock to be used by the CAAS operator
	// for time-related operations.
	Clock clock.Clock

	// PodSpecSetter provides an interface for
	// setting the pod spec for the application.
	PodSpecSetter PodSpecSetter

	// DataDir holds the path to the Juju "data directory",
	// i.e. "/var/lib/juju" (by default). The CAAS operator
	// expects to find the jujud binary at <data-dir>/tools/jujud.
	DataDir string

	// Downloader is an interface used for downloading the
	// application charm.
	Downloader Downloader

	// StatusSetter is an interface used for setting the
	// application status.
	StatusSetter StatusSetter

	// UnitGetter is an interface for getting a unit.
	UnitGetter UnitGetter

	// UnitRemover is an interface for removing a unit.
	UnitRemover UnitRemover

	// ApplicationWatcher is an interface for getting info about an application's charm.
	ApplicationWatcher ApplicationWatcher

	// VersionSetter is an interface for setting the operator agent version.
	VersionSetter VersionSetter

	// LeadershipTrackerFunc is a function for getting a leadership tracker.
	LeadershipTrackerFunc func(unitTag names.UnitTag) leadership.Tracker

	// UniterFacadeFunc is a function for making a uniter facade.
	UniterFacadeFunc func(unitTag names.UnitTag) *apiuniter.State

	// UniterParams are parameters used to construct a uniter worker.
	UniterParams *uniter.UniterParams

	// StartUniterFunc starts a uniter worker using the given runner.
	StartUniterFunc func(runner *worker.Runner, params *uniter.UniterParams) error
}

func (config Config) Validate() error {
	if !names.IsValidApplication(config.Application) {
		return errors.NotValidf("application name %q", config.Application)
	}
	if config.CharmGetter == nil {
		return errors.NotValidf("missing CharmGetter")
	}
	if config.ApplicationWatcher == nil {
		return errors.NotValidf("missing ApplicationWatcher")
	}
	if config.UnitGetter == nil {
		return errors.NotValidf("missing UnitGetter")
	}
	if config.UnitRemover == nil {
		return errors.NotValidf("missing UnitRemover")
	}
	if config.LeadershipTrackerFunc == nil {
		return errors.NotValidf("missing LeadershipTrackerFunc")
	}
	if config.UniterFacadeFunc == nil {
		return errors.NotValidf("missing UniterFacadeFunc")
	}
	if config.UniterParams == nil {
		return errors.NotValidf("missing UniterParams")
	}
	if config.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if config.PodSpecSetter == nil {
		return errors.NotValidf("missing PodSpecSetter")
	}
	if config.DataDir == "" {
		return errors.NotValidf("missing DataDir")
	}
	if config.Downloader == nil {
		return errors.NotValidf("missing Downloader")
	}
	if config.StatusSetter == nil {
		return errors.NotValidf("missing StatusSetter")
	}
	if config.VersionSetter == nil {
		return errors.NotValidf("missing VersionSetter")
	}
	return nil
}

// NewWorker creates a new worker which will install and operate a
// CaaS-based application, by executing hooks and operations in
// response to application state changes.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	paths := NewPaths(config.DataDir, names.NewApplicationTag(config.Application))
	deployer, err := jujucharm.NewDeployer(
		paths.State.CharmDir,
		paths.State.DeployerDir,
		jujucharm.NewBundlesDir(paths.State.BundlesDir, config.Downloader),
	)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create deployer")
	}

	op := &caasOperator{
		config:   config,
		paths:    paths,
		deployer: deployer,
		runner: worker.NewRunner(worker.RunnerParams{
			Clock: config.Clock,

			// One of the uniter workers failing should not
			// prevent the others from running.
			IsFatal: func(error) bool { return false },

			// For any failures, try again in 3 seconds.
			RestartDelay: 3 * time.Second,
		}),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &op.catacomb,
		Work: op.loop,
		Init: []worker.Worker{op.runner},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return op, nil
}

func (op *caasOperator) makeAgentSymlinks(unitTag names.UnitTag) error {
	// All units share the same charm and agent binary.
	// (but with different state dirs for each unit).
	// Set up the required symlinks.

	// First the agent binary.
	agentBinaryDir := op.paths.GetToolsDir()
	unitToolsDir := filepath.Join(agentBinaryDir, unitTag.String())
	err := os.Mkdir(unitToolsDir, 0600)
	if err != nil && !os.IsExist(err) {
		return errors.Trace(err)
	}
	jujudPath := filepath.Join(agentBinaryDir, jujunames.Jujud)
	err = symlink.New(jujudPath, filepath.Join(unitToolsDir, jujunames.Jujud))
	// Ignore permission denied as this won't happen in production
	// but may happen in testing depending on setup of /tmp
	if err != nil && !os.IsExist(err) && !os.IsPermission(err) {
		return errors.Trace(err)
	}

	// TODO(caas) - remove this when upstream charmhelpers are fixed
	// Charmhelpers expect to see a jujud in a machine-X directory.
	legacyMachineDir := filepath.Join(agentBinaryDir, "machine-0")
	err = os.Mkdir(legacyMachineDir, 0600)
	if err != nil && !os.IsExist(err) {
		return errors.Trace(err)
	}
	err = symlink.New(jujudPath, filepath.Join(legacyMachineDir, jujunames.Jujud))
	if err != nil && !os.IsExist(err) && !os.IsPermission(err) {
		return errors.Trace(err)
	}

	// Second the charm directory.
	unitAgentDir := filepath.Join(op.config.DataDir, "agents", unitTag.String())
	err = os.MkdirAll(unitAgentDir, 0600)
	if err != nil && !os.IsExist(err) {
		return errors.Trace(err)
	}
	agentCharmDir := op.paths.GetCharmDir()
	err = symlink.New(agentCharmDir, filepath.Join(unitAgentDir, "charm"))
	// Ignore permission denied as this won't happen in production
	// but may happen in testing depending on setup of /tmp
	if err != nil && !os.IsExist(err) && !os.IsPermission(err) {
		return errors.Trace(err)
	}
	return nil
}

func (op *caasOperator) removeUnitDir(unitTag names.UnitTag) error {
	unitAgentDir := filepath.Join(op.config.DataDir, "agents", unitTag.String())
	return os.RemoveAll(unitAgentDir)
}

func toBinaryVersion(vers version.Number) version.Binary {
	outVers := version.Binary{
		Number: vers,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	return outVers
}

func (op *caasOperator) init() (*LocalState, error) {
	if err := jujucharm.ClearDownloads(op.paths.State.BundlesDir); err != nil {
		logger.Warningf(err.Error())
	}

	op.stateFile = NewStateFile(op.paths.State.OperationsFile)
	localState, err := op.stateFile.Read()
	if err == ErrNoStateFile {
		localState = &LocalState{}
	}

	if err := op.ensureCharm(localState); err != nil {
		if err == jworker.ErrTerminateAgent {
			return nil, err
		}
		return nil, errors.Annotatef(err,
			"failed to initialize caasoperator for %q",
			op.config.Application,
		)
	}
	return localState, nil
}

func (op *caasOperator) loop() (err error) {
	localState, err := op.init()
	if err != nil {
		return err
	}
	logger.Infof("operator %q started", op.config.Application)

	// Start by reporting current tools (which includes arch/series).
	if err := op.config.VersionSetter.SetVersion(
		op.config.Application, toBinaryVersion(jujuversion.Current)); err != nil {
		return errors.Annotate(err, "cannot set agent version")
	}

	var (
		watcher   remotestate.Watcher
		watcherMu sync.Mutex
	)

	restartWatcher := func() error {
		watcherMu.Lock()
		defer watcherMu.Unlock()

		if watcher != nil {
			// watcher added to catacomb, will kill operator if there's an error.
			worker.Stop(watcher)
		}
		var err error
		watcher, err = remotestate.NewWatcher(
			remotestate.WatcherConfig{
				CharmGetter:        op.config.CharmGetter,
				Application:        op.config.Application,
				ApplicationWatcher: op.config.ApplicationWatcher,
			})
		if err != nil {
			return errors.Trace(err)
		}
		if err := op.catacomb.Add(watcher); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	jujuUnitsWatcher, err := op.config.UnitGetter.WatchUnits(op.config.Application)
	if err != nil {
		return errors.Trace(err)
	}
	op.catacomb.Add(jujuUnitsWatcher)

	if err := op.setStatus(status.Active, ""); err != nil {
		return errors.Trace(err)
	}

	// Keep a record of the alive units an a channel used to notify
	// their uniter workers when the charm version has changed.
	aliveUnits := make(map[string]chan struct{})

	if err = restartWatcher(); err != nil {
		err = errors.Annotate(err, "(re)starting watcher")
		return errors.Trace(err)
	}

	// We should not do anything until there has been a change
	// to the remote state. The watcher will trigger at least
	// once initially.
	select {
	case <-op.catacomb.Dying():
		return op.catacomb.ErrDying()
	case <-watcher.RemoteStateChanged():
	}

	for {
		select {
		case <-op.catacomb.Dying():
			return op.catacomb.ErrDying()
		case <-watcher.RemoteStateChanged():
			snap := watcher.Snapshot()
			if charmModified(localState, snap) {
				// Charm changed so download and install the new version.
				err := op.ensureCharm(localState)
				if err != nil {
					return errors.Annotatef(err, "error downloading updated charm %v", localState.CharmURL)
				}
				// Notify all uniters of the change so they run the upgrade-charm hook.
				for unitId, changedChan := range aliveUnits {
					logger.Debugf("trigger upgrade charm for caas unit %v", unitId)
					select {
					case <-op.catacomb.Dying():
						return op.catacomb.ErrDying()
					case changedChan <- struct{}{}:
					}
				}
			}
		case units, ok := <-jujuUnitsWatcher.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			for _, unitId := range units {
				unitLife, err := op.config.UnitGetter.Life(unitId)
				if err != nil && !errors.IsNotFound(err) {
					return errors.Trace(err)
				}
				unitTag := names.NewUnitTag(unitId)
				if errors.IsNotFound(err) || unitLife == life.Dead {
					delete(aliveUnits, unitId)
					if err := op.runner.StopWorker(unitId); err != nil {
						return err
					}
					// Remove the unit's directory
					if err := op.removeUnitDir(unitTag); err != nil {
						return err
					}
					// Remove the unit from state.
					if err := op.config.UnitRemover.RemoveUnit(unitId); err != nil {
						return err
					}
				} else {
					aliveUnits[unitId] = make(chan struct{})
				}
				// Start a worker to manage any new units.
				if _, err := op.runner.Worker(unitId, op.catacomb.Dying()); err == nil || unitLife == life.Dead {
					// Already watching the unit. or we're
					// not yet watching it and it's dead.
					continue
				}

				// Make all the required symlinks.
				if err := op.makeAgentSymlinks(unitTag); err != nil {
					return errors.Trace(err)
				}

				params := op.config.UniterParams
				params.UnitTag = unitTag
				params.UniterFacade = op.config.UniterFacadeFunc(unitTag)
				params.LeadershipTracker = op.config.LeadershipTrackerFunc(unitTag)
				params.ApplicationChannel = aliveUnits[unitId]
				if err := op.config.StartUniterFunc(op.runner, params); err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func charmModified(local *LocalState, remote remotestate.Snapshot) bool {
	// CAAS models may not yet have read the charm url from state.
	if remote.CharmURL == nil {
		return false
	}
	if local == nil || local.CharmURL == nil {
		logger.Warningf("unexpected nil local charm URL")
		return true
	}
	if *local.CharmURL != *remote.CharmURL {
		logger.Debugf("upgrade from %v to %v", local.CharmURL, remote.CharmURL)
		return true
	}

	if local.CharmModifiedVersion != remote.CharmModifiedVersion {
		logger.Debugf("upgrade from CharmModifiedVersion %v to %v", local.CharmModifiedVersion, remote.CharmModifiedVersion)
		return true
	}
	if remote.ForceCharmUpgrade {
		logger.Debugf("force charm upgrade to %v", remote.CharmURL)
		return true
	}
	return false
}

func (op *caasOperator) setStatus(status status.Status, message string, args ...interface{}) error {
	err := op.config.StatusSetter.SetStatus(
		op.config.Application,
		status,
		fmt.Sprintf(message, args...),
		nil,
	)
	return errors.Annotate(err, "setting status")
}

// Kill is part of the worker.Worker interface.
func (op *caasOperator) Kill() {
	op.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (op *caasOperator) Wait() error {
	return op.catacomb.Wait()
}
