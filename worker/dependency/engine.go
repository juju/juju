// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.dependency")

// EngineConfig defines the parameters needed to create a new engine.
type EngineConfig struct {

	// IsFatal returns true when passed an error that should stop the engine.
	// It must not be nil.
	IsFatal IsFatalFunc

	// WorstError returns the more important of two fatal errors passed to it,
	// and is used to determine which fatal error to report when there's more
	// than one. It must not be nil.
	WorstError WorstErrorFunc

	// ErrorDelay controls how long the engine waits before restarting a worker
	// that encountered an unknown error. It must not be negative.
	ErrorDelay time.Duration

	// BounceDelay controls how long the engine waits before restarting a worker
	// that was deliberately shut down because its dependencies changed. It must
	// not be negative.
	BounceDelay time.Duration
}

// Validate returns an error if any field is invalid.
func (config *EngineConfig) Validate() error {
	if config.IsFatal == nil {
		return errors.New("IsFatal not specified")
	}
	if config.WorstError == nil {
		return errors.New("WorstError not specified")
	}
	if config.ErrorDelay < 0 {
		return errors.New("ErrorDelay is negative")
	}
	if config.BounceDelay < 0 {
		return errors.New("BounceDelay is negative")
	}
	return nil
}

// NewEngine returns an Engine that will maintain any installed Manifolds until
// either the engine is stopped or one of the manifolds' workers returns an error
// that satisfies isFatal. The caller takes responsibility for the returned Engine:
// it's responsible for Kill()ing the Engine when no longer used, and must handle
// any error from Wait().
func NewEngine(config EngineConfig) (Engine, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotatef(err, "invalid config")
	}
	engine := &engine{
		config: config,

		manifolds:  Manifolds{},
		dependents: map[string][]string{},
		current:    map[string]workerInfo{},

		install: make(chan installTicket),
		started: make(chan startedTicket),
		stopped: make(chan stoppedTicket),
		report:  make(chan reportTicket),
	}
	go func() {
		defer engine.tomb.Done()
		engine.tomb.Kill(engine.loop())
	}()
	return engine, nil
}

// engine maintains workers corresponding to its installed manifolds, and
// restarts them whenever their inputs change.
type engine struct {

	// config contains values passed in as config when the engine was created.
	config EngineConfig

	// As usual, we use tomb.Tomb to track the lifecycle and error state of the
	// engine worker itself; but we *only* report *internal* errors via the tomb.
	// Fatal errors received from workers are *not* used to kill the tomb; they
	// are tracked separately, and will only be exposed to the client when the
	// engine's tomb has completed its job and encountered no errors.
	tomb tomb.Tomb

	// worstError is used to track the most important fatal error we've received
	// from any manifold. This should be the only place fatal errors are stored;
	// they must *not* be passed into the tomb.
	worstError error

	// manifolds holds the installed manifolds by name.
	manifolds Manifolds

	// dependents holds, for each named manifold, those that depend on it.
	dependents map[string][]string

	// current holds the active worker information for each installed manifold.
	current map[string]workerInfo

	// install, started, report and stopped each communicate requests and changes into
	// the loop goroutine.
	install chan installTicket
	started chan startedTicket
	stopped chan stoppedTicket
	report  chan reportTicket
}

// loop serializes manifold install operations and worker start/stop notifications.
// It's notable for its oneShotDying var, which is necessary because any number of
// start/stop notification could be in flight at the point the engine needs to stop;
// we need to handle all those, and any subsequent messages, until the main loop is
// confident that every worker has stopped. (The usual pattern -- to defer a cleanup
// method to run before tomb.Done in NewEngine -- is not cleanly applicable, because
// it needs to duplicate that start/stop message handling; better to localise that
// in this method.)
func (engine *engine) loop() error {
	oneShotDying := engine.tomb.Dying()
	for {
		select {
		case <-oneShotDying:
			oneShotDying = nil
			for name := range engine.current {
				engine.requestStop(name)
			}
		case ticket := <-engine.report:
			// This is safe so long as the Report method reads the result.
			ticket.result <- engine.liveReport()
		case ticket := <-engine.install:
			// This is safe so long as the Install method reads the result.
			ticket.result <- engine.gotInstall(ticket.name, ticket.manifold)
		case ticket := <-engine.started:
			engine.gotStarted(ticket.name, ticket.worker, ticket.resourceLog)
		case ticket := <-engine.stopped:
			engine.gotStopped(ticket.name, ticket.error, ticket.resourceLog)
		}
		if engine.isDying() {
			if engine.allOthersStopped() {
				return tomb.ErrDying
			}
		}
	}
}

// Kill is part of the worker.Worker interface.
func (engine *engine) Kill() {
	engine.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (engine *engine) Wait() error {
	if tombError := engine.tomb.Wait(); tombError != nil {
		return tombError
	}
	return engine.worstError
}

// Report is part of the Reporter interface.
func (engine *engine) Report() map[string]interface{} {
	report := make(chan map[string]interface{})
	select {
	case engine.report <- reportTicket{report}:
		// This is safe so long as the loop sends a result.
		return <-report
	case <-engine.tomb.Dead():
		// Note that we don't abort on Dying as we usually would; the
		// oneShotDying approach in loop means that it can continue to
		// process requests until the last possible moment. Only once
		// loop has exited do we fall back to this report.
		return map[string]interface{}{
			KeyState:     "stopped",
			KeyError:     engine.Wait(),
			KeyManifolds: engine.manifoldsReport(),
		}
	}
}

// liveReport collects and returns information about the engine, its manifolds,
// and their workers. It must only be called from the loop goroutine.
func (engine *engine) liveReport() map[string]interface{} {
	var reportError error
	state := "started"
	if engine.isDying() {
		state = "stopping"
		if tombError := engine.tomb.Err(); tombError != nil {
			reportError = tombError
		} else {
			reportError = engine.worstError
		}
	}
	return map[string]interface{}{
		KeyState:     state,
		KeyError:     reportError,
		KeyManifolds: engine.manifoldsReport(),
	}
}

// manifoldsReport collects and returns information about the engine's manifolds
// and their workers. Until the tomb is Dead, it should only be called from the
// loop goroutine; after that, it's goroutine-safe.
func (engine *engine) manifoldsReport() map[string]interface{} {
	manifolds := map[string]interface{}{}
	for name, info := range engine.current {
		manifolds[name] = map[string]interface{}{
			KeyState:       info.state(),
			KeyError:       info.err,
			KeyInputs:      engine.manifolds[name].Inputs,
			KeyReport:      info.report(),
			KeyResourceLog: resourceLogReport(info.resourceLog),
		}
	}
	return manifolds
}

// Install is part of the Engine interface.
func (engine *engine) Install(name string, manifold Manifold) error {
	result := make(chan error)
	select {
	case <-engine.tomb.Dying():
		return errors.New("engine is shutting down")
	case engine.install <- installTicket{name, manifold, result}:
		// This is safe so long as the loop sends a result.
		return <-result
	}
}

// gotInstall handles the params originally supplied to Install. It must only be
// called from the loop goroutine.
func (engine *engine) gotInstall(name string, manifold Manifold) error {
	logger.Tracef("installing %q manifold...", name)
	if _, found := engine.manifolds[name]; found {
		return errors.Errorf("%q manifold already installed", name)
	}
	if err := engine.checkAcyclic(name, manifold); err != nil {
		return errors.Annotatef(err, "cannot install %q manifold", name)
	}
	engine.manifolds[name] = manifold
	for _, input := range manifold.Inputs {
		engine.dependents[input] = append(engine.dependents[input], name)
	}
	engine.current[name] = workerInfo{}
	engine.requestStart(name, 0)
	return nil
}

// uninstall removes the named manifold from the engine's records.
func (engine *engine) uninstall(name string) {
	// Note that we *don't* want to remove dependents[name] -- all those other
	// manifolds do still depend on this, and another manifold with the same
	// name might be installed in the future -- but we do want to remove the
	// named manifold from all *values* in the dependents map.
	for dName, dependents := range engine.dependents {
		depSet := set.NewStrings(dependents...)
		depSet.Remove(name)
		engine.dependents[dName] = depSet.Values()
	}
	delete(engine.current, name)
	delete(engine.manifolds, name)
}

// checkAcyclic returns an error if the introduction of the supplied manifold
// would cause the dependency graph to contain cycles.
func (engine *engine) checkAcyclic(name string, manifold Manifold) error {
	manifolds := Manifolds{name: manifold}
	for name, manifold := range engine.manifolds {
		manifolds[name] = manifold
	}
	return Validate(manifolds)
}

// requestStart invokes a runWorker goroutine for the manifold with the supplied
// name. It must only be called from the loop goroutine.
func (engine *engine) requestStart(name string, delay time.Duration) {

	// Check preconditions.
	manifold, found := engine.manifolds[name]
	if !found {
		engine.tomb.Kill(errors.Errorf("fatal: unknown manifold %q", name))
	}

	// Copy current info and check more preconditions.
	info := engine.current[name]
	if !info.stopped() {
		engine.tomb.Kill(errors.Errorf("fatal: trying to start a second %q manifold worker", name))
	}

	// Final check that we're not shutting down yet...
	if engine.isDying() {
		logger.Tracef("not starting %q manifold worker (shutting down)", name)
		return
	}

	// ...then update the info, copy it back to the engine, and start a worker
	// goroutine based on current known state.
	info.starting = true
	engine.current[name] = info
	resourceGetter := engine.resourceGetter(name, manifold.Inputs)
	go engine.runWorker(name, delay, manifold.Start, resourceGetter)
}

// resourceGetter returns a resourceGetter backed by a snapshot of current
// worker state, restricted to those workers declared in inputs. It must only
// be called from the loop goroutine; see inside for a detailed dicsussion of
// why we took this appproach.
func (engine *engine) resourceGetter(name string, inputs []string) *resourceGetter {
	// We snapshot the resources available at invocation time, rather than adding an
	// additional communicate-resource-request channel. The latter approach is not
	// unreasonable... but is prone to inelegant scrambles when starting several
	// dependent workers at once. For example:
	//
	//  * Install manifold A; loop starts worker A
	//  * Install manifold B; loop starts worker B
	//  * A communicates its worker back to loop; main thread bounces B
	//  * B asks for A, gets A, doesn't react to bounce (*)
	//  * B communicates its worker back to loop; loop kills it immediately in
	//    response to earlier bounce
	//  * loop starts worker B again, now everything's fine; but, still, yuck.
	//    This is not a happy path to take by default.
	//
	// The problem, of course, is in the (*); the main thread *does* know that B
	// needs to bounce soon anyway, and it *could* communicate that fact back via
	// an error over a channel back into getResource; the StartFunc could then
	// just return (say) that ErrResourceChanged and avoid the hassle of creating
	// a worker. But that adds a whole layer of complexity (and unpredictability
	// in tests, which is not much fun) for very little benefit.
	//
	// In the analogous scenario with snapshotted dependencies, we see a happier
	// picture at startup time:
	//
	//  * Install manifold A; loop starts worker A
	//  * Install manifold B; loop starts worker B with empty resource snapshot
	//  * A communicates its worker back to loop; main thread bounces B
	//  * B's StartFunc asks for A, gets nothing, returns ErrMissing
	//  * loop restarts worker B with an up-to-date snapshot, B works fine
	//
	// We assume that, in the common case, most workers run without error most
	// of the time; and, thus, that the vast majority of worker startups will
	// happen as an agent starts. Furthermore, most of them will have simple
	// hard dependencies, and their Start funcs will be easy to write; the only
	// components that may be impacted by such a strategy will be those workers
	// which still want to run (with reduced functionality) with some dependency
	// unmet.
	//
	// Those may indeed suffer the occasional extra bounce as the system comes
	// to stability as it starts, or after a change; but workers *must* be
	// written for resilience in the face of arbitrary bounces *anyway*, so it
	// shouldn't be harmful.
	outputs := map[string]OutputFunc{}
	workers := map[string]worker.Worker{}
	for _, resourceName := range inputs {
		outputs[resourceName] = engine.manifolds[resourceName].Output
		workers[resourceName] = engine.current[resourceName].worker
	}
	return &resourceGetter{
		clientName: name,
		expired:    make(chan struct{}),
		workers:    workers,
		outputs:    outputs,
	}
}

// runWorker starts the supplied manifold's worker and communicates it back to the
// loop goroutine; waits for worker completion; and communicates any error encountered
// back to the loop goroutine. It must not be run on the loop goroutine.
func (engine *engine) runWorker(name string, delay time.Duration, start StartFunc, resourceGetter *resourceGetter) {

	errAborted := errors.New("aborted before delay elapsed")

	startAfterDelay := func() (worker.Worker, error) {
		// NOTE: the resourceGetter will expire *after* the worker is started.
		// This is tolerable because
		//  1) we'll still correctly block access attempts most of the time
		//  2) failing to block them won't cause data races anyway
		//  3) it's not worth complicating the interface for every client just
		//     to eliminate the possibility of one harmlessly dumb interaction.
		defer resourceGetter.expire()
		logger.Tracef("starting %q manifold worker in %s...", name, delay)
		select {
		case <-time.After(delay):
		case <-engine.tomb.Dying():
			return nil, errAborted
		}
		logger.Tracef("starting %q manifold worker", name)
		return start(resourceGetter.getResource)
	}

	startWorkerAndWait := func() error {
		worker, err := startAfterDelay()
		switch errors.Cause(err) {
		case errAborted:
			return nil
		case nil:
			logger.Tracef("running %q manifold worker", name)
		default:
			logger.Tracef("failed to start %q manifold worker: %v", name, err)
			return err
		}
		select {
		case <-engine.tomb.Dying():
			logger.Tracef("stopping %q manifold worker (shutting down)", name)
			// Doesn't matter whether worker == engine: if we're already Dying
			// then cleanly Kill()ing ourselves again won't hurt anything.
			worker.Kill()
		case engine.started <- startedTicket{name, worker, resourceGetter.accessLog}:
			logger.Tracef("registered %q manifold worker", name)
		}
		if worker == engine {
			// We mustn't Wait() for ourselves to complete here, or we'll
			// deadlock. But we should wait until we're Dying, because we
			// need this func to keep running to keep the self manifold
			// accessible as a resource.
			<-engine.tomb.Dying()
			return tomb.ErrDying
		}

		return worker.Wait()
	}

	// We may or may not send on started, but we *must* send on stopped.
	engine.stopped <- stoppedTicket{name, startWorkerAndWait(), resourceGetter.accessLog}
}

// gotStarted updates the engine to reflect the creation of a worker. It must
// only be called from the loop goroutine.
func (engine *engine) gotStarted(name string, worker worker.Worker, resourceLog []resourceAccess) {
	// Copy current info; check preconditions and abort the workers if we've
	// already been asked to stop it.
	info := engine.current[name]
	switch {
	case info.worker != nil:
		engine.tomb.Kill(errors.Errorf("fatal: unexpected %q manifold worker start", name))
		fallthrough
	case info.stopping, engine.isDying():
		logger.Tracef("%q manifold worker no longer required", name)
		worker.Kill()
	default:
		// It's fine to use this worker; update info and copy back.
		logger.Debugf("%q manifold worker started", name)
		engine.current[name] = workerInfo{
			worker:      worker,
			resourceLog: resourceLog,
		}

		// Any manifold that declares this one as an input needs to be restarted.
		engine.bounceDependents(name)
	}
}

// gotStopped updates the engine to reflect the demise of (or failure to create)
// a worker. It must only be called from the loop goroutine.
func (engine *engine) gotStopped(name string, err error, resourceLog []resourceAccess) {
	logger.Debugf("%q manifold worker stopped: %v", name, err)

	// Copy current info and check for reasons to stop the engine.
	info := engine.current[name]
	if info.stopped() {
		engine.tomb.Kill(errors.Errorf("fatal: unexpected %q manifold worker stop", name))
	} else if engine.config.IsFatal(err) {
		engine.worstError = engine.config.WorstError(err, engine.worstError)
		engine.tomb.Kill(nil)
	}

	// Reset engine info; and bail out if we can be sure there's no need to bounce.
	engine.current[name] = workerInfo{
		err:         err,
		resourceLog: resourceLog,
	}
	if engine.isDying() {
		logger.Tracef("permanently stopped %q manifold worker (shutting down)", name)
		return
	}

	// If we told the worker to stop, we should start it again immediately,
	// whatever else happened.
	if info.stopping {
		engine.requestStart(name, engine.config.BounceDelay)
	} else {
		// If we didn't stop it ourselves, we need to interpret the error.
		switch errors.Cause(err) {
		case nil:
			// Nothing went wrong; the task completed successfully. Nothing
			// needs to be done (unless the inputs change, in which case it
			// gets to check again).
		case ErrMissing:
			// The task can't even start with the current state. Nothing more
			// can be done (until the inputs change, in which case we retry
			// anyway).
		case ErrBounce:
			// The task exited but wanted to restart immediately.
			engine.requestStart(name, engine.config.BounceDelay)
		case ErrUninstall:
			// The task should never run again, and can be removed completely.
			engine.uninstall(name)
		default:
			// Something went wrong but we don't know what. Try again soon.
			logger.Errorf("%q manifold worker returned unexpected error: %v", name, err)
			engine.requestStart(name, engine.config.ErrorDelay)
		}
	}

	// Manifolds that declared a dependency on this one only need to be notified
	// if the worker has changed; if it was already nil, nobody needs to know.
	if info.worker != nil {
		engine.bounceDependents(name)
	}
}

// requestStop ensures that any running or starting worker will be stopped in the
// near future. It must only be called from the loop goroutine.
func (engine *engine) requestStop(name string) {

	// If already stopping or stopped, just don't do anything.
	info := engine.current[name]
	if info.stopping || info.stopped() {
		return
	}

	// Update info, kill worker if present, and copy info back to engine.
	info.stopping = true
	if info.worker != nil {
		info.worker.Kill()
	}
	engine.current[name] = info
}

// isDying returns true if the engine is shutting down. It's safe to call it
// from any goroutine.
func (engine *engine) isDying() bool {
	select {
	case <-engine.tomb.Dying():
		return true
	default:
		return false
	}
}

// allOthersStopped returns true if no workers (other than the engine itself,
// if it happens to have been injected) are running or starting. It must only
// be called from the loop goroutine.
func (engine *engine) allOthersStopped() bool {
	for _, info := range engine.current {
		if !info.stopped() && info.worker != engine {
			return false
		}
	}
	return true
}

// bounceDependents starts every stopped dependent of the named manifold, and
// stops every started one (and trusts the rest of the engine to restart them).
// It must only be called from the loop goroutine.
func (engine *engine) bounceDependents(name string) {
	logger.Tracef("restarting dependents of %q manifold", name)
	for _, dependentName := range engine.dependents[name] {
		if engine.current[dependentName].stopped() {
			engine.requestStart(dependentName, engine.config.BounceDelay)
		} else {
			engine.requestStop(dependentName)
		}
	}
}

// workerInfo stores what an engine's loop goroutine needs to know about the
// worker for a given Manifold.
type workerInfo struct {
	starting    bool
	stopping    bool
	worker      worker.Worker
	err         error
	resourceLog []resourceAccess
}

// stopped returns true unless the worker is either assigned or starting.
func (info workerInfo) stopped() bool {
	switch {
	case info.worker != nil:
		return false
	case info.starting:
		return false
	}
	return true
}

// state returns the latest known state of the worker, for use in reports.
func (info workerInfo) state() string {
	switch {
	case info.starting:
		return "starting"
	case info.stopping:
		return "stopping"
	case info.worker != nil:
		return "started"
	}
	return "stopped"
}

// report returns any available report from the worker. If the worker is not
// a Reporter, or is not present, this method will return nil.
func (info workerInfo) report() map[string]interface{} {
	if reporter, ok := info.worker.(Reporter); ok {
		return reporter.Report()
	}
	return nil
}

// installTicket is used by engine to induce installation of a named manifold
// and pass on any errors encountered in the process.
type installTicket struct {
	name     string
	manifold Manifold
	result   chan<- error
}

// startedTicket is used by engine to notify the loop of the creation of the
// worker for a particular manifold.
type startedTicket struct {
	name        string
	worker      worker.Worker
	resourceLog []resourceAccess
}

// stoppedTicket is used by engine to notify the loop of the demise of (or
// failure to create) the worker for a particular manifold.
type stoppedTicket struct {
	name        string
	error       error
	resourceLog []resourceAccess
}

// reportTicket is used by the engine to notify the loop that a status report
// should be generated.
type reportTicket struct {
	result chan map[string]interface{}
}
