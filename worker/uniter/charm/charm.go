package charm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	"os"
	"path/filepath"
)

// Status enumerates the possible states for a charm directory.
type Status string

const (
	Installing Status = "installing"
	Deployed   Status = "deployed"
	Upgrading  Status = "upgrading"
	Conflicted Status = "conflicted"
)

var ErrMissing = errors.New("charm deployment not found")
var ErrConflict = errors.New("charm upgrade has conflicts")

// Manager is responsible for downloading, deploying, and upgrading charms.
type Manager struct {
	charmDir   string
	statePath  string
	bundlesDir *BundlesDir
}

// NewManager returns a new Manager that controls the content of charmDir and
// stores its own state in stateDir.
func NewManager(charmDir, stateDir string) *Manager {
	return &Manager{
		charmDir:   charmDir,
		statePath:  filepath.Join(stateDir, "charm"),
		bundlesDir: NewBundlesDir(filepath.Join(stateDir, "bundles")),
	}
}

// CharmDir returns the path to the charm directory.
func (mgr *Manager) CharmDir() string {
	return mgr.charmDir
}

// State describes the state of a charm directory. If Status is Deployed, the
// URL field refers to the current deployed charm; otherwise, it refers to the
// charm proposed by the change described in Status.
type State struct {
	Status Status
	URL    *charm.URL
}

// diskState defines the State serialization.
type diskState struct {
	Status Status
	URL    string
}

// ReadState returns the current state of the charm. If no charm is deployed,
// it returns ErrMissing.
func (mgr *Manager) ReadState() (State, error) {
	var ds diskState
	if err := trivial.ReadYaml(mgr.statePath, &ds); err != nil {
		if os.IsNotExist(err) {
			err = ErrMissing
		}
		return State{}, err
	}
	var err error
	var url *charm.URL
	switch ds.Status {
	case Deployed:
		url, err = mgr.deployedURL()
	case Installing, Upgrading, Conflicted:
		url, err = charm.ParseURL(ds.URL)
	default:
		panic(fmt.Errorf("unhandled charm status %q", ds.Status))
	}
	if err != nil {
		return State{}, err
	}
	return State{ds.Status, url}, nil
}

// deployedURL returns the URL of the currently deployed charm. If no charm is
// deployed, it returns ErrMissing.
func (mgr *Manager) deployedURL() (*charm.URL, error) {
	path := filepath.Join(mgr.charmDir, ".juju-charm")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = ErrMissing
		}
		return nil, err
	}
	return charm.ParseURL(string(data))
}

// WriteState stores the current state of the charm.
func (mgr *Manager) WriteState(st Status, url *charm.URL) error {
	if err := trivial.EnsureDir(filepath.Dir(mgr.statePath)); err != nil {
		return err
	}
	switch st {
	case Deployed, Installing, Upgrading, Conflicted:
	default:
		panic(fmt.Errorf("unhandled charm status %q", st))
	}
	return trivial.WriteYaml(mgr.statePath, &diskState{Status: st, URL: url.String()})
}

// Update sets the content of the charm directory to match the supplied
// charm. If a charm bundle needs to be downloaded, the download will
// abort when a value is received on the supplied channel. If the contents
// of the supplied charm conflict with the contents of the existing
// charm directory, it returns ErrConflict.
func (mgr *Manager) Update(sch *state.Charm, abort <-chan struct{}) (err error) {
	// TODO integrate bzr, to allow for updates/recovery/resolution/rollback.
	bun, err := mgr.bundlesDir.Read(sch, abort)
	if err != nil {
		return err
	}
	defer trivial.ErrorContextf(&err, "failed to write charm to %s", mgr.charmDir)
	if err = bun.ExpandTo(mgr.charmDir); err != nil {
		return err
	}
	path := filepath.Join(mgr.charmDir, ".juju-charm")
	return ioutil.WriteFile(path, []byte(sch.URL().String()), 0644)
}

// Resolved signals that update conflicts have been resolved, and puts the
// charm directory into a state from which a fresh attempt at updating is
// expected to succeed. If conflicts have in fact not been resolved, it
// returns ErrConflict.
func (mgr *Manager) Resolved() error {
	panic("charm.Manager.Resolved not implemented")
}

// Revert restores the content of the charm directory to that which existed
// when it entered its current status. Files not present in the charm bundles
// will not be changed.
func (mgr *Manager) Revert() error {
	panic("charm.Manager.Revert not implemented")
}

// BundlesDir is responsible for storing and retrieving charm bundles
// identified by state charms.
type BundlesDir struct {
	path string
}

// NewBundlesDir returns a new BundlesDir which uses path for storage.
func NewBundlesDir(path string) *BundlesDir {
	return &BundlesDir{path}
}

// Read returns a charm bundle from the directory. If no bundle exists yet,
// one will be downloaded and validated and copied into the directory before
// being returned. Downloads will be aborted if a value is received on abort.
func (d *BundlesDir) Read(sch *state.Charm, abort <-chan struct{}) (*charm.Bundle, error) {
	path := d.bundlePath(sch)
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		} else if err = d.download(sch, abort); err != nil {
			return nil, err
		}
	}
	return charm.ReadBundle(path)
}

// download fetches the supplied charm and checks that it has the correct sha256
// hash, then copies it into the directory. If a value is received on abort, the
// download will be stopped.
func (d *BundlesDir) download(sch *state.Charm, abort <-chan struct{}) (err error) {
	defer trivial.ErrorContextf(&err, "failed to download charm %q from %q", sch.URL(), sch.BundleURL())
	dir := d.downloadsPath()
	if err := trivial.EnsureDir(dir); err != nil {
		return err
	}
	dl := downloader.New(sch.BundleURL().String(), dir)
	defer dl.Stop()
	for {
		select {
		case <-abort:
			return fmt.Errorf("aborted")
		case st := <-dl.Done():
			if st.Err != nil {
				return st.Err
			}
			defer st.File.Close()
			hash := sha256.New()
			if _, err = io.Copy(hash, st.File); err != nil {
				return err
			}
			actualSha256 := hex.EncodeToString(hash.Sum(nil))
			if actualSha256 != sch.BundleSha256() {
				return fmt.Errorf(
					"expected sha256 %q, got %q", sch.BundleSha256(), actualSha256,
				)
			}
			if err := trivial.EnsureDir(d.path); err != nil {
				return err
			}
			return os.Rename(st.File.Name(), d.bundlePath(sch))
		}
	}
	panic("unreachable")
}

// bundlePath returns the path to the location where the verified charm
// bundle identified by sch will be, or has been, saved.
func (d *BundlesDir) bundlePath(sch *state.Charm) string {
	return filepath.Join(d.path, charm.Quote(sch.URL().String()))
}

// downloadsPath returns the path to the directory into which charms are
// downloaded.
func (d *BundlesDir) downloadsPath() string {
	return filepath.Join(d.path, "downloads")
}
