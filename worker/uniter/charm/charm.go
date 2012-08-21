package charm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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
	Missing         Status = ""
	Installing      Status = "installing"
	Installed       Status = "installed"
	Upgrading       Status = "upgrading"
	UpgradingForced Status = "upgrading-forced"
)

// valid returns whether the status is recognized.
func (s Status) valid() bool {
	switch s {
	case Installing, Installed, Upgrading, UpgradingForced:
		return true
	}
	return false
}

// cstate describes a charm directory's state.
type cstate struct {
	Status Status
}

// StateFile contains the description of a charm directory's state.
type StateFile struct {
	path string
}

// NewStateFile returns a new state file at path.
func NewStateFile(path string) *StateFile {
	return &StateFile{path}
}

// Read reads charm state stored at f.
func (f *StateFile) Read() (Status, error) {
	var st cstate
	if err := trivial.ReadYaml(f.path, &st); err != nil {
		if os.IsNotExist(err) {
			return Missing, nil
		}
		return Missing, err
	}
	if !st.Status.valid() {
		return Missing, fmt.Errorf("invalid charm state at %s", f.path)
	}
	return st.Status, nil
}

// Write writes charm state to f.
func (f *StateFile) Write(s Status) error {
	if !s.valid() {
		panic(fmt.Errorf("invalid charm status %q", s))
	}
	return trivial.WriteYaml(f.path, &cstate{s})
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
