// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
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

	// TODO(Wallyworld) - a stop-gap error until we refactor using a tomb.
	LeaseManagerErr = errors.New("lease manager restarted due to error")

	logger = loggo.GetLogger("juju.lease")
)

func init() {
	singleton = &leaseManager{
		claimLease:       make(chan claimLeaseMsg),
		releaseLease:     make(chan releaseLeaseMsg),
		leaseReleasedSub: make(chan leaseReleasedMsg),
		copyOfTokens:     make(chan copyTokensMsg),
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

type claimLeaseResponse struct {
	token Token
	err   error
}

type claimLeaseMsg struct {
	Token    Token
	Response chan<- claimLeaseResponse
}
type releaseLeaseMsg struct {
	Token    Token
	Response chan<- error
}
type leaseReleasedMsg struct {
	Watcher      chan<- struct{}
	ForNamespace string
}
type copyTokensMsg struct {
	Response chan<- []Token
}

type leaseManager struct {
	leasePersistor   leasePersistor
	retrieveLease    chan Token
	claimLease       chan claimLeaseMsg
	releaseLease     chan releaseLeaseMsg
	leaseReleasedSub chan leaseReleasedMsg
	copyOfTokens     chan copyTokensMsg
}

// CopyOfLeaseTokens returns a copy of the lease tokens currently held
// by the manager.
func (m *leaseManager) CopyOfLeaseTokens() []Token {
	ch := make(chan []Token)
	m.copyOfTokens <- copyTokensMsg{ch}
	return <-ch
}

// RetrieveLease returns the lease token currently stored for the
// given namespace.
func (m *leaseManager) RetrieveLease(namespace string) Token {
	for _, tok := range m.CopyOfLeaseTokens() {
		if tok.Namespace != namespace {
			continue
		}
		return tok
	}
	return Token{}
}

// Claimlease claims a lease for the given duration for the given
// namespace and id. If the lease is already owned, a
// LeaseClaimDeniedErr will be returned. Either way the current lease
// owner's ID will be returned.
func (m *leaseManager) ClaimLease(namespace, id string, forDur time.Duration) (leaseOwnerId string, err error) {

	ch := make(chan claimLeaseResponse)
	token := Token{namespace, id, time.Now().Add(forDur)}
	message := claimLeaseMsg{token, ch}
	m.claimLease <- message
	claimResponse := <-ch
	if claimResponse.err != nil {
		return "", claimResponse.err
	}

	leaseOwnerId = claimResponse.token.Id
	if id != leaseOwnerId {
		err = LeaseClaimDeniedErr
	}

	return leaseOwnerId, err
}

// ReleaseLease releases the lease held for namespace by id.
func (m *leaseManager) ReleaseLease(namespace, id string) (err error) {

	ch := make(chan error)
	token := Token{Namespace: namespace, Id: id}
	message := releaseLeaseMsg{token, ch}
	m.releaseLease <- message
	err = <-ch

	if err != nil {
		err = errors.Annotatef(err, `could not release lease for namespace %q, id %q`, namespace, id)

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
			lease := claimLease(leaseCache, claim.Token)
			if lease.Id == claim.Token.Id {
				if err := m.leasePersistor.WriteToken(lease.Namespace, lease); err != nil {
					claim.Response <- claimLeaseResponse{err: LeaseManagerErr}
					return err
				}
				if lease.Expiration.Before(nextExpiration) {
					nextExpiration = lease.Expiration
				}
			}
			claim.Response <- claimLeaseResponse{lease, err}
		case release := <-m.releaseLease:
			// Unwind our layers from most volatile to least.
			err := releaseLease(leaseCache, release.Token)
			if err == nil {
				namespace := release.Token.Namespace
				if err := m.leasePersistor.RemoveToken(namespace); err != nil {
					release.Response <- LeaseManagerErr
					return err
				}
				notifyOfRelease(releaseSubs[namespace], namespace)
			}
			release.Response <- err
		case subscription := <-m.leaseReleasedSub:
			subscribe(releaseSubs, subscription)
		case msg := <-m.copyOfTokens:
			// create a copy of the lease cache for use by code
			// external to our thread-safe context.
			msg.Response <- copyTokens(leaseCache)
		case <-time.After(nextExpiration.Sub(time.Now())):
			nextExpiration = m.expireLeases(leaseCache, releaseSubs)
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
				logger.Debugf("Setting next expiration to %s", nextExpiration)
			}
			continue
		}

		logger.Infof(`Lease for namespace %q has expired.`, token.Namespace)
		if err := releaseLease(cache, token); err != nil {
			// TODO(fwereade): we should certainly be returning the error and
			// killing the main loop.
			logger.Errorf("Failed to release expired lease for namespace %q: %v", token.Namespace, err)
		} else {
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
	logger.Infof(`%q obtained lease for %q`, claim.Id, claim.Namespace)
	return claim
}

func releaseLease(cache map[string]Token, claim Token) error {
	if active, ok := cache[claim.Namespace]; !ok || active.Id != claim.Id {
		return NotLeaseOwnerErr
	}
	delete(cache, claim.Namespace)
	logger.Infof(`%q released lease for namespace %q`, claim.Id, claim.Namespace)
	return nil
}

func subscribe(subMap map[string][]chan<- struct{}, subscription leaseReleasedMsg) {
	subList := subMap[subscription.ForNamespace]
	subList = append(subList, subscription.Watcher)
	subMap[subscription.ForNamespace] = subList
}

func notifyOfRelease(subscribers []chan<- struct{}, namespace string) {
	logger.Infof(`Notifying namespace %q subscribers that its lease has been released.`, namespace)
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
