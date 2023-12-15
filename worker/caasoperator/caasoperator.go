// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jujusymlink "github.com/juju/utils/v3/symlink"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/caas"
	caasconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/sockets"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/caasoperator/remotestate"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/uniter"
	jujucharm "github.com/juju/juju/worker/uniter/charm"
	uniterremotestate "github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/wrench"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one through as config to the worker.
type logger interface{}

var _ logger = struct{}{}

var (
	jujuExec       = paths.JujuExec(paths.CurrentOS())
	jujuDumpLogs   = paths.JujuDumpLogs(paths.CurrentOS())
	jujuIntrospect = paths.JujuIntrospect(paths.CurrentOS())

	jujudSymlinks = []string{
		jujuExec,
		jujuDumpLogs,
		jujuIntrospect,
	}
)

// caasOperator implements the capabilities of the caasoperator agent. It is not intended to
// implement the actual *behaviour* of the caasoperator agent; that responsibility is
// delegated to Mode values, which are expected to react to events and direct
// the caasoperator's responses to them.
type caasOperator struct {
	catacomb       catacomb.Catacomb
	config         Config
	paths          Paths
	runner         *worker.Runner
	deployer       jujucharm.Deployer
	stateFile      *StateFile
	deploymentMode caas.DeploymentMode
}

// Config hold the configuration for a caasoperator worker.
type Config struct {
	Logger Logger

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

	// DataDir holds the path to the Juju "data directory",
	// i.e. "/var/lib/juju" (by default). The CAAS operator
	// expects to find the jujud binary at <data-dir>/tools/jujud.
	DataDir string

	// ProfileDir is where the introspection scripts are written.
	ProfileDir string

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

	// ContainerStartWatcher provides an interface for watching
	// for unit container starts.
	ContainerStartWatcher ContainerStartWatcher

	// VersionSetter is an interface for setting the operator agent version.
	VersionSetter VersionSetter

	// LeadershipTrackerFunc is a function for getting a leadership tracker worker.
	LeadershipTrackerFunc func(unitTag names.UnitTag) leadership.TrackerWorker

	// UniterFacadeFunc is a function for making a uniter facade.
	UniterFacadeFunc func(unitTag names.UnitTag) *apiuniter.State

	// ResourcesFacadeFunc is a function for making a unit resources facade.
	ResourcesFacadeFunc func(unitTag names.UnitTag) (*apiuniter.ResourcesFacadeClient, error)

	// PayloadFacadeFunc is a function for making a unit payload facade.
	PayloadFacadeFunc func() *apiuniter.PayloadFacadeClient

	// UniterParams are parameters used to construct a uniter worker.
	UniterParams *uniter.UniterParams

	// StartUniterFunc starts a uniter worker using the given runner.
	StartUniterFunc func(runner *worker.Runner, params *uniter.UniterParams) error

	// RunListenerSocketFunc returns a socket used for the juju run listener.
	RunListenerSocketFunc func(*uniter.SocketConfig) (*sockets.Socket, error)

	// OperatorInfo contains serving information such as Certs and PrivateKeys.
	OperatorInfo caas.OperatorInfo

	// ExecClientGetter returns an exec client for initializing caas units.
	ExecClientGetter func() (exec.Executor, error)
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
	if config.ContainerStartWatcher == nil {
		return errors.NotValidf("missing ContainerStartWatcher")
	}
	if config.LeadershipTrackerFunc == nil {
		return errors.NotValidf("missing LeadershipTrackerFunc")
	}
	if config.UniterFacadeFunc == nil {
		return errors.NotValidf("missing UniterFacadeFunc")
	}
	if config.ResourcesFacadeFunc == nil {
		return errors.NotValidf("missing ResourcesFacadeFunc")
	}
	if config.PayloadFacadeFunc == nil {
		return errors.NotValidf("missing PayloadFacadeFunc")
	}
	if config.UniterParams == nil {
		return errors.NotValidf("missing UniterParams")
	}
	if config.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if config.DataDir == "" {
		return errors.NotValidf("missing DataDir")
	}
	if config.ProfileDir == "" {
		return errors.NotValidf("missing ProfileDir")
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
	if config.ExecClientGetter == nil {
		return errors.NotValidf("missing ExecClientGetter")
	}

	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

func (config Config) getPaths() Paths {
	return NewPaths(config.DataDir, names.NewApplicationTag(config.Application))
}

// NewWorker creates a new worker which will install and operate a
// CaaS-based application, by executing hooks and operations in
// response to application state changes.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	paths := config.getPaths()
	logger := config.Logger.Child("charm")
	deployer, err := jujucharm.NewDeployer(
		paths.State.CharmDir,
		paths.State.DeployerDir,
		jujucharm.NewBundlesDir(
			paths.State.BundlesDir,
			config.Downloader,
			logger),
		logger,
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
			Logger:       config.Logger.Child("runner"),
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
	// All units share the same agent binary.
	// Set up the required symlinks.

	// First the agent binary.
	agentBinaryDir := op.paths.GetToolsDir()
	unitToolsDir := filepath.Join(agentBinaryDir, unitTag.String())
	err := os.Mkdir(unitToolsDir, 0600)
	if err != nil && !os.IsExist(err) {
		return errors.Trace(err)
	}
	jujudPath := filepath.Join(agentBinaryDir, jujunames.Jujud)
	err = jujusymlink.New(jujudPath, filepath.Join(unitToolsDir, jujunames.Jujud))
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
	err = jujusymlink.New(jujudPath, filepath.Join(legacyMachineDir, jujunames.Jujud))
	if err != nil && !os.IsExist(err) && !os.IsPermission(err) {
		return errors.Trace(err)
	}

	for _, slk := range jujudSymlinks {
		err = jujusymlink.New(jujudPath, slk)
		if err != nil && !os.IsExist(err) && !os.IsPermission(err) {
			return errors.Trace(err)
		}
		// TODO(juju 4) - remove this legacy behaviour.
		// Remove the obsolete "juju-run" symlink
		if strings.Contains(slk, "/juju-exec") {
			runLink := strings.Replace(slk, "/juju-exec", "/juju-run", 1)
			_ = os.Remove(runLink)
		}
	}

	// Ensure legacy charm symlinks created before 2.8 getting unlinked.
	unitCharmDir := filepath.Join(op.config.DataDir, "agents", unitTag.String(), "charm")
	isUnitCharmDirSymlink, err := jujusymlink.IsSymlink(unitCharmDir)
	if os.IsNotExist(errors.Cause(err)) || os.IsPermission(errors.Cause(err)) {
		// Ignore permission denied as this won't happen in production
		// but may happen in testing depending on setup of /tmp.
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	if isUnitCharmDirSymlink {
		op.config.Logger.Warningf("removing legacy charm symlink for %q", unitTag.String())
		if err := os.Remove(unitCharmDir); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (op *caasOperator) removeUnitDir(unitTag names.UnitTag) error {
	unitAgentDir := filepath.Join(op.config.DataDir, "agents", unitTag.String())
	return os.RemoveAll(unitAgentDir)
}

func toBinaryVersion(vers version.Number, osType string) version.Binary {
	outVers := version.Binary{
		Number:  vers,
		Arch:    arch.HostArch(),
		Release: osType,
	}
	return outVers
}

func runListenerSocket(sc *uniter.SocketConfig) (*sockets.Socket, error) {
	socket := sockets.Socket{
		Network:   "tcp",
		Address:   fmt.Sprintf(":%d", caasconstants.JujuExecServerSocketPort),
		TLSConfig: sc.TLSConfig,
	}
	return &socket, nil
}

func (op *caasOperator) init() (*LocalState, error) {
	if err := introspection.WriteProfileFunctions(op.config.ProfileDir); err != nil {
		// This isn't fatal, just annoying.
		op.config.Logger.Errorf("failed to write profile funcs: %v", err)
	}

	if err := jujucharm.ClearDownloads(op.paths.State.BundlesDir); err != nil {
		op.config.Logger.Warningf(err.Error())
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

	// Set up a single remote juju run listener to be used by all workload units.
	if op.deploymentMode != caas.ModeOperator {
		if op.config.RunListenerSocketFunc == nil {
			return nil, errors.New("missing RunListenerSocketFunc")
		}
		if op.config.RunListenerSocketFunc != nil {
			socket, err := op.config.RunListenerSocketFunc(op.config.UniterParams.SocketConfig)
			if err != nil {
				return nil, errors.Annotate(err, "creating juju run socket")
			}
			op.config.Logger.Debugf("starting caas operator juju-exec listener on %v", socket)
			logger := loggo.GetLogger("juju.worker.uniter")
			runListener, err := uniter.NewRunListener(*socket, logger)
			if err != nil {
				return nil, errors.Annotate(err, "creating juju run listener")
			}
			rlw := uniter.NewRunListenerWrapper(runListener, logger)
			if err := op.catacomb.Add(rlw); err != nil {
				return nil, errors.Trace(err)
			}
			op.config.UniterParams.RunListener = runListener
		}
	}
	return localState, nil
}

func (op *caasOperator) loop() (err error) {
	logger := op.config.Logger

	defer func() {
		if err == nil {
			logger.Debugf("operator %q is peacefully shutting down", op.config.Application)
		} else {
			logger.Warningf("operator %q is shutting down, err: %s", op.config.Application, err.Error())
		}
		if errors.IsNotFound(err) {
			err = jworker.ErrTerminateAgent
		}
	}()

	localState, err := op.init()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof("operator %q started", op.config.Application)

	// Start by reporting current tools (which includes arch/ostype).
	hostOSType := coreos.HostOSTypeName()
	if err := op.config.VersionSetter.SetVersion(
		op.config.Application, toBinaryVersion(jujuversion.Current, hostOSType)); err != nil {
		return errors.Annotate(err, "cannot set agent version")
	}

	var remoteWatcher remotestate.Watcher

	restartWatcher := func() error {
		if remoteWatcher != nil {
			// watcher added to catacomb, will kill operator if there's an error.
			_ = worker.Stop(remoteWatcher)
		}
		var err error
		remoteWatcher, err = remotestate.NewWatcher(
			remotestate.WatcherConfig{
				Logger:             loggo.GetLogger("juju.worker.caasoperator.remotestate"),
				CharmGetter:        op.config.CharmGetter,
				Application:        op.config.Application,
				ApplicationWatcher: op.config.ApplicationWatcher,
			})
		if err != nil {
			return errors.Trace(err)
		}
		if err := op.catacomb.Add(remoteWatcher); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	jujuUnitsWatcher, err := op.config.UnitGetter.WatchUnits(op.config.Application)
	if err != nil {
		return errors.Trace(err)
	}
	if err := op.catacomb.Add(jujuUnitsWatcher); err != nil {
		return errors.Trace(err)
	}

	var containerStartChan watcher.StringsChannel
	if op.deploymentMode != caas.ModeOperator {
		// Match the init container and the default container.
		containerRegex := fmt.Sprintf("(?:%s|)", caas.InitContainerName)
		containerStartWatcher, err := op.config.ContainerStartWatcher.WatchContainerStart(
			op.config.Application, containerRegex)
		if err != nil {
			return errors.Trace(err)
		}
		if err := op.catacomb.Add(containerStartWatcher); err != nil {
			return errors.Trace(err)
		}
		containerStartChan = containerStartWatcher.Changes()
	}

	if err := op.setStatus(status.Active, ""); err != nil {
		return errors.Trace(err)
	}

	// Channels used to notify uniter worker that the workload container
	// is running.
	unitRunningChannels := make(map[string]chan struct{})

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
	case <-remoteWatcher.RemoteStateChanged():
	}

	for {
		select {
		case <-op.catacomb.Dying():
			return op.catacomb.ErrDying()
		case <-remoteWatcher.RemoteStateChanged():
			snap := remoteWatcher.Snapshot()
			if op.charmModified(localState, snap) {
				// Charm changed so download and install the new version.
				err := op.ensureCharm(localState)
				if err != nil {
					return errors.Annotatef(err, "error downloading updated charm %v", localState.CharmURL)
				}
				// Reset the application's "Downloading..." message.
				if err := op.setStatus(status.Active, ""); err != nil {
					return errors.Trace(err)
				}
			}
		case units, ok := <-containerStartChan:
			if !ok {
				return errors.New("container start watcher closed channel")
			}
			for _, unitID := range units {
				if runningChan, ok := unitRunningChannels[unitID]; ok {
					logger.Debugf("trigger running status for caas unit %v", unitID)
					select {
					case <-op.catacomb.Dying():
						return op.catacomb.ErrDying()
					case runningChan <- struct{}{}:
					default:
						// This will happen when the buffered channel already
						// has an event. If this is the case it's ok to discard
						// the event.
						logger.Debugf("unit running chan[%q] discarding running event as one already exists", unitID)
					}
				}
			}
		case units, ok := <-jujuUnitsWatcher.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			for _, v := range units {
				unitID := v
				unitLife, err := op.config.UnitGetter.Life(unitID)
				if err != nil && !errors.IsNotFound(err) {
					return errors.Trace(err)
				}
				logger.Debugf("got unit change %q (%s)", unitID, unitLife)
				unitTag := names.NewUnitTag(unitID)
				if errors.IsNotFound(err) || unitLife == life.Dead {
					delete(unitRunningChannels, unitID)
					logger.Debugf("stopping uniter for dead unit %q", unitID)
					if err := op.runner.StopAndRemoveWorker(unitID, op.catacomb.Dying()); err != nil {
						logger.Warningf("stopping uniter for dead unit %q: %v", unitID, err)
					}
					logger.Debugf("removing dead unit %q", unitID)
					// Remove the unit from state.
					if err := op.config.UnitRemover.RemoveUnit(unitID); err != nil {
						return errors.Trace(err)
					}
					logger.Debugf("removing unit dir for dead unit %q", unitID)
					// Remove the unit's directory
					if err := op.removeUnitDir(unitTag); err != nil {
						return errors.Trace(err)
					}
					// Nothing to do for a dead unit further.
					continue
				} else {
					if _, ok := unitRunningChannels[unitID]; !ok && op.deploymentMode != caas.ModeOperator {
						// We make a buffered channel here so that we don't
						// block the operator while the uniter may not be ready
						unitRunningChannels[unitID] = make(chan struct{}, 1)
					}
				}
				// Start a worker to manage any new units.
				if _, err := op.runner.Worker(unitID, op.catacomb.Dying()); err == nil || unitLife == life.Dead {
					// Already watching the unit or we're
					// not yet watching it and it's dead.
					continue
				}

				// Make all the required symlinks.
				if err := op.makeAgentSymlinks(unitTag); err != nil {
					return errors.Trace(err)
				}
				params := *op.config.UniterParams
				params.ModelType = model.CAAS
				params.UnitTag = unitTag
				params.Downloader = op.config.Downloader // TODO(caas): write a cache downloader
				params.UniterFacade = op.config.UniterFacadeFunc(unitTag)
				if params.ResourcesFacade, err = op.config.ResourcesFacadeFunc(unitTag); err != nil {
					return errors.Trace(err)
				}
				params.PayloadFacade = op.config.PayloadFacadeFunc()
				params.LeadershipTrackerFunc = op.config.LeadershipTrackerFunc
				params.Logger = params.Logger.Child(unitID)
				if op.deploymentMode != caas.ModeOperator {
					params.IsRemoteUnit = true
					params.ContainerRunningStatusChannel = unitRunningChannels[unitID]

					execClient, err := op.config.ExecClientGetter()
					if err != nil {
						return errors.Trace(err)
					}
					params.ContainerRunningStatusFunc = func(providerID string) (*uniterremotestate.ContainerRunningStatus, error) {
						if wrench.IsActive("remote-init", "fatal-error") {
							return nil, errors.New("fake remote-init fatal-error")
						}
						return op.runningStatus(execClient, unitTag, providerID)
					}
					params.RemoteInitFunc = func(runningStatus uniterremotestate.ContainerRunningStatus, cancel <-chan struct{}) error {
						// TODO(caas): Remove the cached status uniterremotestate.ContainerRunningStatus from uniter watcher snapshot.
						return op.remoteInitForUniter(execClient, unitTag, runningStatus, cancel)
					}
					params.NewRemoteRunnerExecutor = getNewRunnerExecutor(logger, execClient)
				}
				if err := op.config.StartUniterFunc(op.runner, &params); err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (op *caasOperator) runningStatus(client exec.Executor, unit names.UnitTag, providerID string) (*uniterremotestate.ContainerRunningStatus, error) {
	op.config.Logger.Debugf("request running status for %q %s", unit.String(), providerID)
	params := exec.StatusParams{
		PodName: providerID,
	}
	status, err := client.Status(params)
	if err != nil {
		op.config.Logger.Errorf("could not get pod %q %q %v", unit.String(), providerID, err)
		return nil, errors.Annotatef(err, "getting pod status for unit %q, container %q", unit, providerID)
	}
	result := &uniterremotestate.ContainerRunningStatus{
		PodName: status.PodName,
	}
	once := true
	for _, cs := range status.ContainerStatus {
		if cs.InitContainer && cs.Name == caas.InitContainerName {
			result.Initialising = cs.Running
			result.InitialisingTime = cs.StartedAt
		}
		// Only check the default container.
		if !cs.InitContainer && !cs.EphemeralContainer && once {
			result.Running = cs.Running
			once = false
		}
	}
	return result, nil
}
func (op *caasOperator) remoteInitForUniter(client exec.Executor, unit names.UnitTag, runningStatus uniterremotestate.ContainerRunningStatus, cancel <-chan struct{}) error {
	return runnerWithRetry(
		func() error {
			status, err := op.runningStatus(client, unit, runningStatus.PodName)
			//  get the current status rather than using the status cached in remote state.
			if err != nil {
				return errors.Trace(err)
			}
			return op.remoteInit(client, unit, *status, cancel)
		},
		func(err error) bool {
			// We need to re-fetch the running status then retry remoteInit if the container is not running.
			return err != nil && !exec.IsContainerNotRunningError(err) && !errors.IsNotFound(err)
		}, op.config.Logger, op.config.Clock, cancel,
	)
}

func (op *caasOperator) remoteInit(client exec.Executor, unit names.UnitTag, runningStatus uniterremotestate.ContainerRunningStatus, cancel <-chan struct{}) error {
	op.config.Logger.Debugf("remote init for %q %+v", unit.String(), runningStatus)
	switch {
	case runningStatus.Initialising:
		// all good, continue to do remote-init.
		return errors.Trace(initializeUnit(initializeUnitParams{
			ExecClient:   client,
			Logger:       op.config.Logger,
			OperatorInfo: op.config.OperatorInfo,
			Paths:        op.paths,
			UnitTag:      unit,
			ProviderID:   runningStatus.PodName,
			WriteFile:    os.WriteFile,
			TempDir:      os.MkdirTemp,
			Clock:        op.config.Clock,
			ReTrier:      runnerWithRetry,
		}, cancel))
	case runningStatus.Running:
		op.config.Logger.Debugf("no need to do remote-init for a running container")
		return nil
	default:
		return errors.NotFoundf("container not running")
	}
}

func (op *caasOperator) charmModified(local *LocalState, remote remotestate.Snapshot) bool {
	// CAAS models may not yet have read the charm url from state.
	if remote.CharmURL == nil {
		return false
	}
	if local == nil || local.CharmURL == nil {
		op.config.Logger.Warningf("unexpected nil local charm URL")
		return true
	}
	if *local.CharmURL != *remote.CharmURL {
		op.config.Logger.Debugf("upgrade from %v to %v", local.CharmURL, remote.CharmURL)
		return true
	}

	if local.CharmModifiedVersion != remote.CharmModifiedVersion {
		op.config.Logger.Debugf("upgrade from CharmModifiedVersion %v to %v", local.CharmModifiedVersion, remote.CharmModifiedVersion)
		return true
	}
	if remote.ForceCharmUpgrade {
		op.config.Logger.Debugf("force charm upgrade to %v", remote.CharmURL)
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
