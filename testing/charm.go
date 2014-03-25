// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"launchpad.net/juju-core/charm"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

// Repo represents a charm repository used for testing.
type Repo struct {
	once sync.Once
	path string
}

func (r *Repo) Path() string {
	r.once.Do(r.init)
	return r.path
}

// init is called once when r.Path() is called for the first time, and
// it initializes r.path to the location of the local testing
// repository.
func (r *Repo) init() {
	p, err := build.Import("launchpad.net/juju-core/testing", "", build.FindOnly)
	check(err)
	r.path = filepath.Join(p.Dir, "repo")
}

// Charms represents the specific charm repository stored in this package and
// used by the Juju unit tests. The series name is "quantal".
var Charms = &Repo{}

func clone(dst, src string) string {
	check(exec.Command("cp", "-r", src, dst).Run())
	return filepath.Join(dst, filepath.Base(src))
}

// DirPath returns the path to a charm directory with the given name in the
// default series
func (r *Repo) DirPath(name string) string {
	return filepath.Join(r.Path(), "quantal", name)
}

// Dir returns the actual charm.Dir named name.
func (r *Repo) Dir(name string) *charm.Dir {
	ch, err := charm.ReadDir(r.DirPath(name))
	check(err)
	return ch
}

// ClonedDirPath returns the path to a new copy of the default charm directory
// named name.
func (r *Repo) ClonedDirPath(dst, name string) string {
	return clone(dst, r.DirPath(name))
}

// RenamedClonedDirPath returns the path to a new copy of the default
// charm directory named name, but renames it to newName.
func (r *Repo) RenamedClonedDirPath(dst, name, newName string) string {
	newDst := clone(dst, r.DirPath(name))
	renamedDst := filepath.Join(filepath.Dir(newDst), newName)
	check(os.Rename(newDst, renamedDst))
	return renamedDst
}

// ClonedDir returns an actual charm.Dir based on a new copy of the charm directory
// named name, in the directory dst.
func (r *Repo) ClonedDir(dst, name string) *charm.Dir {
	ch, err := charm.ReadDir(r.ClonedDirPath(dst, name))
	check(err)
	return ch
}

// ClonedURL makes a copy of the charm directory. It will create a directory
// with the series name if it does not exist, and then clone the charm named
// name into that directory. The return value is a URL pointing at the local
// charm.
func (r *Repo) ClonedURL(dst, series, name string) *charm.URL {
	dst = filepath.Join(dst, series)
	if err := os.MkdirAll(dst, os.FileMode(0777)); err != nil {
		panic(fmt.Errorf("cannot make destination directory: %v", err))
	}
	clone(dst, r.DirPath(name))
	return &charm.URL{
		Reference: charm.Reference{
			Schema:   "local",
			Name:     name,
			Revision: -1,
		},
		Series: series,
	}
}

// BundlePath returns the path to a new charm bundle file created from the
// charm directory named name, in the directory dst.
func (r *Repo) BundlePath(dst, name string) string {
	dir := r.Dir(name)
	path := filepath.Join(dst, "bundle.charm")
	file, err := os.Create(path)
	check(err)
	defer file.Close()
	check(dir.BundleTo(file))
	return path
}

// Bundle returns an actual charm.Bundle created from a new charm bundle file
// created from the charm directory named name, in the directory dst.
func (r *Repo) Bundle(dst, name string) *charm.Bundle {
	ch, err := charm.ReadBundle(r.BundlePath(dst, name))
	check(err)
	return ch
}

// MockCharmStore implements charm.Repository and is used to isolate tests
// that would otherwise need to hit the real charm store.
type MockCharmStore struct {
	charms        map[string]map[int]*charm.Bundle
	AuthAttrs     string
	TestMode      bool
	DefaultSeries string
}

func NewMockCharmStore() *MockCharmStore {
	return &MockCharmStore{charms: map[string]map[int]*charm.Bundle{}}
}

func (s *MockCharmStore) WithAuthAttrs(auth string) charm.Repository {
	s.AuthAttrs = auth
	return s
}

func (s *MockCharmStore) WithTestMode(testMode bool) charm.Repository {
	s.TestMode = testMode
	return s
}

func (s *MockCharmStore) WithDefaultSeries(series string) charm.Repository {
	s.DefaultSeries = series
	return s
}

func (s *MockCharmStore) Resolve(ref charm.Reference) (*charm.URL, error) {
	if s.DefaultSeries == "" {
		return nil, fmt.Errorf("missing default series, cannot resolve charm url: %q", ref)
	}
	return &charm.URL{Reference: ref, Series: s.DefaultSeries}, nil
}

// SetCharm adds and removes charms in s. The affected charm is identified by
// charmURL, which must be revisioned. If bundle is nil, the charm will be
// removed; otherwise, it will be stored. It is an error to store a bundle
// under a charmURL that does not share its name and revision.
func (s *MockCharmStore) SetCharm(charmURL *charm.URL, bundle *charm.Bundle) error {
	base := charmURL.WithRevision(-1).String()
	if charmURL.Revision < 0 {
		return fmt.Errorf("bad charm url revision")
	}
	if bundle == nil {
		delete(s.charms[base], charmURL.Revision)
		return nil
	}
	bundleRev := bundle.Revision()
	bundleName := bundle.Meta().Name
	if bundleName != charmURL.Name || bundleRev != charmURL.Revision {
		return fmt.Errorf("charm url %s mismatch with bundle %s-%d", charmURL, bundleName, bundleRev)
	}
	if _, found := s.charms[base]; !found {
		s.charms[base] = map[int]*charm.Bundle{}
	}
	s.charms[base][charmURL.Revision] = bundle
	return nil
}

// interpret extracts from charmURL information relevant to both Latest and
// Get. The returned "base" is always the string representation of the
// unrevisioned part of charmURL; the "rev" wil be taken from the charmURL if
// available, and will otherwise be the revision of the latest charm in the
// store with the same "base".
func (s *MockCharmStore) interpret(charmURL *charm.URL) (base string, rev int) {
	base, rev = charmURL.WithRevision(-1).String(), charmURL.Revision
	if rev == -1 {
		for candidate := range s.charms[base] {
			if candidate > rev {
				rev = candidate
			}
		}
	}
	return
}

// Get implements charm.Repository.Get.
func (s *MockCharmStore) Get(charmURL *charm.URL) (charm.Charm, error) {
	base, rev := s.interpret(charmURL)
	charm, found := s.charms[base][rev]
	if !found {
		return nil, fmt.Errorf("charm not found in mock store: %s", charmURL)
	}
	return charm, nil
}

// Latest implements charm.Repository.Latest.
func (s *MockCharmStore) Latest(charmURLs ...*charm.URL) ([]charm.CharmRevision, error) {
	result := make([]charm.CharmRevision, len(charmURLs))
	for i, curl := range charmURLs {
		charmURL := curl.WithRevision(-1)
		base, rev := s.interpret(charmURL)
		if _, found := s.charms[base][rev]; !found {
			result[i].Err = fmt.Errorf("charm not found in mock store: %s", charmURL)
		} else {
			result[i].Revision = rev
		}
	}
	return result, nil
}
