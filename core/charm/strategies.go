// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/core/lxdprofile"
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
	charm.Charm
	charm.LXDProfiler
	Version() string
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

// AlwaysChecksum will always return true and is an effective no-op.
func AlwaysChecksum(string) bool {
	return true
}

// VersionValidator validates the version of the store charm.
type VersionValidator interface {
	// Check the version is valid for the given version.
	Validate(*charm.Meta) error
}

// Strategy defines a procedure for adding a charm to state.
type Strategy struct {
	charmURL   *charm.URL
	store      Store
	force      bool
	deferFuncs []func() error
}

// DownloadFromCharmStore will creates a procedure to install a charm from the
// charm store.
func DownloadFromCharmStore(charmRepo charmrepo.Interface, url string, force bool) (*Strategy, error) {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Strategy{
		charmURL: curl,
		store: StoreCharmStore{
			charmRepo: charmRepo,
		},
		force: force,
	}, nil
}

// DownloadFromCharmHub will creates a procedure to install a charm from the
// charm hub.
func DownloadFromCharmHub(url string, force bool) (*Strategy, error) {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Strategy{
		charmURL: curl,
		store:    StoreCharmHub{},
	}, nil
}

// CharmURL returns the strategy URL associated with it.
func (p *Strategy) CharmURL() *charm.URL {
	return p.charmURL
}

// Validate will attempt to validate the requirements for adding a charm to the
// store.
func (p *Strategy) Validate() error {
	if err := p.store.Validate(p.charmURL); err != nil {
		return errors.Trace(err)
	}
	if p.charmURL.Revision < 0 {
		return errors.Errorf("charm URL must include a revision")
	}
	return nil
}

// DownloadResult defines the result from the attempt to install a charm into
// state.
type DownloadResult struct {
	// Charm is the metadata about the charm for the archive.
	Charm StoreCharm

	// Data contains the bytes of the archive.
	Data io.Reader

	// Size is the number of bytes in Data.
	Size int64

	// SHA256 is the hash of the bytes in Data.
	SHA256 string
}

// Run the procedure against the correct store.
func (p *Strategy) Run(state State, version VersionValidator) (DownloadResult, bool, error) {
	charm, err := state.PrepareCharmUpload(p.charmURL)
	if err != nil {
		return DownloadResult{}, false, errors.Trace(err)
	}

	// Charm is already in state, so we can exit out early.
	if charm.IsUploaded() {
		return DownloadResult{}, true, nil
	}

	// Get the charm and its information from the store.
	file, err := ioutil.TempFile("", p.charmURL.Name)
	if err != nil {
		return DownloadResult{}, false, errors.Trace(err)
	}

	p.deferFunc(func() error {
		_ = file.Close()
		return nil
	})
	p.deferFunc(func() error {
		_ = os.Remove(file.Name())
		return nil
	})

	archive, checksum, err := p.store.Download(p.charmURL, file.Name())
	if err != nil {
		return DownloadResult{}, false, errors.Trace(err)
	}

	if err := version.Validate(archive.Meta()); err != nil {
		return DownloadResult{}, false, errors.Trace(err)
	}

	// Validate the charm lxd profile once we've downloaded it.
	if err := lxdprofile.ValidateLXDProfile(makeStoreCharmLXDProfiler(archive)); err != nil && !p.force {
		return DownloadResult{}, false, errors.Annotate(err, "cannot add charm")
	}

	result, err := p.downloadResult(file.Name(), checksum)
	if err != nil {
		return DownloadResult{}, false, errors.Trace(err)
	}

	return DownloadResult{
		Charm:  archive,
		Data:   result.Data,
		Size:   result.Size,
		SHA256: result.SHA256,
	}, false, nil
}

// Finish will attempt to close out the procedure and clean up any outstanding
// tasks.
func (p *Strategy) Finish() error {
	for _, f := range p.deferFuncs {
		if err := f(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (p *Strategy) deferFunc(fn func() error) {
	p.deferFuncs = append(p.deferFuncs, fn)
}

func (p *Strategy) downloadResult(file string, checksum Checksum) (DownloadResult, error) {
	// Open it and calculate the SHA256 hash.
	tar, err := os.Open(file)
	if err != nil {
		return DownloadResult{}, errors.Annotate(err, "cannot read downloaded charm")
	}

	p.deferFunc(func() error {
		_ = tar.Close()
		return nil
	})

	sha, size, err := utils.ReadSHA256(tar)
	if err != nil {
		return DownloadResult{}, errors.Annotate(err, "cannot calculate SHA256 hash of charm")
	}

	if !checksum(sha) {
		return DownloadResult{}, errors.Annotate(err, "invalid download checksum")
	}

	if _, err := tar.Seek(0, 0); err != nil {
		return DownloadResult{}, errors.Annotatef(err, "can not reset charm archive")
	}

	return DownloadResult{
		Data:   tar,
		SHA256: sha,
		Size:   size,
	}, nil
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
	return newStoreCharmShim(archive), AlwaysChecksum, nil
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
	*charm.CharmArchive
}

func newStoreCharmShim(archive *charm.CharmArchive) *storeCharmShim {
	return &storeCharmShim{
		CharmArchive: archive,
	}
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p *storeCharmShim) LXDProfile() *charm.LXDProfile {
	if p.CharmArchive == nil {
		return nil
	}

	profile := p.CharmArchive.LXDProfile()
	if profile == nil {
		return nil
	}
	return profile
}

// storeCharmLXDProfiler massages a *charm.CharmArchive into a LXDProfiler
// inside of the core package.
type storeCharmLXDProfiler struct {
	StoreCharm
}

func makeStoreCharmLXDProfiler(shim StoreCharm) storeCharmLXDProfiler {
	return storeCharmLXDProfiler{
		StoreCharm: shim,
	}
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p storeCharmLXDProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.StoreCharm == nil {
		return nil
	}
	profile := p.StoreCharm.LXDProfile()
	if profile == nil {
		return nil
	}
	return profile
}
