// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/retry.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/database/txn"
)

const (
	// maxRetries gives the maximum number of attempts we'll try if
	// there are timeouts.
	maxRetries = 10

	// initialRetryDelay is the starting delay - this will be
	// increased exponentially up maxRetries.
	initialRetryDelay = 50 * time.Millisecond

	// retryBackoffFactor is how much longer we wait after a failing retry.
	// Retrying 10 times starting at 50ms and backing off 1.6x gives us a total
	// delay time of about 9s.
	retryBackoffFactor = 1.6

	// maxShutdownWait is the maximum time to wait for the async
	// claims and expires to complete before stopping the worker
	// anyway. Picked to be slightly quicker than the httpserver
	// shutdown timeout.
	maxShutdownWait = 55 * time.Second
)

// errStopped is returned to clients when an operation cannot complete because
// the manager has started (and possibly finished) shutdown.
var errStopped = errors.New("lease manager stopped")

// NewManager returns a new *Manager configured as supplied. The caller takes
// responsibility for killing, and handling errors from, the returned Worker.
func NewManager(config ManagerConfig) (*Manager, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	logContext := config.EntityUUID
	if len(logContext) > 6 {
		logContext = logContext[:6]
	}
	manager := &Manager{
		config:     config,
		claims:     make(chan claim),
		revokes:    make(chan revoke),
		checks:     make(chan check),
		blocks:     make(chan block),
		expireDone: make(chan struct{}),
		pins:       make(chan pin),
		unpins:     make(chan pin),
		logContext: logContext,
	}
	manager.tomb.Go(manager.loop)
	return manager, nil
}

// Manager implements worker.Worker and can be bound to get
// lease.Checkers and lease.Claimers.
type Manager struct {
	tomb tomb.Tomb

	// config collects all external configuration and dependencies.
	config ManagerConfig

	// logContext is just a string that associates messages in the log
	// It is seeded with the first six characters of the config.EntityUUID
	// if supplied
	logContext string

	// nextTimeout is the next time that has a possible expiry that we would care
	// about, capped at the maximum time.
	nextTimeout time.Time

	// timer tracks when nextTimeout would expire and triggers when it does
	timer clock.Timer

	// claims is used to deliver lease claim requests to the loop.
	claims chan claim

	// revokes is used to deliver lease revoke requests to the loop.
	revokes chan revoke

	// checks is used to deliver lease check requests to the loop.
	checks chan check

	// expireDone is sent an event when we successfully finish a call to expire()
	expireDone chan struct{}

	// blocks is used to deliver expiry block requests to the loop.
	blocks chan block

	// pins is used to deliver lease pin requests to the loop.
	pins chan pin

	// unpins is used to deliver lease unpin requests to the loop.
	unpins chan pin

	// wg is used to ensure that all child goroutines are finished
	// before we stop.
	wg sync.WaitGroup

	// outstandingClaims tracks how many unfinished claim goroutines
	// are running (for debugging purposes).
	outstandingClaims int64

	// outstandingRevokes tracks how many unfinished revoke goroutines
	// are running (for debugging purposes).
	outstandingRevokes int64
}

// Kill is part of the worker.Worker interface.
func (manager *Manager) Kill() {
	manager.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (manager *Manager) Wait() error {
	return manager.tomb.Wait()
}

// loop runs until the manager is stopped.
func (manager *Manager) loop() error {
	if collector, ok := manager.config.Store.(prometheus.Collector); ok && manager.config.PrometheusRegisterer != nil {
		// The store implements the collector interface, but the lease.Store
		// does not expose those.
		_ = manager.config.PrometheusRegisterer.Register(collector)
		defer manager.config.PrometheusRegisterer.Unregister(collector)
	}

	defer manager.waitForGoroutines()

	// This context is passed into all lease store operations.
	// Doing this ensures that no such operations can block worker shutdown.
	// Killing the tomb, cancels the context.
	ctx := manager.tomb.Context(context.Background())
	ctx = trace.WithTracer(ctx, manager.config.Tracer)

	leases, err := manager.config.Store.Leases(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	manager.computeNextTimeout(leases)

	blocks := make(blocks)
	for {
		if err := manager.choose(ctx, blocks); err != nil {
			if errors.Is(err, tomb.ErrDying) {
				err = manager.tomb.Err()
			}
			manager.config.Logger.Tracef("[%s] exiting main loop with error: %v", manager.logContext, err)
			return errors.Trace(err)
		}
	}
}

func (manager *Manager) lookupLease(ctx context.Context, leaseKey lease.Key) (lease.Info, bool, error) {
	leases, err := manager.config.Store.Leases(ctx, leaseKey)
	if err != nil {
		return lease.Info{}, false, errors.Trace(err)
	}

	l, exists := leases[leaseKey]
	return l, exists, nil
}

// choose breaks the select out of loop to make the blocking logic clearer.
func (manager *Manager) choose(ctx context.Context, blocks blocks) error {
	select {
	case <-manager.tomb.Dying():
		return tomb.ErrDying

	case check := <-manager.checks:
		return manager.handleCheck(ctx, check)

	case now := <-manager.timer.Chan():
		if err := manager.tick(ctx, now, blocks); err != nil {
			return errors.Trace(err)
		}

	case <-manager.expireDone:
		if err := manager.checkBlocks(ctx, blocks); err != nil {
			return errors.Trace(err)
		}

	case claim := <-manager.claims:
		manager.startingClaim()
		go manager.withTrace(ctx, "claim", func() {
			manager.retryingClaim(ctx, claim)
		})

	case revoke := <-manager.revokes:
		manager.startingRevoke()
		go manager.withTrace(ctx, "revoke", func() {
			manager.retryingRevoke(ctx, revoke)
		})

	case pin := <-manager.pins:
		manager.withTrace(ctx, "pin", func() {
			manager.handlePin(ctx, pin)
		})

	case unpin := <-manager.unpins:
		manager.withTrace(ctx, "unpin", func() {
			manager.handleUnpin(ctx, unpin)
		})

	case block := <-manager.blocks:
		manager.config.Logger.Tracef("[%s] adding block for: %s", manager.logContext, block.leaseKey.Lease)
		blocks.add(block)
	}
	return nil
}

func (manager *Manager) bind(namespace, modelUUID string) (broker, error) {
	finder := manager.config.SecretaryFinder
	secretary, err := finder.SecretaryFor(namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &boundManager{
		manager:   manager,
		secretary: secretary,
		namespace: namespace,
		modelUUID: modelUUID,
	}, nil
}

// Checker returns a lease.Checker for the specified namespace and model.
func (manager *Manager) Checker(namespace, modelUUID string) (lease.Checker, error) {
	return manager.bind(namespace, modelUUID)
}

// Claimer returns a lease.Claimer for the specified namespace and model.
func (manager *Manager) Claimer(namespace, modelUUID string) (lease.Claimer, error) {
	return manager.bind(namespace, modelUUID)
}

// Revoker returns a lease.Revoker for the specified namespace and model.
func (manager *Manager) Revoker(namespace, modelUUID string) (lease.Revoker, error) {
	return manager.bind(namespace, modelUUID)
}

// Pinner returns a lease.Pinner for the specified namespace and model.
func (manager *Manager) Pinner(namespace, modelUUID string) (lease.Pinner, error) {
	return manager.bind(namespace, modelUUID)
}

// Reader returns a lease.Reader for the specified namespace and model.
func (manager *Manager) Reader(namespace, modelUUID string) (lease.Reader, error) {
	return manager.bind(namespace, modelUUID)
}

// retryingClaim handles timeouts when claiming, and responds to the
// claiming party when it eventually succeeds or fails, or if it times
// out after a number of retries.
func (manager *Manager) retryingClaim(ctx context.Context, claim claim) {
	defer manager.finishedClaim()
	var (
		err     error
		success bool
	)

	for a := manager.startRetry(); a.Next(); {
		var act action
		act, success, err = manager.handleClaim(ctx, claim)
		if isFatalClaimRetryError(act, err, a.Count()) {
			break
		}

		if a.More() {
			switch {
			case lease.IsInvalid(err):
				manager.config.Logger.Tracef("[%s] request by %s for lease %s %v, retrying...",
					manager.logContext, claim.holderName, claim.leaseKey.Lease, err)

			default:
				manager.config.Logger.Tracef("[%s] timed out handling claim by %s for lease %s, retrying...",
					manager.logContext, claim.holderName, claim.leaseKey.Lease)
			}
		}
	}

	if err == nil {
		if !success {
			claim.respond(lease.ErrClaimDenied)
			return
		}
		claim.respond(nil)
	} else {
		switch {
		case lease.IsTimeout(err), txn.IsErrRetryable(err):
			manager.config.Logger.Warningf("[%s] retrying timed out while handling claim %q for %q",
				manager.logContext, claim.leaseKey, claim.holderName)
			claim.respond(lease.ErrTimeout)

		case lease.IsInvalid(err):
			// We want to see this, but it doesn't indicate something a user
			// can do something about.
			manager.config.Logger.Infof("[%s] got %v after %d retries, denying claim %q for %q",
				manager.logContext, err, maxRetries, claim.leaseKey, claim.holderName)
			claim.respond(lease.ErrClaimDenied)

		case lease.IsHeld(err):
			// This can happen in HA if the original check for an extant lease
			// (against the local node) returned nothing, but the leader FSM
			// has this lease being held by another entity.
			manager.config.Logger.Tracef(
				"[%s] %s asked for lease %s, held by by another entity",
				manager.logContext, claim.holderName, claim.leaseKey.Lease)
			claim.respond(lease.ErrClaimDenied)

		default:
			// Stop the main loop because we got an abnormal error
			manager.tomb.Kill(errors.Trace(err))
		}
	}
}

// handleClaim processes the supplied claim. It will only return
// unrecoverable errors or timeouts; mere failure to claim just
// indicates a bad request, and is returned as (false, nil).
func (manager *Manager) handleClaim(ctx context.Context, claim claim) (action, bool, error) {
	logger := manager.config.Logger
	var act action

	select {
	case <-manager.tomb.Dying():
		return "unknown", false, tomb.ErrDying
	default:
		info, found, err := manager.lookupLease(ctx, claim.leaseKey)
		if err != nil {
			return "unknown", false, errors.Trace(err)
		}

		request := lease.Request{Holder: claim.holderName, Duration: claim.duration}
		store := manager.config.Store

		switch {
		case !found:
			logger.Tracef("[%s] %s asked for lease %s (%s), no lease found, claiming for %s",
				manager.logContext, claim.holderName, claim.leaseKey.Lease, claim.leaseKey.Namespace, claim.duration)
			act = claimAction
			err = store.ClaimLease(ctx, claim.leaseKey, request)

		case info.Holder == claim.holderName:
			logger.Tracef("[%s] %s extending lease %s (%s) for %s",
				manager.logContext, claim.holderName, claim.leaseKey.Lease, claim.leaseKey.Namespace, claim.duration)
			act = extendAction
			err = store.ExtendLease(ctx, claim.leaseKey, request)

		default:
			// Note: (jam) 2017-10-31) We don't check here if the lease has
			// expired for the current holder. Should we?
			remaining := info.Expiry.Sub(manager.config.Clock.Now())
			logger.Tracef("[%s] %s asked for lease %s, held by %s for another %s, rejecting",
				manager.logContext, claim.holderName, claim.leaseKey.Lease, info.Holder, remaining)
			return "unknown", false, nil
		}

		if lease.IsAborted(err) {
			return act, false, tomb.ErrDying
		}
		if err != nil {
			return act, false, errors.Trace(err)
		}
	}

	logger.Tracef("[%s] %s %s lease %s for %s successful",
		manager.logContext, claim.holderName, act.String(), claim.leaseKey.Lease, claim.duration)
	return act, true, nil
}

func (manager *Manager) withTrace(ctx context.Context, action string, fn func()) {
	var err error

	ctx, span := trace.Start(ctx, trace.NameFromFunc(), trace.WithAttributes(
		trace.StringAttr("namespace.worker", "lease"),
		trace.StringAttr("namespace.action", "handle-"+action),
	))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	pprof.Do(ctx, pprof.Labels(trace.OTELTraceID, trace.TraceIDFromContext(ctx)), func(ctx context.Context) {
		fn()
	})
}

// retryingRevoke handles timeouts when revoking, and responds to the
// revoking party when it eventually succeeds or fails, or if it times
// out after a number of retries.
func (manager *Manager) retryingRevoke(ctx context.Context, revoke revoke) {
	defer manager.finishedRevoke()

	var err error
	for a := manager.startRetry(); a.Next(); {
		err = manager.handleRevoke(ctx, revoke)
		if isFatalRetryError(err) {
			break
		}

		if a.More() {
			switch {
			case lease.IsInvalid(err):
				manager.config.Logger.Tracef("[%s] request by %s for revoking lease %s %v, retrying...",
					manager.logContext, revoke.holderName, revoke.leaseKey.Lease, err)

			default:
				manager.config.Logger.Tracef("[%s] timed out handling revoke by %s for lease %s, retrying...",
					manager.logContext, revoke.holderName, revoke.leaseKey.Lease)
			}
		}
	}

	if err == nil {
		revoke.respond(nil)
		// If we send back an error, then the main loop won't listen for expireDone
		select {
		case <-manager.tomb.Dying():
			return
		case manager.expireDone <- struct{}{}:
		}
	} else {
		switch {
		case lease.IsTimeout(err), txn.IsErrRetryable(err):
			manager.config.Logger.Warningf("[%s] retrying timed out while handling revoke %q for %q",
				manager.logContext, revoke.leaseKey, revoke.holderName)
			revoke.respond(lease.ErrTimeout)

		case lease.IsInvalid(err):
			// we want to see this, but it doesn't indicate something a user can do something about
			manager.config.Logger.Infof("[%s] got %v after %d retries, revoke %q for %q",
				manager.logContext, err, maxRetries, revoke.leaseKey, revoke.holderName)
			revoke.respond(err)

		case lease.IsNotHeld(err):
			// we want to see this, but it doesn't indicate something a user can do something about
			manager.config.Logger.Infof("[%s] got %v after %d retries, revoke %q for %q",
				manager.logContext, err, maxRetries, revoke.leaseKey, revoke.holderName)
			revoke.respond(err)

		default:
			// Stop the main loop because we got an abnormal error
			manager.tomb.Kill(errors.Trace(err))
		}
	}
}

// handleRevoke processes the supplied revocation. It will only return
// unrecoverable errors or timeouts.
func (manager *Manager) handleRevoke(ctx context.Context, revoke revoke) error {
	logger := manager.config.Logger

	select {
	case <-manager.tomb.Dying():
		return tomb.ErrDying
	default:
		info, found, err := manager.lookupLease(ctx, revoke.leaseKey)
		if err != nil {
			return errors.Trace(err)
		}

		switch {
		case !found:
			logger.Tracef("[%s] %s asked to revoke lease %s, no lease found",
				manager.logContext, revoke.holderName, revoke.leaseKey.Lease)
			return nil

		case info.Holder == revoke.holderName:
			logger.Tracef("[%s] %s revoking lease %s", manager.logContext, revoke.holderName, revoke.leaseKey.Lease)
			err = manager.config.Store.RevokeLease(ctx, revoke.leaseKey, revoke.holderName)

		default:
			logger.Tracef("[%s] %s revoking lease %s, held by %s, rejecting",
				manager.logContext, revoke.holderName, revoke.leaseKey.Lease, info.Holder)
			return lease.ErrNotHeld
		}

		if lease.IsAborted(err) {
			return tomb.ErrDying
		}
		if err != nil {
			return errors.Trace(err)
		}
	}

	logger.Tracef("[%s] %s revoked lease %s successful", manager.logContext, revoke.holderName, revoke.leaseKey.Lease)
	return nil
}

// handleCheck processes and responds to the supplied check. It will only return
// unrecoverable errors; mere untruth of the assertion just indicates a bad
// request, and is communicated back to the check's originator.
func (manager *Manager) handleCheck(ctx context.Context, check check) (err error) {
	key := check.leaseKey

	manager.config.Logger.Tracef("[%s] handling Check for lease %s on behalf of %s",
		manager.logContext, key.Lease, check.holderName)

	info, found, err := manager.lookupLease(ctx, key)
	if err != nil {
		return errors.Trace(err)
	}

	var response error
	if !found || info.Holder != check.holderName {
		if found {
			manager.config.Logger.Tracef("[%s] handling Check for lease %s on behalf of %s, found held by %s",
				manager.logContext, key.Lease, check.holderName, info.Holder)
		} else {
			// Someone thought they were the lease-holder, otherwise they
			// wouldn't be confirming via the check. However, the lease has
			// expired, and they are out of sync. Schedule a block check.
			manager.setNextTimeout(manager.config.Clock.Now().Add(time.Second))

			manager.config.Logger.Tracef("[%s] handling Check for lease %s on behalf of %s, not found",
				manager.logContext, key.Lease, check.holderName)
		}

		response = lease.ErrNotHeld
	}
	check.respond(errors.Trace(response))
	return nil
}

// tick triggers when we think a lease might be expiring, so we check if there
// are leases to expire, and then unblock anything that is no longer blocked,
// and then compute the next time we should wake up.
func (manager *Manager) tick(ctx context.Context, now time.Time, blocks blocks) error {
	manager.config.Logger.Tracef("[%s] tick at %v, running expiry checks\n", manager.logContext, now)
	// Check for blocks that need to be notified.
	return errors.Trace(manager.checkBlocks(ctx, blocks))
}

func (manager *Manager) checkBlocks(ctx context.Context, blocks blocks) error {
	manager.config.Logger.Tracef("[%s] evaluating %d blocks", manager.logContext, len(blocks))

	leases, err := manager.config.Store.Leases(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	for leaseName := range blocks {
		if _, found := leases[leaseName]; !found {
			manager.config.Logger.Tracef("[%s] unblocking: %s", manager.logContext, leaseName)
			blocks.unblock(leaseName)
		}
	}
	manager.computeNextTimeout(leases)
	return nil
}

// computeNextTimeout iterates the leases and finds out what the next time we
// want to wake up, expire any leases and then handle any unblocks that happen.
// It is the earliest lease expiration due in the future, but before MaxSleep.
func (manager *Manager) computeNextTimeout(leases map[lease.Key]lease.Info) {
	now := manager.config.Clock.Now()
	nextTick := now.Add(manager.config.MaxSleep)
	for _, info := range leases {
		if info.Expiry.After(nextTick) {
			continue
		}
		nextTick = info.Expiry
	}

	// If we had leases set to expire in the past, then we assume that our FSM
	// is behind the leader and will soon indicate their expiration.
	// Check the blocks again soon.
	if !nextTick.After(now) {
		nextTick = now
	}

	// The lease clock ticks *at least* a second from now. Expirations only
	// occur when the global clock updater ticks the clock, so this avoids
	// too frequently checking with the potential of having no work to do.
	// The blanket addition of a second is no big deal.
	nextTick = nextTick.Add(time.Second)

	nextDuration := nextTick.Sub(now).Round(time.Millisecond)
	manager.config.Logger.Tracef("[%s] next expire in %v %v", manager.logContext, nextDuration, nextTick)
	manager.setNextTimeout(nextTick)
}

func (manager *Manager) setNextTimeout(t time.Time) {
	now := manager.config.Clock.Now()

	// Ensure we never walk the next check back without have performed a
	// scheduled check *unless* we think our last check was in the past.
	if !manager.nextTimeout.Before(now) && !t.Before(manager.nextTimeout) {
		manager.config.Logger.Tracef("[%s] not rescheduling check from %v to %v based on current time %v",
			manager.logContext, manager.nextTimeout, t, now)
		return
	}
	manager.nextTimeout = t

	d := t.Sub(now)
	if manager.timer == nil {
		manager.timer = manager.config.Clock.NewTimer(d)
	} else {
		// See the docs on Timer.Reset() that says it isn't safe to call
		// on a non-stopped channel, and if it is stopped, you need to check
		// if the channel needs to be drained anyway. It isn't safe to drain
		// unconditionally in case another goroutine has already noticed,
		// but make an attempt.
		if !manager.timer.Stop() {
			select {
			case <-manager.timer.Chan():
			default:
			}
		}
		manager.timer.Reset(d)
	}
}

func (manager *Manager) startRetry() *retry.Attempt {
	return retry.StartWithCancel(
		retry.LimitCount(maxRetries, retry.Exponential{
			Initial: initialRetryDelay,
			Factor:  retryBackoffFactor,
			Jitter:  true,
		}),
		manager.config.Clock,
		manager.tomb.Dying(),
	)
}

func (manager *Manager) handlePin(ctx context.Context, p pin) {
	p.respond(errors.Trace(manager.config.Store.PinLease(ctx, p.leaseKey, p.entity)))
}

func (manager *Manager) handleUnpin(ctx context.Context, p pin) {
	p.respond(errors.Trace(manager.config.Store.UnpinLease(ctx, p.leaseKey, p.entity)))
}

// pinned returns lease names and the entities requiring their pinned
// behaviour, from the input namespace/model for which leases are pinned.
func (manager *Manager) pinned(ctx context.Context, namespace, modelUUID string) (map[string][]string, error) {
	pinned, err := manager.config.Store.Pinned(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string][]string)
	for key, entities := range pinned {
		if key.Namespace == namespace && key.ModelUUID == modelUUID {
			result[key.Lease] = entities
		}
	}
	return result, nil
}

func (manager *Manager) leases(ctx context.Context, namespace, modelUUID string) (map[string]string, error) {
	group, err := manager.config.Store.LeaseGroup(ctx, namespace, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	leases := make(map[string]string)
	for key, info := range group {
		leases[key.Lease] = info.Holder
	}
	return leases, nil
}

func (manager *Manager) startingClaim() {
	atomic.AddInt64(&manager.outstandingClaims, 1)
	manager.wg.Add(1)
}

func (manager *Manager) finishedClaim() {
	manager.wg.Done()
	atomic.AddInt64(&manager.outstandingClaims, -1)
}

func (manager *Manager) startingRevoke() {
	atomic.AddInt64(&manager.outstandingRevokes, 1)
	manager.wg.Add(1)
}

func (manager *Manager) finishedRevoke() {
	manager.wg.Done()
	atomic.AddInt64(&manager.outstandingRevokes, -1)
}

// Report is part of dependency.Reporter
func (manager *Manager) Report() map[string]interface{} {
	out := make(map[string]interface{})
	out["entity-uuid"] = manager.config.EntityUUID
	out["outstanding-claims"] = atomic.LoadInt64(&manager.outstandingClaims)
	out["outstanding-revokes"] = atomic.LoadInt64(&manager.outstandingRevokes)
	return out
}

func (manager *Manager) waitForGoroutines() {
	// Wait for the waitgroup to finish, but only up to a point.
	groupDone := make(chan struct{})
	go func() {
		manager.wg.Wait()
		close(groupDone)
	}()

	select {
	case <-groupDone:
		return
	case <-manager.config.Clock.After(maxShutdownWait):
	}
	msg := "timeout waiting for lease manager shutdown"
	dumpFile, err := manager.dumpDebug()
	logger := manager.config.Logger
	if err == nil {
		logger.Warningf("%v\ndebug info written to %v", msg, dumpFile)
	} else {
		logger.Warningf("%v\nerror writing debug info: %v", msg, err)
	}

}

func (manager *Manager) dumpDebug() (string, error) {
	dumpFile, err := os.OpenFile(filepath.Join(manager.config.LogDir, "lease-manager-debug.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() { _ = dumpFile.Close() }()

	claims := atomic.LoadInt64(&manager.outstandingClaims)
	revokes := atomic.LoadInt64(&manager.outstandingRevokes)
	template := `
lease manager state dump %v
entity-uuid: %v
outstanding-claims: %v
outstanding-revokes: %v

`[1:]
	message := fmt.Sprintf(template,
		time.Now().Format(time.RFC3339),
		manager.config.EntityUUID,
		claims,
		revokes,
	)
	if _, err = io.WriteString(dumpFile, message); err != nil {
		return "", errors.Annotate(err, "writing state to debug log file")
	}
	// Including the goroutines because the httpserver won't dump them
	// anymore if this worker stops happily.
	return dumpFile.Name(), pprof.Lookup("goroutine").WriteTo(dumpFile, 1)
}

func isFatalRetryError(err error) bool {
	switch {
	case txn.IsErrRetryable(err):
		return false
	case lease.IsTimeout(err):
		return false
	case lease.IsInvalid(err):
		return false
	}
	return true
}

type action string

const (
	claimAction  action = "claim"
	extendAction action = "extend"
)

func (a action) String() string {
	switch a {
	case claimAction:
		return "claiming"
	case extendAction:
		return "extending"
	}
	return "unknown"
}

func isFatalClaimRetryError(act action, err error, count int) bool {
	switch {
	case txn.IsErrRetryable(err):
		return false
	case lease.IsTimeout(err):
		return false
	case lease.IsInvalid(err):
		return false
	}
	return true
}
