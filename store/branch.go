package store

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"launchpad.net/juju/go/charm"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)


// PublishBazaarBranch checks out the Bazaar branch from burl and
// publishes its latest revision at urls in the given store.
// The digest parameter must be the most recent known Bazaar
// revision id for the branch tip. In case publishing this specific
// digest for these URLs has been attempted already, the publishing
// procedure may abort early. The published digest is the Bazaar
// revision id of the checked out branch's tip, though, which may
// differ from the provided parameter.
func PublishBazaarBranch(store *Store, urls []*charm.URL, burl string, digest string) (err error) {

	// Prevent other publishers from updating these specific URLs
	// concurrently.
	lock, err := store.LockUpdates(urls)
	if err != nil {
		return err
	}
	defer lock.Unlock()

	var branchDir string
NewTip:
	// Prepare the charm publisher. This will already compute what will
	// be the revision assigned to the charm, and it will also fail in
	// case the operation is unnecessary because charms are up-to-date.
	pub, err := store.CharmPublisher(urls, digest)
	if err != nil {
		return
	}

	// Figure if publishing this charm was already attempted before and
	// failed. We won't try again endlessly if so. In the future we may
	// retry automatically in certain circumstances.
	event, err := store.CharmEvent(urls[0], digest)
	if err == nil && event.Kind != EventPublished {
		return fmt.Errorf("charm publishing previously failed: %s", strings.Join(event.Errors, "; "))
	} else if err != nil && err != ErrNotFound {
		return err
	}

	if branchDir == "" {
		// Retrieve the branch with a lightweight checkout, so that it
		// builds a working tree as cheaply as possible. History
		// doesn't matter here.
		tempDir, err := ioutil.TempDir("", "publish-branch-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
		branchDir = filepath.Join(tempDir, "branch")
		output, err := exec.Command("bzr", "checkout", "--lightweight", burl, branchDir).CombinedOutput()
		if err != nil {
			return outputErr(output, err)
		}

		// Pick actual digest from tip. Publishing the real tip
		// revision rather than the revision for the digest provided is
		// strictly necessary to prevent a race condition. If the
		// provided digest was published instead, there's a chance
		// another publisher concurrently running could have found a
		// newer revision and published that first, and the digest
		// parameter provided is in fact an old version that would
		// overwrite the new version.
		tipDigest, err := bzrRevisionId(branchDir)
		if err != nil {
			return err
		}
		if tipDigest != digest {
			digest = tipDigest
			goto NewTip
		}
	}

	ch, err := charm.ReadDir(branchDir)
	if err == nil {
		// Hand over the charm to the store for bundling and
		// streaming its content into the database.
		err = pub.Publish(ch)
		if err == ErrUpdateConflict {
			// A conflict may happen in edge cases if the whole
			// locking mechanism fails due to an expiration event,
			// and then the expired concurrent publisher revives
			// for whatever reason and attempts to finish
			// publishing. The state of the system is still
			// consistent in that case, and the error isn't logged
			// since the revision was properly published before.
			return err
		}
	}

	// Publishing is done. Log failure or error.
	event = &CharmEvent{
		URLs: urls,
		Digest: digest,
	}
	if err == nil {
		event.Kind = EventPublished
		event.Revision = pub.Revision()
	} else {
		event.Kind = EventPublishError
		event.Errors = []string{err.Error()}
	}
	if logerr := store.LogCharmEvent(event); logerr != nil {
		if err == nil {
			err = logerr
		} else {
			err = fmt.Errorf("%v; %v", err, logerr)
		}
	}
	return err
}

// bzrRevisionId returns the Bazaar revision id for the branch in branchDir.
func bzrRevisionId(branchDir string) (string, error) {
	cmd := exec.Command("bzr", "revision-info")
	cmd.Dir = branchDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", outputErr(output, err)
	}
	pair := bytes.Fields(output)
	if len(pair) != 2 {
		return "", fmt.Errorf(`invalid output from "bzr revision-info": %s`, output)
	}
	return string(pair[1]), nil
}

// outputErr returns an error that assembles some command's output and its
// error, if both output and err are set, and returns only err if output is nil.
func outputErr(output []byte, err error) error {
	if len(output) > 0 {
		return fmt.Errorf("%v\n%s", err, output)
	}
	return err
}
