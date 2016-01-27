// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"launchpad.net/tomb"

	"github.com/juju/juju/leadership"
	"github.com/juju/juju/state/lease"
)

var logger = loggo.GetLogger("juju.state.leadership")

// NewManager returns a Manager implementation, backed by a lease.Client,
// which (in addition to its exposed Manager capabilities) will expire all
// known leases as they run out. The caller takes responsibility for killing,
// and handling errors from, the returned Worker.
func NewManager(config ManagerConfig) (ManagerWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	manager := &manager{
		config: config,
		claims: make(chan claim),
		checks: make(chan check),
		blocks: make(chan block),
	}
	go func() {
		defer manager.tomb.Done()
		// note: we don't directly tomb.Kill, because we may need to
		// unwrap tomb.ErrDying in order to function correctly.
		manager.kill(manager.loop())
	}()
	return manager, nil
}

// manager implements ManagerWorker.
type manager struct {
	tomb tomb.Tomb

	// config collects all external configuration and dependencies.
	config ManagerConfig

	// claims is used to deliver leadership claim requests to the loop.
	claims chan claim

	// checks is used to deliver leadership check requests to the loop.
	checks chan check

	// blocks is used to deliver leaderlessness block requests to the loop.
	blocks chan block
}

// Kill is part of the worker.Worker interface.
func (manager *manager) Kill() {
	manager.kill(nil)
}

// kill unwraps tomb.ErrDying before killing the tomb, thus allowing the worker
// to use errors.Trace liberally and still stop cleanly.
func (manager *manager) kill(err error) {
	if errors.Cause(err) == tomb.ErrDying {
		err = tomb.ErrDying
	} else if err != nil {
		logger.Errorf("stopping leadership manager with error: %v", err)
	}
	manager.tomb.Kill(err)
}

// Wait is part of the worker.Worker interface.
func (manager *manager) Wait() error {
	return manager.tomb.Wait()
}

// loop runs until the manager is stopped.
func (manager *manager) loop() error {
	blocks := make(blocks)
	for {
		if err := manager.choose(blocks); err != nil {
			return errors.Trace(err)
		}

		leases := manager.config.Client.Leases()
		for serviceName := range blocks {
			if _, found := leases[serviceName]; !found {
				blocks.unblock(serviceName)
			}
		}
	}
}

// choose breaks the select out of loop to make the blocking logic clearer.
func (manager *manager) choose(blocks blocks) error {
	select {
	case <-manager.tomb.Dying():
		return tomb.ErrDying
	case <-manager.nextTick():
		return manager.tick()
	case claim := <-manager.claims:
		return manager.handleClaim(claim)
	case check := <-manager.checks:
		return manager.handleCheck(check)
	case block := <-manager.blocks:
		blocks.add(block)
		return nil
	}
}

// ClaimLeadership is part of the leadership.Claimer interface.
func (manager *manager) ClaimLeadership(serviceName, unitName string, duration time.Duration) error {
	return claim{
		serviceName: serviceName,
		unitName:    unitName,
		duration:    duration,
		response:    make(chan bool),
		abort:       manager.tomb.Dying(),
	}.invoke(manager.claims)
}

// handleClaim processes and responds to the supplied claim. It will only return
// unrecoverable errors; mere failure to claim just indicates a bad request, and
// is communicated back to the claim's originator.
func (manager *manager) handleClaim(claim claim) error {
	client := manager.config.Client
	request := lease.Request{claim.unitName, claim.duration}
	err := lease.ErrInvalid
	for err == lease.ErrInvalid {
		select {
		case <-manager.tomb.Dying():
			return tomb.ErrDying
		default:
			info, found := client.Leases()[claim.serviceName]
			switch {
			case !found:
				err = client.ClaimLease(claim.serviceName, request)
			case info.Holder == claim.unitName:
				err = client.ExtendLease(claim.serviceName, request)
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

// LeadershipCheck is part of the leadership.Checker interface.
//
// The token returned will accept a `*[]txn.Op` passed to Check, and will
// populate it with transaction operations that will fail if the unit is
// not leader of the service.
func (manager *manager) LeadershipCheck(serviceName, unitName string) leadership.Token {
	return token{
		serviceName: serviceName,
		unitName:    unitName,
		checks:      manager.checks,
		abort:       manager.tomb.Dying(),
	}
}

// handleCheck processes and responds to the supplied check. It will only return
// unrecoverable errors; mere untruth of the assertion just indicates a bad
// request, and is communicated back to the check's originator.
func (manager *manager) handleCheck(check check) error {
	client := manager.config.Client
	info, found := client.Leases()[check.serviceName]
	if !found || info.Holder != check.unitName {
		if err := client.Refresh(); err != nil {
			return errors.Trace(err)
		}
		info, found = client.Leases()[check.serviceName]
	}
	if found && info.Holder == check.unitName {
		check.succeed(info.AssertOp)
	} else {
		check.fail()
	}
	return nil
}

// BlockUntilLeadershipReleased is part of the leadership.Claimer interface.
func (manager *manager) BlockUntilLeadershipReleased(serviceName string) error {
	return block{
		serviceName: serviceName,
		unblock:     make(chan struct{}),
		abort:       manager.tomb.Dying(),
	}.invoke(manager.blocks)
}

// nextTick returns a channel that will send a value at some point when
// we expect to have to do some work; either because at least one lease
// may be ready to expire, or because enough enough time has passed that
// it's worth checking for stalled collaborators.
func (manager *manager) nextTick() <-chan time.Time {
	now := manager.config.Clock.Now()
	nextTick := now.Add(manager.config.MaxSleep)
	for _, info := range manager.config.Client.Leases() {
		if info.Expiry.After(nextTick) {
			continue
		}
		nextTick = info.Expiry
	}
	logger.Debugf("waking to check leases at %s", nextTick)
	return clock.Alarm(manager.config.Clock, nextTick)
}

// tick snapshots recent leases and expires any that it can. There
// might be none that need attention; or those that do might already
// have been extended or expired by someone else; so ErrInvalid is
// expected, and ignored, comfortable that the client will have been
// updated in the background; and that we'll see fresh info when we
// subsequently check nextWake().
//
// It will return only unrecoverable errors.
func (manager *manager) tick() error {
	logger.Tracef("refreshing leases...")
	client := manager.config.Client
	if err := client.Refresh(); err != nil {
		return errors.Trace(err)
	}
	leases := client.Leases()

	// Sort lease names so we expire in a predictable order for the tests.
	names := make([]string, 0, len(leases))
	for name := range leases {
		names = append(names, name)
	}
	sort.Strings(names)

	logger.Tracef("expiring leases...")
	now := manager.config.Clock.Now()
	for _, name := range names {
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
