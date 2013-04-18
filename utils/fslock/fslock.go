// On-disk mutex protecting a resource
//
// A lock is represented on disk by a directory of a particular name,
// containing an information file.  Taking a lock is done by renaming a
// temporary directory into place.  We use temporary directories because for
// all filesystems we believe that exactly one attempt to claim the lock will
// succeed and the others will fail.
package fslock

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"time"

	"launchpad.net/juju-core/log"
)

const (
	nameRegexp      = "^[a-z]+[a-z0-9.-]*$"
	heldFilename    = "held"
	messageFilename = "message"
)

var (
	ErrLockNotHeld = errors.New("lock not held")
	ErrTimeout     = errors.New("lock timeout exceeded")

	validName = regexp.MustCompile(nameRegexp)

	lockWaitDelay = 1 * time.Second
)

type Lock struct {
	name   string
	parent string
	nonce  []byte
}

func generateNonce() ([]byte, error) {
	nonce := make([]byte, 20)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return nonce, nil
}

// NewLock returns a new lock with the given name within the given lock
// directory, without acquiring it. The lock name must match the regular
// expression `^[a-z]+[a-z0-9.-]*`.
func NewLock(lockDir, name string) (*Lock, error) {
	if !validName.MatchString(name) {
		return nil, fmt.Errorf("Invalid lock name %q.  Names must match %q", name, nameRegexp)
	}
	nonce, err := generateNonce()
	if err != nil {
		return nil, err
	}
	lock := &Lock{
		name:   name,
		parent: lockDir,
		nonce:  nonce,
	}
	// Ensure the parent exists.
	if err := os.MkdirAll(lock.parent, 0755); err != nil {
		return nil, err
	}
	return lock, nil
}

func (lock *Lock) lockDir() string {
	return path.Join(lock.parent, lock.name)
}

func (lock *Lock) heldFile() string {
	return path.Join(lock.lockDir(), "held")
}

func (lock *Lock) messageFile() string {
	return path.Join(lock.lockDir(), "message")
}

// If message is set, it will write the message to the lock directory as the
// lock is taken.
func (lock *Lock) acquire(message string) (bool, error) {
	// If the lockDir exists, then the lock is held by someone else.
	_, err := os.Stat(lock.lockDir())
	if err == nil {
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	// Create a temporary directory (in the parent dir), and then move it to
	// the right name.  Using the same directory to make sure the directories
	// are on the same filesystem.  Use a directory name starting with "." as
	// it isn't a valid lock name.
	tempLockName := fmt.Sprintf(".%x", lock.nonce)
	tempDirName, err := ioutil.TempDir(lock.parent, tempLockName)
	if err != nil {
		return false, err // this shouldn't really fail...
	}
	// write nonce into the temp dir
	err = ioutil.WriteFile(path.Join(tempDirName, heldFilename), lock.nonce, 0755)
	if err != nil {
		return false, err
	}
	if message != "" {
		err = ioutil.WriteFile(path.Join(tempDirName, messageFilename), []byte(message), 0755)
		if err != nil {
			return false, err
		}
	}
	// Now move the temp directory to the lock directory.
	err = os.Rename(tempDirName, lock.lockDir())
	if err != nil {
		// Any error on rename means we failed.
		log.Infof("Lock %q beaten to the dir rename, %s, currently held: %s", lock.name, message, lock.Message())

		// Beaten to it, clean up temporary directory.
		os.RemoveAll(tempDirName)
		return false, nil
	}
	// We now have the lock.
	return true, nil
}

// Lock blocks until it is able to acquire the lock.  Since we are dealing
// with sharing and locking using the filesystem, it is good behaviour to
// provide a message that is saved with the lock.  This is output in debugging
// information, and can be queried by any other Lock dealing with the same
// lock name and lock directory.
func (lock *Lock) Lock(message string) error {
	var heldMessage = ""
	for {
		acquired, err := lock.acquire(message)
		if err != nil {
			return err
		}
		if acquired {
			return nil
		}
		currMessage := lock.Message()
		if currMessage != heldMessage {
			log.Infof("Attempt Lock failed %q, %s, currently held: %s", lock.name, message, currMessage)
			heldMessage = currMessage
		}
		time.Sleep(lockWaitDelay)
	}
	panic("unreachable")
}

// LockWithTimeout tries to acquire the lock. If it cannot acquire the lock
// within the given duration, it returns ErrTimeout.  See `Lock` for
// information about the message.
func (lock *Lock) LockWithTimeout(duration time.Duration, message string) error {
	deadline := time.Now().Add(duration)
	for {
		acquired, err := lock.acquire(message)
		if err != nil {
			return err
		}
		if acquired {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrTimeout
		}
		time.Sleep(lockWaitDelay)
	}
	panic("unreachable")
}

// IsHeld returns whether the lock is currently held by the receiver.
func (lock *Lock) IsLockHeld() bool {
	heldNonce, err := ioutil.ReadFile(lock.heldFile())
	if err != nil {
		return false
	}
	return bytes.Equal(heldNonce, lock.nonce)
}

// Unlock releases a held lock.  If the lock is not held ErrLockNotHeld is
// returned.
func (lock *Lock) Unlock() error {
	if !lock.IsLockHeld() {
		return ErrLockNotHeld
	}
	// To ensure reasonable unlocking, we should rename to a temp name, and delete that.
	tempLockName := fmt.Sprintf(".%s.%x", lock.name, lock.nonce)
	tempDirName := path.Join(lock.parent, tempLockName)
	// Now move the lock directory to the temp directory to release the lock.
	if err := os.Rename(lock.lockDir(), tempDirName); err != nil {
		return err
	}
	// And now cleanup.
	return os.RemoveAll(tempDirName)
}

// IsLocked returns true if the lock is currently held by anyone.
func (lock *Lock) IsLocked() bool {
	_, err := os.Stat(lock.heldFile())
	return err == nil
}

// BreakLock forcably breaks the lock that is currently being held.
func (lock *Lock) BreakLock() error {
	return os.RemoveAll(lock.lockDir())
}

// SetMessage saves the message if and only if the lock is held.
func (lock *Lock) SetMessage(message string) error {
	if !lock.IsLockHeld() {
		return ErrLockNotHeld
	}
	// Since the message can be read by anyone, make this an atomic write by
	// writing to a temp file and renaming.
	tempFile, err := ioutil.TempFile(lock.lockDir(), ".message")
	if err != nil {
		return err
	}
	tempFilename := tempFile.Name()
	fmt.Fprint(tempFile, message)
	tempFile.Close()
	return os.Rename(tempFilename, lock.messageFile())
}

// Message returns the saved message, or the empty string if there is no
// saved message.
func (lock *Lock) Message() string {
	message, err := ioutil.ReadFile(lock.messageFile())
	if err != nil {
		return ""
	}
	return string(message)
}
