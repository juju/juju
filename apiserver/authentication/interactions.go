// Copyright 2016 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
)

// ErrWaitCanceled is returned by Interactions.Wait when the cancel
// channel is signalled.
var ErrWaitCanceled = errors.New("wait canceled")

// ErrExpired is returned by Interactions.Wait when interactions expire
// before they are done.
var ErrExpired = errors.New("interaction timed out")

// Interactions maintains a set of Interactions.
type Interactions struct {
	mu    sync.Mutex
	items map[string]*item
}

type item struct {
	c        chan Interaction
	caveatId []byte
	expiry   time.Time
	done     bool
}

// Interaction records details of an in-progress interactive
// macaroon-based login.
type Interaction struct {
	CaveatId   []byte
	LoginUser  names.UserTag
	LoginError error
}

// NewInteractions returns a new Interactions.
func NewInteractions() *Interactions {
	return &Interactions{
		items: make(map[string]*item),
	}
}

func newId() (string, error) {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("cannot read random id: %v", err)
	}
	return fmt.Sprintf("%x", id[:]), nil
}

// Start records the start of an interactive login, and returns a random ID
// that uniquely identifies it. A call to Wait with the same ID will return
// the Interaction once it is done.
func (m *Interactions) Start(caveatId []byte, expiry time.Time) (string, error) {
	id, err := newId()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[id] = &item{
		c:        make(chan Interaction, 1),
		caveatId: caveatId,
		expiry:   expiry,
	}
	return id, nil
}

// Done signals that the user has either logged in, or attempted to and failed.
func (m *Interactions) Done(id string, loginUser names.UserTag, loginError error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.items[id]

	if item == nil {
		return errors.NotFoundf("interaction %q", id)
	}
	if item.done {
		return errors.Errorf("interaction %q already done", id)
	}
	item.done = true
	item.c <- Interaction{
		CaveatId:   item.caveatId,
		LoginUser:  loginUser,
		LoginError: loginError,
	}
	return nil
}

// Wait waits until the identified interaction is done, and returns the
// corresponding Interaction. If the cancel channel is signalled before
// the interaction is done, then ErrWaitCanceled is returned. If the
// interaction expires before it is done, ErrExpired is returned.
func (m *Interactions) Wait(id string, cancel <-chan struct{}) (*Interaction, error) {
	m.mu.Lock()
	item := m.items[id]
	m.mu.Unlock()
	if item == nil {
		return nil, errors.NotFoundf("interaction %q", id)
	}
	select {
	case <-cancel:
		return nil, ErrWaitCanceled
	case interaction, ok := <-item.c:
		if !ok {
			return nil, ErrExpired
		}
		m.mu.Lock()
		delete(m.items, id)
		m.mu.Unlock()
		return &interaction, nil
	}
}

// Expire removes any interactions that were due to expire by the
// specified time.
func (m *Interactions) Expire(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, item := range m.items {
		if item.done || item.expiry.After(t) {
			continue
		}
		delete(m.items, id)
		close(item.c)
	}
}
