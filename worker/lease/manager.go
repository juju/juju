// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1/catacomb"
	"gopkg.in/retry.v1"

	"github.com/juju/juju/core/lease"
)

const (
	// maxRetries gives the maximum number of attempts we'll try if
	// there are timeouts.
	maxRetries = 5

	// initialRetryDelay is the starting delay - this will be
	// increased exponentially up maxRetries.
	initialRetryDelay = 50 * time.Millisecond
)

// errStopped is returned to clients when an operation cannot complete because
// the manager has started (and possibly finished) shutdown.
var errStopped = errors.New("lease manager stopped")

type dummySecretary struct{}

func (d dummySecretary) CheckLease(key lease.Key) error             { return nil }
func (d dummySecretary) CheckHolder(name string) error              { return nil }
func (d dummySecretary) CheckDuration(duration time.Duration) error { return nil }

// NewDeadManager returns a manager that's already dead
// and always returns the given error.
func NewDeadManager(err error) *Manager {
	var secretary dummySecretary
	m := Manager{
		config: ManagerConfig{
			Secretary: func(_ string) (Secretary, error) {
				return secretary, nil
			},
		},
	}
	_ = catacomb.Invoke(catacomb.Plan{
		Site: &m.catacomb,
		Work: func() error {
			return errors.Trace(err)
		},
	})
	return &m
}

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
		checks:     make(chan check),
		blocks:     make(chan block),
		pins:       make(chan pin),
		unpins:     make(chan pin),
		errors:     make(chan error),
		logContext: logContext,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &manager.catacomb,
		Work: manager.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return manager, nil
}

// Manager implements worker.Worker and can be bound to get
// lease.Checkers and lease.Claimers.
type Manager struct {
	catacomb catacomb.Catacomb

	// config collects all external configuration and dependencies.
	config ManagerConfig

	// logContext is just a string that associates messages in the log
	// It is seeded with the first six characters of the config.EntityUUID
	// if supplied
	logContext string

	// nextTimeout is the next time that has a possibly expiry that we would care
	// about, capped at the maximum time.
	nextTimeout time.Time

	// timer tracks when nextTimeout would expire and triggers when it does
	timer clock.Timer

	// muNextTimeout protects accesses to nextTimeout
	muNextTimeout sync.Mutex

	// claims is used to deliver lease claim requests to the loop.
	claims chan claim

	// checks is used to deliver lease check requests to the loop.
	checks chan check

	// blocks is used to deliver expiry block requests to the loop.
	blocks chan block

	// pins is used to deliver lease pin requests to the loop.
	pins chan pin

	// unpins is used to deliver lease unpin requests to the loop.
	unpins chan pin

	// errors is used to send errors from background claim or expire
	// goroutines back to the main loop.
	errors chan error

	// wg is used to ensure that all child goroutines are finished
	// before we stop.
	wg sync.WaitGroup
}

// Kill is part of the worker.Worker interface.
func (manager *Manager) Kill() {
	manager.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (manager *Manager) Wait() error {
	return manager.catacomb.Wait()
}

// loop runs until the manager is stopped.
func (manager *Manager) loop() error {
	if collector, ok := manager.config.Store.(prometheus.Collector); ok && manager.config.PrometheusRegisterer != nil {
		// The store implements the collector interface, but the lease.Store
		// does not expose those.
		_ = manager.config.PrometheusRegisterer.Register(collector)
		defer manager.config.PrometheusRegisterer.Unregister(collector)
	}

	defer manager.wg.Wait()
	blocks := make(blocks)
	manager.setupInitialTimer()
	for {
		if err := manager.choose(blocks); err != nil {
			manager.config.Logger.Tracef("[%s] exiting main loop with error: %v", manager.logContext, err)
			return errors.Trace(err)
		}
	}
}

// choose breaks the select out of loop to make the blocking logic clearer.
func (manager *Manager) choose(blocks blocks) error {
	select {
	case <-manager.catacomb.Dying():
		return manager.catacomb.ErrDying()
	case err := <-manager.errors:
		return errors.Trace(err)
	case check := <-manager.checks:
		return manager.handleCheck(check)
	case now := <-manager.timer.Chan():
		manager.tick(now, blocks)
	case claim := <-manager.claims:
		manager.wg.Add(1)
		go manager.retryingClaim(claim)
	case pin := <-manager.pins:
		manager.handlePin(pin)
	case unpin := <-manager.unpins:
		manager.handleUnpin(unpin)
	case block := <-manager.blocks:
		// TODO(raftlease): Include the other key items.
		manager.config.Logger.Tracef("[%s] adding block for: %s", manager.logContext, block.leaseKey.Lease)
		// TODO(jam): 2019-02-04 If we are adding a block for a Lease, we need
		// to check if that lease is known to us.
		// This is a little bit odd, in that our Leases information might be
		// out of date, if this is a relatively new lease, which means Leases
		// hasn't seen it show up yet. It seems that *something* new about the
		// Lease enough to deny the claim, so that the client fell back to
		// ask for BlockUntilReleased. However, it is also possible for a
		// client to call Claim, have that rejected, and in the time between
		// being rejected and the caller coming back again with a BlockUntil,
		// the Lease will have expired, in which case they *should* be told that
		// they can immediately try to Claim again.
		// I guess since we don't guarantee that the lease is actually available
		// when we return from here, and that *will* be validated by Claim(), we
		// can probably just check
		// lease, exists := store.Leases(block.leaseKey)[block.leaseKey]
		// if !exists { block.unblock(block.leaseKey) }
		// needs testing
		blocks.add(block)
	}
	return nil
}

func (manager *Manager) bind(namespace, modelUUID string) (broker, error) {
	secretary, err := manager.config.Secretary(namespace)
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

// Pinner returns a lease.Pinner for the specified namespace and model.
func (manager *Manager) Pinner(namespace, modelUUID string) (lease.Pinner, error) {
	return manager.bind(namespace, modelUUID)
}

// retryingClaim handles timeouts when claiming, and responds to the
// claiming party when it eventually succeeds or fails, or if it times
// out after a number of retries.
func (manager *Manager) retryingClaim(claim claim) {
	defer manager.wg.Done()
	var (
		err     error
		success bool
	)
	for a := manager.startRetry(); a.Next(); {
		success, err = manager.handleClaim(claim)
		if !lease.IsTimeout(err) && !lease.IsInvalid(err) {
			break
		}
		if a.More() {
			if lease.IsInvalid(err) {
				manager.config.Logger.Tracef("[%s] request by %s for lease %s %v, retrying...",
					manager.logContext, claim.holderName, claim.leaseKey.Lease, err)
			} else {
				manager.config.Logger.Tracef("[%s] timed out handling claim by %s for lease %s, retrying...",
					manager.logContext, claim.holderName, claim.leaseKey.Lease)
			}
		}
	}

	if lease.IsTimeout(err) {
		claim.respond(lease.ErrTimeout)
		manager.config.Logger.Warningf("[%s] retrying timed out while handling claim", manager.logContext)
		return
	}
	if err == nil {
		if !success {
			claim.respond(lease.ErrClaimDenied)
			return
		}
		claim.respond(nil)
		// now, this isn't strictly true, as the lease can be given for longer
		// than the requested duration. However, it cannot be shorter.
		// Doing it this way, we'll wake up, and then see we can sleep
		// for a bit longer. But we'll always wake up in time.
		manager.ensureNextTimeout(claim.duration)
	} else {
		// Stop the main loop because we got an abnormal error
		err := errors.Trace(err)
		select {
		case <-manager.catacomb.Dying():
			return
		case manager.errors <- err:
		}
	}
}

// handleClaim processes the supplied claim. It will only return
// unrecoverable errors or timeouts; mere failure to claim just
// indicates a bad request, and is returned as (false, nil).
func (manager *Manager) handleClaim(claim claim) (bool, error) {
	store := manager.config.Store
	request := lease.Request{Holder: claim.holderName, Duration: claim.duration}
	err := lease.ErrInvalid
	action := "unknown"
	select {
	case <-manager.catacomb.Dying():
		return false, manager.catacomb.ErrDying()
	default:
		info, found := store.Leases(claim.leaseKey)[claim.leaseKey]
		switch {
		case !found:
			manager.config.Logger.Tracef("[%s] %s asked for lease %s, no lease found, claiming for %s",
				manager.logContext, claim.holderName, claim.leaseKey.Lease, claim.duration)
			action = "claiming"
			err = store.ClaimLease(claim.leaseKey, request)
		case info.Holder == claim.holderName:
			manager.config.Logger.Tracef("[%s] %s extending lease %s for %s",
				manager.logContext, claim.holderName, claim.leaseKey.Lease, claim.duration)
			action = "extending"
			err = store.ExtendLease(claim.leaseKey, request)
		default:
			// Note: (jam) 2017-10-31) We don't check here if the lease has
			// expired for the current holder. Should we?
			remaining := info.Expiry.Sub(manager.config.Clock.Now())
			manager.config.Logger.Tracef("[%s] %s asked for lease %s, held by %s for another %s, rejecting",
				manager.logContext, claim.holderName, claim.leaseKey.Lease, info.Holder, remaining)
			return false, nil
		}
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	manager.config.Logger.Tracef("[%s] %s %s lease %s for %s successful",
		manager.logContext, claim.holderName, action, claim.leaseKey.Lease, claim.duration)
	return true, nil
}

// handleCheck processes and responds to the supplied check. It will only return
// unrecoverable errors; mere untruth of the assertion just indicates a bad
// request, and is communicated back to the check's originator.
func (manager *Manager) handleCheck(check check) error {
	key := check.leaseKey
	store := manager.config.Store
	manager.config.Logger.Tracef("[%s] handling Check for lease %s on behalf of %s",
		manager.logContext, key.Lease, check.holderName)

	info := store.Leases(key)[key]
	if info.Holder != check.holderName {
		manager.config.Logger.Tracef("[%s] handling Check for lease %s on behalf of %s, not found, refreshing",
			manager.logContext, key.Lease, check.holderName)
		if err := store.Refresh(); err != nil {
			return errors.Trace(err)
		}
		info = store.Leases(key)[key]
	}

	var response error
	if info.Holder != check.holderName {
		manager.config.Logger.Tracef("[%s] handling Check for lease %s on behalf of %s, not held",
			manager.logContext, key.Lease, check.holderName)
		response = lease.ErrNotHeld
	} else if check.trapdoorKey != nil {
		response = info.Trapdoor(check.attempt, check.trapdoorKey)
	}
	check.respond(errors.Trace(response))
	return nil
}

// tick triggers when we think a lease might be expiring, so we check if there
// are leases to expire, and then unblock anything that is no longer blocked,
// and then compute the next time we should wake up.
func (manager *Manager) tick(now time.Time, blocks blocks) {
	manager.config.Logger.Tracef("[%s] tick at %v, running expiry checks\n", manager.logContext, now)
	// TODO(jam): 2019-02-03 This retryingTick is being fired off in its own
	//  goroutine without synchronizing with manager.nextTick above.
	//  Which means that if handling expiring tokens takes any real length
	//  of time, then we'll get a backlog of *many* goroutines all trying
	//  to expire tokens at some previous point in time.
	//  We might want to stop the ticking while retryingTick is happening,
	//  *or* tell retryingTick that it should update its time when a new
	//  nextTick occurs.
	// If we need to expire leases we should do it, otherwise this
	// is just an opportunity to check for blocks that need to be
	// notified.
	expired := make(chan struct{})
	if !manager.config.Store.Autoexpire() {
		manager.wg.Add(1)
		go manager.retryingExpire(now, expired)
	} else {
		close(expired)
	}
	// Wait for the goroutine to do at least one expiry pass. That way we
	// know that the leases map is reasonably up-to-date.
	t := manager.config.Clock.NewTimer(initialRetryDelay)
	select {
	case <-manager.catacomb.Dying():
		return
	case <-expired:
		t.Stop()
	case <-t.Chan():
		// Don't let a blocked expire attempt prevent our core loop from operating.
	}
	manager.config.Logger.Tracef("[%s] evaluating %d blocks", manager.logContext, len(blocks))
	leases := manager.config.Store.Leases()
	for leaseName := range blocks {
		if _, found := leases[leaseName]; !found {
			manager.config.Logger.Tracef("[%s] unblocking: %s", manager.logContext, leaseName)
			blocks.unblock(leaseName)
		}
	}
	manager.computeNextTimeout(now, leases)
}

// computeNextTimeout iterates the leases and finds out what the next time we
// want to wake up, expire any leases and then handle any unblocks that happen.
// It is based on the MaxSleep time, and on any expire that is going to expire
// after now but before MaxSleep.
// it's worth checking for stalled collaborators.
func (manager *Manager) computeNextTimeout(lastTick time.Time, leases map[lease.Key]lease.Info) {
	now := manager.config.Clock.Now()
	nextTick := now.Add(manager.config.MaxSleep)
	for _, info := range leases {
		if !info.Expiry.After(lastTick) {
			// The previous expire will expire this lease eventually, or
			// the manager will die with an error. Either way, we
			// don't need to worry about expiries in a previous expire
			// here.
			continue
		}
		if info.Expiry.After(nextTick) {
			continue
		}
		nextTick = info.Expiry
	}
	manager.config.Logger.Tracef("[%s] next expire decided on %v %v",
		manager.logContext, nextTick.Sub(now).Round(time.Millisecond), nextTick)
	manager.setNextTimeout(nextTick)
}

func (manager *Manager) setupInitialTimer() {
	// Create a timer at max timeout, we'll update after refreshing leases
	manager.muNextTimeout.Lock()
	manager.timer = manager.config.Clock.NewTimer(manager.config.MaxSleep)
	manager.muNextTimeout.Unlock()
	// lastTick has never happened, so pass in the epoch time
	manager.computeNextTimeout(time.Time{}, manager.config.Store.Leases())
}

func (manager *Manager) setNextTimeout(t time.Time) {
	manager.muNextTimeout.Lock()
	manager.nextTimeout = t
	d := t.Sub(manager.config.Clock.Now())
	manager.timer.Reset(d)
	manager.muNextTimeout.Unlock()
}

// ensureNextTimeout makes sure that the next timeout happens no-later than
// duration from now.
func (manager *Manager) ensureNextTimeout(d time.Duration) {
	manager.muNextTimeout.Lock()
	next := manager.nextTimeout
	proposed := manager.config.Clock.Now().Add(d)
	if next.After(proposed) {
		manager.config.Logger.Tracef("[%s] ensuring we wake up before %v at %v\n",
			manager.logContext, d, next)
		manager.nextTimeout = next
		manager.timer.Reset(d)
	}
	manager.muNextTimeout.Unlock()
}

// retryingExpire runs expire and retries any timeouts.
func (manager *Manager) retryingExpire(now time.Time, expired chan struct{}) {
	manager.config.Logger.Tracef("[%s] expire looking for leases to expire\n", manager.logContext)
	defer manager.wg.Done()
	var err error
	for a := manager.startRetry(); a.Next(); {
		err = manager.expire(now)
		// We've done at least 1 attempt
		if expired != nil {
			close(expired)
			expired = nil
		}
		if !lease.IsTimeout(err) {
			break
		}
		if a.More() {
			manager.config.Logger.Tracef("[%s] timed out during expire, retrying...", manager.logContext)
		}
	}
	// Don't bother sending an error if we're dying - this avoids a
	// race in the tests.
	select {
	case <-manager.catacomb.Dying():
		return
	default:
	}

	if lease.IsTimeout(err) {
		// We don't crash on timeouts to avoid bouncing the API server.
		manager.config.Logger.Warningf("[%s] retrying timed out in expire", manager.logContext)
		return
	}
	select {
	case <-manager.catacomb.Dying():
		return
	case manager.errors <- err:
		// We're done.
	}
}

// expire snapshots recent leases and expires any that it can. There
// might be none that need attention; or those that do might already
// have been extended or expired by someone else; so ErrInvalid is
// expected, and ignored, comfortable that the store will have been
// updated in the background; and that we'll see fresh info when we
// subsequently check nextWake().
//
// It will return only unrecoverable errors.
func (manager *Manager) expire(now time.Time) error {
	store := manager.config.Store
	if err := store.Refresh(); err != nil {
		return errors.Trace(err)
	}
	leases := store.Leases()

	// Sort lease keys so we expire in a predictable order for the tests.
	keys := make([]lease.Key, 0, len(leases))
	for key := range leases {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keysLess(keys[i], keys[j])
	})

	manager.config.Logger.Tracef("[%s] checking expiry on %d leases", manager.logContext, len(leases))
	expired := make([]lease.Key, 0)
	for _, key := range keys {
		if leases[key].Expiry.After(now) {
			continue
		}
		err := store.ExpireLease(key)
		if err != nil && !lease.IsInvalid(err) {
			return errors.Trace(err)
		}
		expired = append(expired, key)
	}
	if len(expired) == 0 {
		manager.config.Logger.Debugf("[%s] no leases to expire", manager.logContext)
	} else {
		names := make([]string, 0, len(expired))
		for _, expiredKey := range expired {
			names = append(names, expiredKey.Lease)
		}
		manager.config.Logger.Debugf(
			"[%s] expired %d leases: %s", manager.logContext, len(expired), strings.Join(names, ", "))
	}
	return nil
}

func (manager *Manager) startRetry() *retry.Attempt {
	return retry.StartWithCancel(
		retry.LimitCount(maxRetries, retry.Exponential{
			Initial: initialRetryDelay,
			Jitter:  true,
		}),
		manager.config.Clock,
		manager.catacomb.Dying(),
	)
}

func (manager *Manager) handlePin(p pin) {
	p.respond(errors.Trace(manager.config.Store.PinLease(p.leaseKey, p.entity)))
}

func (manager *Manager) handleUnpin(p pin) {
	p.respond(errors.Trace(manager.config.Store.UnpinLease(p.leaseKey, p.entity)))
}

// pinned returns lease names and the entities requiring their pinned
// behaviour, from the input namespace/model for which leases are pinned.
func (manager *Manager) pinned(namespace, modelUUID string) map[string][]string {
	pinned := make(map[string][]string)
	for key, entities := range manager.config.Store.Pinned() {
		if key.Namespace == namespace && key.ModelUUID == modelUUID {
			pinned[key.Lease] = entities
		}
	}
	return pinned
}

func keysLess(a, b lease.Key) bool {
	if a.Namespace == b.Namespace && a.ModelUUID == b.ModelUUID {
		return a.Lease < b.Lease
	}
	if a.Namespace == b.Namespace {
		return a.ModelUUID < b.ModelUUID
	}
	return a.Namespace < b.Namespace
}
