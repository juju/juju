// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.lease")

// errStopped is returned to clients when an operation cannot complete because
// the manager has started (and possibly finished) shutdown.
var errStopped = errors.New("lease manager stopped")

// NewManager returns a new *Manager configured as supplied. The caller takes
// responsibility for killing, and handling errors from, the returned Worker.
func NewManager(config ManagerConfig) (*Manager, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	manager := &Manager{
		config: config,
		claims: make(chan claim),
		checks: make(chan check),
		blocks: make(chan block),
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

// Manager implements lease.Claimer, lease.Checker, and worker.Worker.
type Manager struct {
	catacomb catacomb.Catacomb

	// config collects all external configuration and dependencies.
	config ManagerConfig

	// claims is used to deliver lease claim requests to the loop.
	claims chan claim

	// checks is used to deliver lease check requests to the loop.
	checks chan check

	// blocks is used to deliver expiry block requests to the loop.
	blocks chan block
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

		leases := manager.config.Client.Leases()
		for leaseName := range blocks {
			if _, found := leases[leaseName]; !found {
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
	case <-manager.nextExpiry():
		return manager.expire()
	case claim := <-manager.claims:
		return manager.handleClaim(claim)
	case check := <-manager.checks:
		return manager.handleCheck(check)
	case block := <-manager.blocks:
		blocks.add(block)
		return nil
	}
}

// Claim is part of the lease.Claimer interface.
func (manager *Manager) Claim(leaseName, holderName string, duration time.Duration) error {
	if err := manager.config.Secretary.CheckLease(leaseName); err != nil {
		return errors.Annotatef(err, "cannot claim lease %q", leaseName)
	}
	if err := manager.config.Secretary.CheckHolder(holderName); err != nil {
		return errors.Annotatef(err, "cannot claim lease for holder %q", holderName)
	}
	if err := manager.config.Secretary.CheckDuration(duration); err != nil {
		return errors.Annotatef(err, "cannot claim lease for %s", duration)
	}
	return claim{
		leaseName:  leaseName,
		holderName: holderName,
		duration:   duration,
		response:   make(chan bool),
		abort:      manager.catacomb.Dying(),
	}.invoke(manager.claims)
}

// handleClaim processes and responds to the supplied claim. It will only return
// unrecoverable errors; mere failure to claim just indicates a bad request, and
// is communicated back to the claim's originator.
func (manager *Manager) handleClaim(claim claim) error {
	client := manager.config.Client
	request := lease.Request{claim.holderName, claim.duration}
	err := lease.ErrInvalid
	for err == lease.ErrInvalid {
		select {
		case <-manager.catacomb.Dying():
			return manager.catacomb.ErrDying()
		default:
			info, found := client.Leases()[claim.leaseName]
			switch {
			case !found:
				err = client.ClaimLease(claim.leaseName, request)
			case info.Holder == claim.holderName:
				err = client.ExtendLease(claim.leaseName, request)
			default:
				claim.respond(false)
				return nil
			}
		}
	}
	if err != nil {
		return errors.Trace(err)
	}
	claim.respond(true)
	return nil
}

// Token is part of the lease.Checker interface.
func (manager *Manager) Token(leaseName, holderName string) lease.Token {
	return token{
		leaseName:  leaseName,
		holderName: holderName,
		secretary:  manager.config.Secretary,
		checks:     manager.checks,
		abort:      manager.catacomb.Dying(),
	}
}

// handleCheck processes and responds to the supplied check. It will only return
// unrecoverable errors; mere untruth of the assertion just indicates a bad
// request, and is communicated back to the check's originator.
func (manager *Manager) handleCheck(check check) error {
	client := manager.config.Client
	info, found := client.Leases()[check.leaseName]
	if !found || info.Holder != check.holderName {
		if err := client.Refresh(); err != nil {
			return errors.Trace(err)
		}
		info, found = client.Leases()[check.leaseName]
	}

	var response error
	if !found || info.Holder != check.holderName {
		response = lease.ErrNotHeld
	} else if check.trapdoorKey != nil {
		response = info.Trapdoor(check.trapdoorKey)
	}
	check.respond(errors.Trace(response))
	return nil
}

// WaitUntilExpired is part of the lease.Claimer interface.
func (manager *Manager) WaitUntilExpired(leaseName string) error {
	if err := manager.config.Secretary.CheckLease(leaseName); err != nil {
		return errors.Annotatef(err, "cannot wait for lease %q expiry", leaseName)
	}
	return block{
		leaseName: leaseName,
		unblock:   make(chan struct{}),
		abort:     manager.catacomb.Dying(),
	}.invoke(manager.blocks)
}

// nextExpiry returns a channel that will send a value at some point when we
// expect at least one lease to be ready to expire. If no leases are known,
// it will return nil.
func (manager *Manager) nextExpiry() <-chan time.Time {
	var nextExpiry time.Time
	for _, info := range manager.config.Client.Leases() {
		if !nextExpiry.IsZero() {
			if info.Expiry.After(nextExpiry) {
				continue
			}
		}
		nextExpiry = info.Expiry
	}
	if nextExpiry.IsZero() {
		logger.Tracef("no leases recorded; never waking for expiry")
		return nil
	}
	logger.Tracef("waking to expire leases at %s", nextExpiry)
	return clock.Alarm(manager.config.Clock, nextExpiry)
}

// expire will attempt to expire all leases that may have expired. There might
// be none; they might have been extended or expired already by someone else; so
// ErrInvalid is expected, and ignored, in the comfortable knowledge that the
// client will have been updated and we'll see fresh info when we scan for new
// expiries next time through the loop. It will return only unrecoverable errors.
func (manager *Manager) expire() error {
	logger.Tracef("expiring leases...")
	client := manager.config.Client
	leases := client.Leases()

	// Sort lease names so we expire in a predictable order for the tests.
	names := make([]string, 0, len(leases))
	for name := range leases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		now := manager.config.Clock.Now()
		if leases[name].Expiry.After(now) {
			continue
		}
		switch err := client.ExpireLease(name); err {
		case nil, lease.ErrInvalid:
		default:
			return errors.Trace(err)
		}
	}
	return nil
}
