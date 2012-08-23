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
	"launchpad.net/tomb"
	"os"
	"path/filepath"
)

// Status enumerates the possible states for a charm directory.
type Status string

const (
	Missing    Status = ""
	Installing Status = "installing"
	Installed  Status = "installed"
	Upgrading  Status = "upgrading"
	Conflicted Status = "conflicted"
)

var ErrMissing = errors.New("charm is not present")
var ErrConflicted = errors.New("charm upgrade has conflicts")

// Manager is responsible for downloading, installing, and upgrading charms.
type Manager struct {
	charmPath  string
	statePath  string
	bundlesDir *BundlesDir
}

// NewManager returns a new Manages which controls the content of charmDir and
// stores its own state in stateDir.
func NewManager(charmDir, stateDir string) *Manager {
	return &Manager{
		charmPath:  charmDir,
		statePath:  filepath.Join(stateDir, "charm"),
		bundlesDir: NewBundlesDir(filepath.Join(stateDir, "bundles")),
	}
}

// Path returns the path to the charm directory.
func (mgr *Manager) Path() string {
	return mgr.charmPath
}

// ReadURL returns the URL of the currently installed charm. If no charm is
// installed, it returns ErrMissing. Clients should only trust ReadURL while
// a change operation is *not* in progress.
func (mgr *Manager) ReadURL() (*charm.URL, error) {
	path := filepath.Join(mgr.charmPath, ".juju-charm")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = ErrMissing
		}
		return nil, err
	}
	return charm.ParseURL(string(data))
}

type diskState struct {
	Status Status
	URL    string
}

// ReadStatus returns the current status of the charm, and the associated
// charm URL. If no charm is installed, it returns ErrMissing. If the status
// is Installed, the returned URL is that of the installed charm; in all
// other cases, it is the URL of the charm that will be installed once the
// current change succeeds.
func (mgr *Manager) ReadStatus() (Status, *charm.URL, error) {
	var ds diskState
	if err := trivial.ReadYaml(mgr.statePath, &ds); err != nil {
		if os.IsNotExist(err) {
			err = ErrMissing
		}
		return Missing, nil, err
	}
	var err error
	var url *charm.URL
	switch ds.Status {
	case Installed:
		url, err = mgr.ReadURL()
	case Installing, Upgrading, Conflicted:
		url, err = charm.ParseURL(ds.URL)
	default:
		panic(fmt.Errorf("unhandled charm status %q", ds.Status))
	}
	if err != nil {
		return Missing, nil, err
	}
	return ds.Status, url, nil
}

// WriteStatus stores the current status of the charm. If st is Installed,
// url is ignored; in all other cases, the URL must be that of the charm
// that will be installed when the current change succeeds.
func (mgr *Manager) WriteStatus(st Status, url *charm.URL) error {
	if err := trivial.EnsureDir(filepath.Dir(mgr.statePath)); err != nil {
		return err
	}
	surl := ""
	switch st {
	case Installed:
	case Installing, Upgrading, Conflicted:
		surl = url.String()
	default:
		panic(fmt.Errorf("unhandled charm status %q", st))
	}
	return trivial.WriteYaml(mgr.statePath, &diskState{Status: st, URL: surl})
}

// Update sets the content of the charm directory to match the supplied
// charm. If a charm bundle needs to be downloaded, the download will
// abort upon death of the supplied tomb. If the contents of the supplied
// charm conflict with the contents of the existing charm directory, it
// returns ErrConflicted.
func (mgr *Manager) Update(sch *state.Charm, t *tomb.Tomb) (err error) {
	// TODO integrate bzr, to allow for updates/recovery/resolution/rollback.
	bun, err := mgr.bundlesDir.Read(sch, t)
	if err != nil {
		return err
	}
	defer trivial.ErrorContextf(&err, "failed to write charm to %s", mgr.charmPath)
	if err = bun.ExpandTo(mgr.charmPath); err != nil {
		return err
	}
	path := filepath.Join(mgr.charmPath, ".juju-charm")
	return ioutil.WriteFile(path, []byte(sch.URL().String()), 0644)
}

// Resolved signals that update conflicts have been resolved, and puts the
// charm directory into a state from which a fresh attempt at updating is
// expected to succeed. If conflicts have in fact not been resolved, it
// returns ErrConflicted.
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
// being returned. Downloads will be aborted if the supplied tomb dies.
func (d *BundlesDir) Read(sch *state.Charm, t *tomb.Tomb) (*charm.Bundle, error) {
	path := d.bundlePath(sch)
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		} else if err = d.download(sch, t); err != nil {
			return nil, err
		}
	}
	return charm.ReadBundle(path)
}

// download fetches the supplied charm and checks that it has the correct sha256
// hash, then copies it into the directory. If the supplied tomb dies, the
// download will abort.
func (d *BundlesDir) download(sch *state.Charm, t *tomb.Tomb) (err error) {
	defer trivial.ErrorContextf(&err, "failed to download charm %q from %q", sch.URL(), sch.BundleURL())
	dir := d.downloadsPath()
	if err := trivial.EnsureDir(dir); err != nil {
		return err
	}
	dl := downloader.New(sch.BundleURL().String(), dir)
	defer dl.Stop()
	for {
		select {
		case <-t.Dying():
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
