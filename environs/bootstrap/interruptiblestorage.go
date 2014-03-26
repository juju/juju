// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"errors"
	"io"

	"launchpad.net/juju-core/environs/storage"
)

var interruptedError = errors.New("interrupted")

// interruptibleStorage is a storage.Storage that sits
// between the user and another Storage, allowing the
// Put method to be interrupted.
type interruptibleStorage struct {
	storage.Storage
	interrupt <-chan struct{}
}

// newInterruptibleStorage wraps the provided Storage so that Put
// will immediately return an error if the provided channel is
// closed.
func newInterruptibleStorage(s storage.Storage, interrupt <-chan struct{}) storage.Storage {
	return &interruptibleStorage{s, interrupt}
}

type interruptibleReader struct {
	io.Reader
	interrupt <-chan struct{}
}

func (r *interruptibleReader) Read(p []byte) (int, error) {
	// if the interrupt channel is already
	// closed, just drop out immediately.
	select {
	case <-r.interrupt:
		return 0, interruptedError
	default:
	}

	// read and wait for interruption concurrently
	var n int
	var err error
	done := make(chan struct{})
	go func() {
		defer close(done)
		n, err = r.Reader.Read(p)
	}()
	select {
	case <-done:
		return n, err
	case <-r.interrupt:
		return 0, interruptedError
	}
}

func (s *interruptibleStorage) Put(name string, r io.Reader, length int64) error {
	return s.Storage.Put(name, &interruptibleReader{r, s.interrupt}, length)
}
