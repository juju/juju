// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// +build windows

package mutex

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/juju/errors"
)

var (
	modkernel32             = syscall.NewLazyDLL("kernel32.dll")
	procCreateSemaphore     = modkernel32.NewProc("CreateSemaphoreW")
	procReleaseSemaphore    = modkernel32.NewProc("ReleaseSemaphore")
	procCloseHandle         = modkernel32.NewProc("CloseHandle")
	procWaitForSingleObject = modkernel32.NewProc("WaitForSingleObject")
)

const (
	ERROR_ALREADY_EXISTS = 183

	WAIT_TIMEOUT = 0x00000102
)

type mutex struct {
	handle syscall.Handle
	mu     sync.Mutex
}

func acquire(name string) (Releaser, error) {
	handle, err := createSemaphore("juju-" + name)
	if err != nil {
		return nil, err
	}
	return &mutex{handle: handle}, nil
}

// Release implements Releaser.
func (m *mutex) Release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handle == syscall.InvalidHandle {
		return
	}
	if err := releaseSemaphone(m.handle); err != nil {
		panic(err)
	}
	if err := closeHandle(m.handle); err != nil {
		panic(err)
	}
	m.handle = syscall.InvalidHandle
}

func createSemaphore(name string) (syscall.Handle, error) {
	var handle syscall.Handle
	defaultAttributes := 0
	initialCount := 0
	maxCount := 1
	semName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return handle, errors.Trace(err)
	}

	result, _, errno := syscall.Syscall6(procCreateSemaphore.Addr(), 4, uintptr(defaultAttributes), uintptr(initialCount), uintptr(maxCount), uintptr(unsafe.Pointer(semName)), 0, 0)
	if result == 0 {
		if errno != 0 {
			return handle, errno
		}
		return handle, syscall.EINVAL
	}
	handle = syscall.Handle(result)
	if errno == ERROR_ALREADY_EXISTS {
		if err := waitForSingleObject(handle); err != nil {
			// Close the handle and return.
			closeHandle(handle)
			return handle, err
		}
	}
	return handle, nil
}

func releaseSemaphone(handle syscall.Handle) error {
	result, _, errno := syscall.Syscall(procReleaseSemaphore.Addr(), 3, uintptr(handle), uintptr(1), 0)
	if result == 0 {
		if errno != 0 {
			return errno
		}
		return syscall.EINVAL
	}
	return nil
}

func waitForSingleObject(handle syscall.Handle) error {
	noWait := 0
	result, _, errno := syscall.Syscall(procWaitForSingleObject.Addr(), 2, uintptr(handle), uintptr(noWait), 0)

	switch result {
	case 0:
		return nil
	case WAIT_TIMEOUT:
		return errLocked
	default:
		if errno != 0 {
			return errno
		}
		return syscall.EINVAL
	}
}

func closeHandle(handle syscall.Handle) error {
	result, _, errno := syscall.Syscall(procCloseHandle.Addr(), 1, uintptr(handle), 0, 0)
	if result == 0 {
		if errno != 0 {
			return errno
		}
		return syscall.EINVAL
	}
	return nil
}
