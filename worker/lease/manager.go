// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"sort"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
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

var logger = loggo.GetLogger("juju.worker.lease")

// errStopped is returned to clients when an operation cannot complete because
// the manager has started (and possibly finished) shutdown.
var errStopped = errors.New("lease manager stopped")

type dummySecretary struct{}

func (d dummySecretary) CheckLease(name string) error               { return nil }
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
	catacomb.Invoke(catacomb.Plan{
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

	// now is the time of the current tick - expiries are done on the
	// basis of this time.
	now time.Time

	// claims is used to deliver lease claim requests to the loop.
	claims chan claim

	// checks is used to deliver lease check requests to the loop.
	checks chan check

	// blocks is used to deliver expiry block requests to the loop.
	blocks chan block

	// errors is used to send errors from background claim or tick
	// goroutines back to the main loop.
	errors chan error
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
	blocks := make(blocks)
	for {
		if err := manager.choose(blocks); err != nil {
			return errors.Trace(err)
		}

		leases := manager.config.Store.Leases()
		for leaseName := range blocks {
			if _, found := leases[leaseName]; !found {
				logger.Tracef("[%s] unblocking: %s", manager.logContext, leaseName)
				blocks.unblock(leaseName)
			}
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
	case manager.now = <-manager.nextTick(manager.now):
		go manager.retryingTick(manager.now)
	case claim := <-manager.claims:
		go manager.retryingClaim(claim)
	case block := <-manager.blocks:
		// TODO(raftlease): Include the other key items.
		logger.Tracef("[%s] adding block for: %s", manager.logContext, block.leaseKey.Lease)
		blocks.add(block)
	}
	return nil
}

func (manager *Manager) bind(namespace, modelUUID string) (checkerClaimer, error) {
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

// retryingClaim handles timeouts when claiming, and responds to the
// claiming party when it eventually succeeds or fails, or if it times
// out after a number of retries.
func (manager *Manager) retryingClaim(claim claim) {
	var (
		err     error
		success bool
	)
	for a := manager.startRetry(); a.Next(); {
		success, err = manager.handleClaim(claim)
		if errors.Cause(err) != lease.ErrTimeout {
			break
		}
		if a.More() {
			logger.Tracef("[%s] timed out handling claim, retrying...", manager.logContext)
		}
	}

	if success {
		claim.respond(nil)
	} else if errors.Cause(err) == lease.ErrTimeout {
		claim.respond(lease.ErrTimeout)
	} else if err == nil {
		claim.respond(lease.ErrClaimDenied)
	}
	// Otherwise we allow the fatal error to send errStopped.

	select {
	case <-manager.catacomb.Dying():
		return
	default:
	}
	manager.errors <- err
}

// handleClaim processes the supplied claim. It will only return
// unrecoverable errors or timeouts; mere failure to claim just
// indicates a bad request, and is returned as (false, nil).
func (manager *Manager) handleClaim(claim claim) (bool, error) {
	store := manager.config.Store
	request := lease.Request{claim.holderName, claim.duration}
	err := lease.ErrInvalid
	for err == lease.ErrInvalid {
		select {
		case <-manager.catacomb.Dying():
			return false, manager.catacomb.ErrDying()
		default:
			// TODO(jam) 2017-10-31: We are asking for all leases just to look
			// up one of them. Shouldn't the store.Leases() interface allow us
			// to just query for a single entry?
			info, found := store.Leases()[claim.leaseKey]
			switch {
			case !found:
				logger.Tracef("[%s] %s asked for lease %s, no lease found, claiming for %s", manager.logContext, claim.holderName, claim.leaseKey.Lease, claim.duration)
				err = store.ClaimLease(claim.leaseKey, request)
			case info.Holder == claim.holderName:
				logger.Tracef("[%s] %s extending lease %s for %s", manager.logContext, claim.holderName, claim.leaseKey.Lease, claim.duration)
				err = store.ExtendLease(claim.leaseKey, request)
			default:
				// Note: (jam) 2017-10-31) We don't check here if the lease has
				// expired for the current holder. Should we?
				remaining := info.Expiry.Sub(manager.config.Clock.Now())
				logger.Tracef("[%s] %s asked for lease %s, held by %s for another %s, rejecting",
					manager.logContext, claim.holderName, claim.leaseKey.Lease, info.Holder, remaining)
				return false, nil
			}
		}
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

// handleCheck processes and responds to the supplied check. It will only return
// unrecoverable errors; mere untruth of the assertion just indicates a bad
// request, and is communicated back to the check's originator.
func (manager *Manager) handleCheck(check check) error {
	store := manager.config.Store
	logger.Tracef("[%s] handling Check for lease %s on behalf of %s", manager.logContext, check.leaseKey.Lease, check.holderName)
	info, found := store.Leases()[check.leaseKey]
	if !found || info.Holder != check.holderName {
		logger.Tracef("[%s] handling Check for lease %s on behalf of %s, not found, refreshing", manager.logContext, check.leaseKey.Lease, check.holderName)
		if err := store.Refresh(); err != nil {
			return errors.Trace(err)
		}
		info, found = store.Leases()[check.leaseKey]
	}

	var response error
	if !found || info.Holder != check.holderName {
		logger.Tracef("[%s] handling Check for lease %s on behalf of %s, not held", manager.logContext, check.leaseKey.Lease, check.holderName)
		response = lease.ErrNotHeld
	} else if check.trapdoorKey != nil {
		response = info.Trapdoor(check.trapdoorKey)
	}
	check.respond(errors.Trace(response))
	return nil
}

// nextTick returns a channel that will send a value at some point when
// we expect to have to do some work; either because at least one lease
// may be ready to expire, or because enough enough time has passed that
// it's worth checking for stalled collaborators.
func (manager *Manager) nextTick(lastTick time.Time) <-chan time.Time {
	now := manager.config.Clock.Now()
	nextTick := now.Add(manager.config.MaxSleep)
	leases := manager.config.Store.Leases()
	for _, info := range leases {
		if !info.Expiry.After(lastTick) {
			// The previous tick will expire this lease eventually, or
			// the manager will die with an error. Either way, we
			// don't need to worry about expiries in a previous tick
			// here.
			continue
		}
		if info.Expiry.After(nextTick) {
			continue
		}
		nextTick = info.Expiry
	}
	return clock.Alarm(manager.config.Clock, nextTick)
}

// retryingTick runs tick and retries any timeouts.
func (manager *Manager) retryingTick(now time.Time) {
	var err error
	for a := manager.startRetry(); a.Next(); {
		err = manager.tick(now)
		if errors.Cause(err) != lease.ErrTimeout {
			break
		}
		if a.More() {
			logger.Tracef("[%s] timed out during tick, retrying...", manager.logContext)
		}
	}
	// Don't bother sending an error if we're dying - this avoids a
	// race in the tests.
	select {
	case <-manager.catacomb.Dying():
		return
	default:
	}
	manager.errors <- err
}

// tick snapshots recent leases and expires any that it can. There
// might be none that need attention; or those that do might already
// have been extended or expired by someone else; so ErrInvalid is
// expected, and ignored, comfortable that the store will have been
// updated in the background; and that we'll see fresh info when we
// subsequently check nextWake().
//
// It will return only unrecoverable errors.
func (manager *Manager) tick(now time.Time) error {
	logger.Tracef("[%s] waking up to refresh and expire leases", manager.logContext)
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

	logger.Tracef("[%s] checking expiry on %d leases", manager.logContext, len(leases))
	expired := make([]lease.Key, 0)
	for _, key := range keys {
		if leases[key].Expiry.After(now) {
			continue
		}
		switch err := store.ExpireLease(key); err {
		case nil, lease.ErrInvalid:
		default:
			return errors.Trace(err)
		}
		expired = append(expired, key)
	}
	if len(expired) == 0 {
		logger.Debugf("[%s] no leases to expire", manager.logContext)
	} else {
		names := make([]string, 0, len(expired))
		for _, expiredKey := range expired {
			names = append(names, expiredKey.Lease)
		}
		logger.Debugf("[%s] expired %d leases: %s", manager.logContext, len(expired), strings.Join(names, ", "))
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

func keysLess(a, b lease.Key) bool {
	if a.Namespace == b.Namespace && a.ModelUUID == b.ModelUUID {
		return a.Lease < b.Lease
	}
	if a.Namespace == b.Namespace {
		return a.ModelUUID < b.ModelUUID
	}
	return a.Namespace < b.Namespace
}
