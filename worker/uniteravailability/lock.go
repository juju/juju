// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniteravailability

import (
	"errors"
	"sync/atomic"
)

var ErrAbort = errors.New("abort")

type RWAbortableLock struct {
	writeLock chan struct{}
	readLock  chan struct{}
	readers   uint32
	abort     chan struct{}
}

func NewRWAbortableLock() *RWAbortableLock {
	l := &RWAbortableLock{
		writeLock: make(chan struct{}, 1),
		readLock:  make(chan struct{}, 1),
		abort:     make(chan struct{}, 1),
	}
	return l
}

// Lock acquires a write lock. It only returns an error if an abort
// signal was received during the lock acquisition.
func (l *RWAbortableLock) Lock() error {
	// Lock writes.
	select {
	case <-l.abort:
		return ErrAbort
	case l.writeLock <- struct{}{}:
	}
	// Lock reads.
	select {
	case <-l.abort:
		return ErrAbort
	case l.readLock <- struct{}{}:
	}
	return nil
}

// Unlock releases the write lock.
func (l *RWAbortableLock) Unlock() {
	select {
	case <-l.writeLock:
		<-l.readLock
	default:
		// Lock already unlocked, consider this a nop.
	}
}

// Abort signals the lock that any blocked lock acquisitions should abort.
func (l *RWAbortableLock) Abort() {
	l.abort <- struct{}{}
}

// RLock acquires the read lock. It only returns an error if an abort
// signal was received during the lock acquisition.
func (l *RWAbortableLock) RLock() error {
	// Lock writes.
	select {
	case <-l.abort:
		return ErrAbort
	case l.writeLock <- struct{}{}:
	}
	// Lock reads.
	for {
		readers := l.readers
		newReaders := readers + 1
		if atomic.CompareAndSwapUint32(&l.readers, readers, newReaders) {
			select {
			case <-l.abort:
				return ErrAbort
			case l.readLock <- struct{}{}:
			default:
				// We don't care if the read lock is already acquired.
			}
			break
		}
	}
	<-l.writeLock
	return nil
}

// RUnlock releases the read lock.
func (l *RWAbortableLock) RUnlock() {
	for {
		readers := l.readers
		if readers == 0 {
			break
		}
		newReaders := readers - 1
		if atomic.CompareAndSwapUint32(&l.readers, readers, newReaders) {
			if newReaders == 0 {
				select {
				case <-l.readLock:
				default:
				}
			}
			break
		}
	}
}
