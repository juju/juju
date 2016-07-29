// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// +build linux

package mutex

import (
	"net"
	"path/filepath"
	"sync"

	"github.com/juju/errors"
)

const prefix = "@/var/lib/juju/mutex-"

type mutex struct {
	socket *net.UnixListener
	mu     sync.Mutex
}

func acquire(name string) (Releaser, error) {
	path := filepath.Join(prefix, name)
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, errors.Trace(err)
	}
	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, errLocked
	}
	return &mutex{socket: l}, nil
}

// Release implements Releaser.
func (m *mutex) Release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.socket == nil {
		return
	}
	if err := m.socket.Close(); err != nil {
		panic(err)
	}
	m.socket = nil
}
