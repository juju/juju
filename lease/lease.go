// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

const (
	// There are no blocking calls, so this can be long. We just don't
	// want goroutines to hang around indefinitely, so notifications
	// will time out after this value.
	notificationTimeout = 1 * time.Minute

	// This is a useful thing to know in several contexts.
	maxDuration = time.Duration(1<<63 - 1)
)

var (
	singleton           *leaseManager
	LeaseClaimDeniedErr = errors.New("lease claim denied")
	NotLeaseOwnerErr    = errors.Unauthorizedf("caller did not own lease for namespace")
	logger              = loggo.GetLogger("juju.lease")
)

func init() {
	singleton = &leaseManager{
		claimLease:       make(chan Token),
		releaseLease:     make(chan releaseLeaseMsg),
		leaseReleasedSub: make(chan leaseReleasedMsg),
		copyOfTokens:     make(chan []Token),
	}
}

type leasePersistor interface {
	WriteToken(string, Token) error
	RemoveToken(id string) error
	PersistedTokens() ([]Token, error)
}

// WorkerLoop returns a function which can be utilized within a
// worker.
func WorkerLoop(persistor leasePersistor) func(<-chan struct{}) error {
	singleton.leasePersistor = persistor
	return singleton.workerLoop
}

// Token represents a lease claim.
type Token struct {
	Namespace, Id string
	Expiration    time.Time
}

// Manager returns a manager.
func Manager() *leaseManager {
	// Guaranteed to be initialized because the init function runs
	// first.
	return singleton
}

//
// Messages for channels.
//

type releaseLeaseMsg struct {
	Token *Token
	Err   error
}
type leaseReleasedMsg struct {
	Watcher      chan<- struct{}
	ForNamespace string
}

type leaseManager struct {
	leasePersistor   leasePersistor
	claimLease       chan Token
	releaseLease     chan releaseLeaseMsg
	leaseReleasedSub chan leaseReleasedMsg
	copyOfTokens     chan []Token
}

// CopyOfLeaseTokens returns a copy of the lease tokens current held
// by the manager.
func (m *leaseManager) CopyOfLeaseTokens() []Token {
	m.copyOfTokens <- nil
	return <-m.copyOfTokens
}

// Claimlease claims a lease for the given duration for the given
// namespace and id. If the lease is already owned, a
// LeaseClaimDeniedErr will be returned. Either way the current lease
// owner's ID will be returned.
func (m *leaseManager) ClaimLease(namespace, id string, forDur time.Duration) (leaseOwnerId string, err error) {

	token := Token{namespace, id, time.Now().Add(forDur)}
	m.claimLease <- token
	activeClaim := <-m.claimLease

	leaseOwnerId = activeClaim.Id
	if id != leaseOwnerId {
		err = LeaseClaimDeniedErr
	}

	return leaseOwnerId, err
}

// ReleaseLease releases the lease held for namespace by id.
func (m *leaseManager) ReleaseLease(namespace, id string) (err error) {

	token := Token{Namespace: namespace, Id: id}
	m.releaseLease <- releaseLeaseMsg{Token: &token}
	response := <-m.releaseLease

	if err := response.Err; err != nil {
		err = errors.Annotatef(response.Err, `could not release lease for namespace "%s", id "%s"`, namespace, id)

		// Log errors so that we're aware they're happening, but don't
		// burden the caller with dealing with an error if it's
		// essential a no-op.
		if errors.IsUnauthorized(err) {
			logger.Warningf(err.Error())
			return nil
		}
		return err
	}

	return nil
}

// LeaseReleasedNotifier returns a channel a caller can block on to be
// notified of when a lease is released for namespace. This channel is
// reusable, but will be closed if it does not respond within
// "notificationTimeout".
func (m *leaseManager) LeaseReleasedNotifier(namespace string) (notifier <-chan struct{}) {
	watcher := make(chan struct{})
	m.leaseReleasedSub <- leaseReleasedMsg{watcher, namespace}

	return watcher
}

// workerLoop serializes all requests into a single thread.
func (m *leaseManager) workerLoop(stop <-chan struct{}) error {

	// These data-structures are local to ensure they're only utilized
	// within this thread-safe context.

	releaseSubs := make(map[string][]chan<- struct{}, 0)

	// Pull everything off our data-store & check for expirations.
	leaseCache, err := populateTokenCache(m.leasePersistor)
	if err != nil {
		return err
	}
	nextExpiration := m.expireLeases(leaseCache, releaseSubs)

	for {
		select {
		case <-stop:
			return nil
		case claim := <-m.claimLease:
			lease := claimLease(leaseCache, claim)
			if lease.Id != claim.Id {
				m.claimLease <- lease
			}

			m.leasePersistor.WriteToken(lease.Namespace, lease)
			if lease.Expiration.Before(nextExpiration) {
				nextExpiration = lease.Expiration
			}
			m.claimLease <- lease
		case claim := <-m.releaseLease:
			var response releaseLeaseMsg
			response.Err = releaseLease(leaseCache, claim.Token)
			if response.Err != nil {
				m.releaseLease <- response
			}

			// Unwind our layers from most volatile to least.
			response.Err = m.leasePersistor.RemoveToken(claim.Token.Namespace)
			m.releaseLease <- response
			notifyOfRelease(releaseSubs[claim.Token.Namespace], claim.Token.Namespace)

		case subscription := <-m.leaseReleasedSub:
			subscribe(releaseSubs, subscription)
		case <-m.copyOfTokens:
			// create a copy of the lease cache for use by code
			// external to our thread-safe context.
			m.copyOfTokens <- copyTokens(leaseCache)
		case <-time.After(nextExpiration.Sub(time.Now())):
			nextExpiration = m.expireLeases(leaseCache, releaseSubs)
			break
		}
	}
}

func (m *leaseManager) expireLeases(
	cache map[string]Token,
	subscribers map[string][]chan<- struct{},
) time.Time {

	// Having just looped through all the leases we're holding, we can
	// inform the caller of when the next expiration will occur.
	nextExpiration := time.Now().Add(maxDuration)

	for _, token := range cache {

		if token.Expiration.After(time.Now()) {
			// For the tokens that aren't expiring yet, find the
			// minimum time we should wait before cleaning up again.
			if nextExpiration.After(token.Expiration) {
				nextExpiration = token.Expiration
				fmt.Printf("Setting next expiration to %s\n", nextExpiration)
			}
			continue
		}

		logger.Infof(`Lease for namespace "%s" has expired.`, token.Namespace)
		if err := releaseLease(cache, &token); err == nil {
			notifyOfRelease(subscribers[token.Namespace], token.Namespace)
		}
	}

	return nextExpiration
}

func copyTokens(cache map[string]Token) (copy []Token) {
	for _, t := range cache {
		copy = append(copy, t)
	}
	return copy
}

func claimLease(cache map[string]Token, claim Token) Token {
	if active, ok := cache[claim.Namespace]; ok && active.Id != claim.Id {
		return active
	}
	cache[claim.Namespace] = claim
	logger.Infof(`"%s" obtained lease for "%s"`, claim.Id, claim.Namespace)
	return claim
}

func releaseLease(cache map[string]Token, claim *Token) error {
	if active, ok := cache[claim.Namespace]; !ok || active.Id != claim.Id {
		return NotLeaseOwnerErr
	}
	delete(cache, claim.Namespace)
	logger.Infof(`"%s" released lease for namespace "%s"`, claim.Id, claim.Namespace)
	return nil
}

func subscribe(subMap map[string][]chan<- struct{}, subscription leaseReleasedMsg) {
	subList := subMap[subscription.ForNamespace]
	subList = append(subList, subscription.Watcher)
	subMap[subscription.ForNamespace] = subList
}

func notifyOfRelease(subscribers []chan<- struct{}, namespace string) {
	logger.Infof(`Notifying namespace "%s" subscribers that its lease has been released.`, namespace)
	for _, subscriber := range subscribers {
		// Spin off into go-routine so we don't rely on listeners to
		// not block.
		go func(subscriber chan<- struct{}) {
			select {
			case subscriber <- struct{}{}:
			case <-time.After(notificationTimeout):
				// TODO(kate): Remove this bad-citizen from the
				// notifier's list.
				logger.Warningf("A notification timed out after %s.", notificationTimeout)
			}
		}(subscriber)
	}
}

func populateTokenCache(persistor leasePersistor) (map[string]Token, error) {

	tokens, err := persistor.PersistedTokens()
	if err != nil {
		return nil, err
	}

	cache := make(map[string]Token)
	for _, tok := range tokens {
		cache[tok.Namespace] = tok
	}

	return cache, nil
}
