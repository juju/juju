package lease

import (
	"errors"
	"sync"
	"time"
)

// ErrNoLeaseManager is the error returned by
// singletonLeaseManager methods if no lease
// manager is running.
var ErrNoLeaseManager = errors.New("no active lease manager")

// ErrLeaseManagerRunning is the error returned
// by NewLeaseManager if there is already one
// running.
var ErrLeaseManagerRunning = errors.New("lease manager already running")

var singleton singletonLeaseManager

type singletonLeaseManager struct {
	mu sync.Mutex
	m  *leaseManager
}

// Manager returns a singleton lease manager, which is active so long
// as there is a running leaseManager worker.
func Manager() *singletonLeaseManager {
	return &singleton
}

func (s *singletonLeaseManager) setLeaseManager(m *leaseManager) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m != nil && s.m != nil {
		return ErrLeaseManagerRunning
	}
	s.m = m
	return nil
}

func (s *singletonLeaseManager) getLeaseManager() (*leaseManager, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		return nil, ErrNoLeaseManager
	}
	return s.m, nil
}

func (s *singletonLeaseManager) ClaimLease(namespace, id string, forDur time.Duration) (leaseOwnerId string, err error) {
	m, err := s.getLeaseManager()
	if err != nil {
		return "", err
	}
	return m.ClaimLease(namespace, id, forDur)
}

func (s *singletonLeaseManager) ReleaseLease(namespace, id string) (err error) {
	m, err := s.getLeaseManager()
	if err != nil {
		return err
	}
	return m.ReleaseLease(namespace, id)
}

func (s *singletonLeaseManager) RetrieveLease(namespace string) (Token, error) {
	m, err := s.getLeaseManager()
	if err != nil {
		return Token{}, err
	}
	return m.RetrieveLease(namespace)
}

func (s *singletonLeaseManager) LeaseReleasedNotifier(namespace string) (notifier <-chan struct{}, err error) {
	m, err := s.getLeaseManager()
	if err != nil {
		return nil, err
	}
	return m.LeaseReleasedNotifier(namespace)
}
