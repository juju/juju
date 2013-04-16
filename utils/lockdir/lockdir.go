// On-disk mutex protecting a resource
//
// A lock is represented on disk by a directory of a particular name,
// containing an information file.  Taking a lock is done by renaming a
// temporary directory into place.  We use temporary directories because for
// all filesystems we believe that exactly one attempt to claim the lock will
// succeed and the others will fail.  (Files won't do because some filesystems
// or transports only have rename-and-overwrite, making it hard to tell who
// won.)

package lockdir

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"time"
)

var (
	InvalidLockName = errors.New("Lock names must match regex `^[a-z]+[a-z0-9.-]*$")
	LockNotHeld     = errors.New("Lock not held")

	validName = regexp.MustCompile("^[a-z]+[a-z0-9.-]*$")

	lockWaitDelay = 1 * time.Second
)

type Lock struct {
	name    string
	lockDir string
	nonce   string
}

func GenerateNonce() (string, error) {
	const size = 20
	var nonce [size]byte
	if _, err := io.ReadFull(rand.Reader, []byte(nonce[0:size])); err != nil {
		return "", err
	}
	return string(nonce[:]), nil
}

// Return a new lock.
func NewLock(lockDir, name string) (*Lock, error) {
	nonce, err := GenerateNonce()
	if !validName.MatchString(name) {
		return nil, InvalidLockName
	}
	if err != nil {
		return nil, err
	}
	lock := &Lock{
		name:    name,
		lockDir: lockDir,
		nonce:   nonce,
	}
	// Ensure the lockDir exists.
	dir, err := os.Open(lock.lockDir)
	if os.IsNotExist(err) {
		// try to make it
		err = os.MkdirAll(lock.lockDir, 0755)
		// Since we have just created the directory successfully, return now.
		if err == nil {
			return lock, nil
		}
	}
	if err != nil {
		return nil, err
	}
	// Make sure it is actually a directory
	fileInfo, err := dir.Stat()
	if err != nil {
		return nil, err
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("lock dir %q exists and is a file not a directory", lockDir)
	}
	return lock, nil
}

func (lock *Lock) namedLockDir() string {
	return path.Join(lock.lockDir, lock.name)
}

func (lock *Lock) heldFile() string {
	return path.Join(lock.namedLockDir(), "held")
}

func (lock *Lock) acquire() (bool, error) {
	// If the namedLockDir exists, then the lock is held by someone else.
	dir, err := os.Open(lock.namedLockDir())
	if err == nil {
		dir.Close()
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	// Create a temporary directory (in the temp dir), and then move it to the right name.
	tempDirName, err := ioutil.TempDir("", "temp-lock")
	if err != nil {
		return false, err // this shouldn't really fail...
	}
	err = os.Rename(tempDirName, lock.namedLockDir())
	if os.IsExist(err) {
		// Beaten to it, clean up temporary directory.
		os.RemoveAll(tempDirName)
		return false, nil
	} else if err != nil {
		return false, err
	}
	// write nonce
	err = ioutil.WriteFile(lock.heldFile(), []byte(lock.nonce), 0755)
	if err != nil {
		return false, err
	}
	// We now have the lock.
	return true, nil
}

// Lock blocks until it is able to acquire the lock.
func (lock *Lock) Lock() error {
	for {
		acquired, err := lock.acquire()
		if err != nil {
			return err
		}
		if acquired {
			return nil
		}
		time.Sleep(lockWaitDelay)
	}
	panic("unreachable")
	return nil // unreachable
}

func (lock *Lock) TryLock(duration time.Duration) (isLocked bool, err error) {
	locked := make(chan bool)
	error := make(chan error)
	timeout := make(chan struct{})
	defer func() {
		close(locked)
		close(error)
		close(timeout)
	}()

	go func() {
		for {
			acquired, err := lock.acquire()
			if err != nil {
				locked <- false
				error <- err
				return
			}
			if acquired {
				locked <- true
				error <- nil
				return
			}
			select {
			case <-timeout:
				locked <- false
				error <- nil
				return
			case <-time.After(lockWaitDelay):
				// Keep trying...
			}
		}
	}()

	select {
	case isLocked = <-locked:
		err = <-error
		return
	case <-time.After(duration):
		timeout <- struct{}{}
	}
	// It is possible that the timeout got signalled just before the goroutine
	// tried again, so check the results rather than automatically failing.
	return <-locked, <-error
}

// IsLockHeld returns true if and only if the namedLockDir exists, and the
// file 'held' in that directory contains the nonce for this lock.
func (lock *Lock) IsLockHeld() bool {
	heldNonce, err := ioutil.ReadFile(lock.heldFile())
	if err != nil {
		return false
	}
	return string(heldNonce) == lock.nonce
}

func (lock *Lock) Unlock() error {
	if !lock.IsLockHeld() {
		return LockNotHeld
	}
	return os.RemoveAll(lock.namedLockDir())
}
