package charm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/tomb"
	"os"
	"path/filepath"
)

// Status enumerates the possible states for the charm directory.
type Status string

const (
	Missing         Status = ""
	Installing      Status = "installing"
	Installed       Status = "installed"
	Upgrading       Status = "upgrading"
	UpgradingForced Status = "upgrading-forced"
)

// valid returns false if the status is not recognized.
func (s Status) valid() bool {
	switch s {
	case Missing, Installing, Installed, Upgrading, UpgradingForced:
		return true
	}
	return false
}

// State describes a charm directory's state.
type State struct {
	Status Status
}

// StateFile gives access to persistent charm state.
type StateFile string

// Read reads charm state stored at f.
func (f StateFile) Read() (st State, err error) {
	data, err := ioutil.ReadFile(string(f))
	if os.IsNotExist(err) {
		return st, nil
	} else if err != nil {
		return
	}
	if err = goyaml.Unmarshal(data, &st); err != nil {
		return
	}
	if !st.Status.valid() {
		return State{}, fmt.Errorf("invalid charm state at %s", f)
	}
	return
}

// Write writes charm state to f.
func (f StateFile) Write(s Status) error {
	if !s.valid() {
		panic(fmt.Errorf("unknown charm status %q", s))
	} else if s == Missing {
		panic("insane operation")
	}
	return trivial.AtomicWrite(f, &State{s})
}

// Bundles is responsible for storing and retrieving charm bundles
// identified by state charms.
type Bundles struct {
	path string
}

func NewBundles(path string) *Bundles {
	return *Bundles{path}
}

// Get returns a charm bundle from the directory. If no bundle exists yet,
// one will be downloaded and validated and copied into the bundles directory
// before being returned. Downloads will be aborted if the supplied tomb dies.
func (b Bundles) Get(sch *state.Charm, t *tomb.Tomb) (*charm.Bundle, error) {
	path := b.path(sch)
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		} else if err = b.download(sch, t); err != nil {
			return nil, err
		}
	}
	return charm.ReadBundle(path)
}

// download gets the supplied charm and checks that it has the correct sha256
// hash, then copies it into the bundles directory. If the supplied tomb dies,
// the download will abort.
func (b Bundles) download(sch *state.Charm, t *tomb.Tomb) (err error) {
	dl := downloader.New(sch.BundleURL().String())
	defer dl.Stop()
	for {
		select {
		case <-t.Dying():
			return tomb.ErrDying
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
					"sha256 mismatch for %q from %q: expected %q, got %q",
					sch.URL(), sch.BundleURL(), sch.BundleSha256(), actualSha256,
				)
			}
			if err := trivial.EnsureDir(b); err != nil {
				return err
			}
			return os.Rename(st.File.Name(), b.path(sch))
		}
	}
	panic("unreachable")
}

// path returns the path to the location where the verified charm
// bundle identified by sch will be, or has been, saved.
func (b Bundles) bundlePath(sch *state.Charm) string {
	return filepath.Join(b, charm.Quote(sch.URL().String()))
}
