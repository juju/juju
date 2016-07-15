// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrepo // import "gopkg.in/juju/charmrepo.v2-unstable"

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
)

// LocalRepository represents a local directory containing subdirectories
// named after an Ubuntu series, each of which contains charms targeted for
// that series. For example:
//
//   /path/to/repository/oneiric/mongodb/
//   /path/to/repository/precise/mongodb.charm
//   /path/to/repository/precise/wordpress/
type LocalRepository struct {
	Path string
}

var _ Interface = (*LocalRepository)(nil)

// NewLocalRepository creates and return a new local Juju repository pointing
// to the given local path.
func NewLocalRepository(path string) (Interface, error) {
	if path == "" {
		return nil, errgo.New("path to local repository not specified")
	}
	return &LocalRepository{
		Path: path,
	}, nil
}

// Resolve implements Interface.Resolve.
func (r *LocalRepository) Resolve(ref *charm.URL) (*charm.URL, []string, error) {
	if ref.Series == "" {
		return nil, nil, errgo.Newf("no series specified for %s", ref)
	}
	if ref.Revision != -1 {
		return ref, nil, nil
	}
	if ref.Series == "bundle" {
		// Bundles do not have revision files and the revision is not included
		// in metadata. For this reason, local bundles always have revision 0.
		return ref.WithRevision(0), nil, nil
	}
	ch, err := r.Get(ref)
	if err != nil {
		return nil, nil, err
	}
	// This is strictly speaking unnecessary, but just in case a bad charm is
	// used locally, we'll check the series.
	_, err = charm.SeriesForCharm(ref.Series, ch.Meta().Series)
	if err != nil {
		return nil, nil, err
	}
	// We return nil for supported series because even though a charm in a local
	// repository may declare series, it doesn't make sense because charms are
	// expected to be for a single series only in the repository. The local
	// repository concept is deprecated for multi series charms.
	return ref.WithRevision(ch.Revision()), nil, nil
}

func mightBeCharm(info os.FileInfo) bool {
	if info.IsDir() {
		return !strings.HasPrefix(info.Name(), ".")
	}
	return strings.HasSuffix(info.Name(), ".charm")
}

// Get returns a charm matching curl, if one exists. If curl has a revision of
// -1, it returns the latest charm that matches curl. If multiple candidates
// satisfy the foregoing, the first one encountered will be returned.
func (r *LocalRepository) Get(curl *charm.URL) (charm.Charm, error) {
	if err := r.checkUrlAndPath(curl); err != nil {
		return nil, err
	}
	if curl.Series == "bundle" {
		return nil, errgo.Newf("expected a charm URL, got bundle URL %q", curl)
	}
	path := filepath.Join(r.Path, curl.Series)
	infos, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, entityNotFound(curl, r.Path)
	}
	var latest charm.Charm
	for _, info := range infos {
		chPath := filepath.Join(path, info.Name())
		if info.Mode()&os.ModeSymlink != 0 {
			var err error
			if info, err = os.Stat(chPath); err != nil {
				return nil, err
			}
		}
		if !mightBeCharm(info) {
			continue
		}
		if ch, err := charm.ReadCharm(chPath); err != nil {
			logger.Warningf("failed to load charm at %q: %s", chPath, err)
		} else if ch.Meta().Name == curl.Name {
			if ch.Revision() == curl.Revision {
				return ch, nil
			}
			if latest == nil || ch.Revision() > latest.Revision() {
				latest = ch
			}
		}
	}
	if curl.Revision == -1 && latest != nil {
		return latest, nil
	}
	return nil, entityNotFound(curl, r.Path)
}

// GetBundle implements Interface.GetBundle.
func (r *LocalRepository) GetBundle(curl *charm.URL) (charm.Bundle, error) {
	if err := r.checkUrlAndPath(curl); err != nil {
		return nil, err
	}
	if curl.Series != "bundle" {
		return nil, errgo.Newf("expected a bundle URL, got charm URL %q", curl)
	}
	// Note that the bundle does not inherently own a name different than the
	// directory name. Neither the name nor the revision are included in the
	// bundle metadata.
	// TODO frankban: handle bundle revisions, totally ignored for now.
	path := filepath.Join(r.Path, curl.Series, curl.Name)
	info, err := os.Stat(path)
	if err != nil {
		return nil, entityNotFound(curl, r.Path)
	}
	// Do not support bundle archives for the time being. What archive name
	// should we use? What's the use case for compressing bundles anyway?
	if !info.IsDir() {
		return nil, entityNotFound(curl, r.Path)
	}
	return charm.ReadBundleDir(path)
}

// checkUrlAndPath checks that the given URL represents a local entity and that
// the repository path exists.
func (r *LocalRepository) checkUrlAndPath(curl *charm.URL) error {
	if curl.Schema != "local" {
		return fmt.Errorf("local repository got URL with non-local schema: %q", curl)
	}
	info, err := os.Stat(r.Path)
	if err != nil {
		if isNotExistsError(err) {
			return repoNotFound(r.Path)
		}
		return err
	}
	if !info.IsDir() {
		return repoNotFound(r.Path)
	}
	return nil
}
