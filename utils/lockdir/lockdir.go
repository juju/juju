// On-disk mutex protecting a resource
//
// Waiting for a lock must be done by polling; this can be aborted after a timeout.
//
// Locks must always be explicitly released, typically using defer.
//
// Locks may fail to be released if the process is abruptly terminated (machine stop, SIGKILL).
//
// A lock is represented on disk by a directory of a particular name,
// containing an information file.  Taking a lock is done by renaming a
// temporary directory into place.  We use temporary directories because for
// all filesystems we believe that exactly one attempt to claim the lock will
// succeed and the others will fail.  (Files won't do because some filesystems
// or transports only have rename-and-overwrite, making it hard to tell who
// won.)
//
// The desired characteristics are:
//
// TODO: check these
// * Locks are not reentrant.  (That is, a client that tries to take a
//   lock it already holds may deadlock or fail.)
// * Stale locks can be guessed at by a heuristic
// * Lost locks can be broken by any client
// * Failed lock operations leave little or no mess
// * Deadlocks are avoided by having a timeout always in use, clients
//   desiring indefinite waits can retry or set a silly big timeout.
//
// Locks are generally stored in the JUJU_DATA dir, in a locks directory.
//
// Locks are named, the name should be lower case with dashes, and will be
// enforced through a regex.

package lockdir

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
)

var (
	InvalidLockName = errors.New("Lock names must match regex `^[a-z]+[a-z0-9.-]*$")
	LockFailed      = errors.New("xxx")

	validName = regexp.MustCompile("^[a-z]+[a-z0-9.-]*$")
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
	// TODO: check name is valid.
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

func (lock *Lock) NamedLockDir() string {
	return path.Join(lock.lockDir, lock.name)
}

func (lock *Lock) Acquire() {
}

// use a real time out...
func (lock *Lock) TryAcquire(timeout int) error {
	return nil

	// + select {
	// + case o := <-s.op:
	// + c.Fatalf("unexpected operation %#v", o)
	// + case <-time.After(200 * time.Millisecond):
	// + return

}

// IsLockHeld returns true if and only if the
func (lock *Lock) IsLockHeld() bool {
	return false
}

func (lock *Lock) Unlock() {
}
