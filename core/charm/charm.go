// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"io/ioutil"
	"os"

	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	"github.com/juju/errors"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/utils"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
)

// StateCharm represents a stored charm from state.
type StateCharm interface {
	IsUploaded() bool
}

// State defines the underlying state for handling charms.
type State interface {
	PrepareCharmUpload(*charm.URL) (StateCharm, error)
}

// StoreCharm represents a store charm.
type StoreCharm interface {
	lxdprofile.LXDProfiler
	Meta() *charm.Meta
}

// Store defines the store for which the charm is being downloaded from.
type Store interface {
	// Validate checks to ensure that the charm URL is valid for the store.
	Validate(*charm.URL) error
	// Download a charm from the store using the charm URL.
	Download(*charm.URL, string) (StoreCharm, Checksum, error)
}

// Checksum defines a function for running checksums against.
type Checksum func(string) bool

// VersionValidator validates the version of the store charm.
type VersionValidator interface {
	// Check the version is valid for the given version.
	Validate(*charm.Meta) error
}

// Procedure defines a procedure for adding a charm to state.
type Procedure struct {
	charmURL *charm.URL
	store    Store
	state    State
	version  VersionValidator
	force    bool
}

// DownloadFromCharmStore will creates a procedure to install a charm from the
// charm store.
func DownloadFromCharmStore(url string, force bool) (*Procedure, error) {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Procedure{
		charmURL: curl,
		store:    StoreCharmStore{},
		force:    force,
	}, nil
}

// DownloadFromCharmHub will creates a procedure to install a charm from the
// charm hub.
func DownloadFromCharmHub(url string, force bool) (*Procedure, error) {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Procedure{
		charmURL: curl,
		store:    StoreCharmHub{},
	}, nil
}

// Validate will attempt to validate the requirements for adding a charm to the
// store.
func (p *Procedure) Validate() error {
	if err := p.store.Validate(p.charmURL); err != nil {
		return errors.Trace(err)
	}
	if p.charmURL.Revision < 0 {
		return errors.Errorf("charm URL must include a revision")
	}
	return nil
}

// Run the procedure against the correct store.
func (p *Procedure) Run() (func() error, error) {
	charm, err := p.state.PrepareCharmUpload(p.charmURL)
	if err != nil {
		return func() error { return nil }, errors.Trace(err)
	}

	// Charm is already in state, so we can exit out early.
	if charm.IsUploaded() {
		return func() error { return nil }, nil
	}

	// Get the charm and its information from the store.
	file, err := ioutil.TempFile("", p.charmURL.Name)
	if err != nil {
		return func() error { return nil }, errors.Trace(err)
	}

	cleanup := func() error {
		if file.Close(); err != nil {
			return errors.Trace(err)
		}
		// Clean up the downloaded charm - we don't need to cache it in
		// the filesystem as well as in blob storage.
		if err := os.Remove(file.Name()); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	archive, checksum, err := p.store.Download(p.charmURL, file.Name())
	if err != nil {
		return cleanup, errors.Trace(err)
	}

	if err := p.version.Validate(archive.Meta()); err != nil {
		return cleanup, errors.Trace(err)
	}

	// Validate the charm lxd profile once we've downloaded it.
	if err := lxdprofile.ValidateLXDProfile(archive); err != nil && !p.force {
		return cleanup, errors.Annotate(err, "cannot add charm")
	}

	result, resultCleanup, err := p.downloadResult(file.Name(), checksum)
	if err != nil {
		return composeCleanups(cleanup, resultCleanup), errors.Trace(err)
	}

	cleanup = composeCleanups(cleanup, resultCleanup)

	return cleanup, nil
}

// DownloadResult returns the result from the download.
type DownloadResult struct {
	Archive *os.File
	Sha     string
	Size    int64
}

func (p *Procedure) downloadResult(file string, checksum Checksum) (DownloadResult, func() error, error) {
	// Open it and calculate the SHA256 hash.
	tar, err := os.Open(file)
	if err != nil {
		return DownloadResult{}, func() error { return nil }, errors.Annotate(err, "cannot read downloaded charm")
	}

	sha, size, err := utils.ReadSHA256(tar)
	if err != nil {
		return DownloadResult{}, tar.Close, errors.Annotate(err, "cannot calculate SHA256 hash of charm")
	}

	if !checksum(sha) {
		return DownloadResult{}, tar.Close, errors.Annotate(err, "invalid download checksum")
	}

	if _, err := tar.Seek(0, 0); err != nil {
		return DownloadResult{}, tar.Close, errors.Annotatef(err, "can not reset charm archive")
	}

	return DownloadResult{
		Archive: tar,
		Sha:     sha,
		Size:    size,
	}, tar.Close, nil
}

// StoreCharmStore defines a type for interacting with the charm store.
type StoreCharmStore struct {
	charmRepo charmrepo.Interface
}

// Validate checks to ensure that the schema is valid for the store.
func (StoreCharmStore) Validate(curl *charm.URL) error {
	if charm.CharmStore != charm.Schema(curl.Schema) {
		return errors.Errorf("only charm store charm URLs are supported, with cs: schema")
	}
	return nil
}

// Download the charm from the charm store.
func (s StoreCharmStore) Download(curl *charm.URL, file string) (StoreCharm, Checksum, error) {
	archive, err := s.charmRepo.Get(curl, file)
	if err != nil {
		if cause := errors.Cause(err); httpbakery.IsDischargeError(cause) || httpbakery.IsInteractionError(cause) {
			return nil, nil, errors.NewUnauthorized(err, "")
		}
		return nil, nil, errors.Trace(err)
	}
	// Ignore the checksum for charm store, as there isn't any information
	// available to us to perform the downloaded checksum.
	return makeStoreCharmShim(archive), func(string) bool {
		return true
	}, nil
}

// StoreCharmHub defines a type for interacting with the charm hub.
type StoreCharmHub struct{}

// Validate checks to ensure that the schema is valid for the store.
func (StoreCharmHub) Validate(curl *charm.URL) error {
	if charm.CharmStore != charm.Schema(curl.Schema) {
		return errors.Errorf("only charm hub charm URLs are supported")
	}
	return nil
}

// Download the charm from the charm hub.
func (StoreCharmHub) Download(curl *charm.URL, file string) (StoreCharm, Checksum, error) {
	return nil, nil, nil
}

// storeCharmShim massages a *charm.CharmArchive into a LXDProfiler
// inside of the core package.
type storeCharmShim struct {
	archive *charm.CharmArchive
}

func makeStoreCharmShim(archive *charm.CharmArchive) storeCharmShim {
	return storeCharmShim{
		archive: archive,
	}
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p storeCharmShim) LXDProfile() lxdprofile.LXDProfile {
	if p.archive == nil {
		return nil
	}

	profile := p.archive.LXDProfile()
	if profile == nil {
		return nil
	}
	return profile
}

// Meta returns the meta data of an archive.
func (p storeCharmShim) Meta() *charm.Meta {
	return p.archive.Meta()
}

func composeCleanups(fns ...func() error) func() error {
	return func() error {
		for _, f := range fns {
			if err := f(); err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	}
}
