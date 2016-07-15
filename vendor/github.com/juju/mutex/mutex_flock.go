// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// +build !windows,!linux

package mutex

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/juju/errors"
)

type mutex struct {
	fd int
	mu sync.Mutex
}

func acquire(name string) (Releaser, error) {
	flockName := filepath.Join(os.TempDir(), "juju-"+name)
	fd, err := syscall.Open(flockName, syscall.O_CREAT|syscall.O_RDONLY, 0600)
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		syscall.Close(fd)

		if err == syscall.EWOULDBLOCK {
			return nil, errLocked
		}
		return nil, errors.Trace(err)
	}

	return &mutex{fd: fd}, nil
}

// Release implements Releaser.
func (m *mutex) Release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fd == 0 {
		return
	}
	err := syscall.Close(m.fd)
	if err != nil {
		panic(err)
	}
	m.fd = 0
}
