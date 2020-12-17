// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/v2/series"
	"github.com/juju/utils/v2"
	"github.com/kr/pretty"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/core/arch"
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

// Logger defines the logging methods that the package uses.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(string, ...interface{})
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})

	Child(name string) Logger
}

type loggerShim struct {
	loggo.Logger
}

func (l *loggerShim) Child(name string) Logger {
	return &loggerShim{l.Logger.Child(name)}
}

// Store defines the store for which the charm is being downloaded from.
type Store interface {
	// Validate checks to ensure that the charm URL is valid for the store.
	Validate(*charm.URL) error
	// Download a charm from the store using the charm URL.
	Download(*charm.URL, string, Origin) (StoreCharm, ChecksumCheckFn, Origin, error)
	// DownloadOrigin returns an origin with the id and hash, without
	// downloading the charm.
	DownloadOrigin(curl *charm.URL, origin Origin) (Origin, error)
}

// ChecksumCheckFn defines a function for running checksums against.
type ChecksumCheckFn func(string) bool

// AlwaysMatchChecksum will always return true and is an effective no-op.
func AlwaysMatchChecksum(string) bool {
	return true
}

// MatchChecksum validates a checksum against another checksum.
func MatchChecksum(hash string) ChecksumCheckFn {
	return func(other string) bool {
		return hash == other
	}
}

// JujuVersionValidator validates the version of Juju against the charm meta
// data.
// The charm.Meta contains a MinJujuVersion and we can use that to check that
// for a valid charm.
type JujuVersionValidator interface {
	// Check the version is valid for the given Juju version.
	Validate(*charm.Meta) error
}

// Strategy defines a procedure for adding a charm to state.
type Strategy struct {
	charmURL   *charm.URL
	store      Store
	force      bool
	deferFuncs []func() error
	logger     Logger
}

// DownloadRepo defines methods required for the repo to download a charm.
type DownloadRepo interface {
	DownloadCharm(resourceURL, archivePath string) (*charm.CharmArchive, error)
	FindDownloadURL(*charm.URL, Origin) (*url.URL, Origin, error)
}

// DownloadFromCharmStore will creates a procedure to install a charm from the
// charm store.
func DownloadFromCharmStore(logger loggo.Logger, repository DownloadRepo, url string, force bool) (*Strategy, error) {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	log := &loggerShim{logger}
	return &Strategy{
		charmURL: curl,
		store: StoreCharmStore{
			repository: repository,
			logger:     log.Child("charmstore"),
		},
		force:  force,
		logger: log,
	}, nil
}

// DownloadFromCharmHub will creates a procedure to install a charm from the
// charm hub.
func DownloadFromCharmHub(logger loggo.Logger, repository DownloadRepo, curl string, force bool) (*Strategy, error) {
	churl, err := charm.ParseURL(curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	log := &loggerShim{logger}
	return &Strategy{
		charmURL: churl,
		store: StoreCharmHub{
			repository: repository,
			logger:     log.Child("charmhub"),
		},
		force:  force,
		logger: log,
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

// Run the download procedure against the supplied store adapter.
// Includes downloading the blob to a temp file and validating the contents
// of the charm.Meta and LXD profile data.
func (p *Strategy) Run(state State, version JujuVersionValidator, origin Origin) (DownloadResult, bool, Origin, error) {
	p.logger.Tracef("Run %+v", origin)
	ch, err := state.PrepareCharmUpload(p.charmURL)
	if err != nil {
		return DownloadResult{}, false, origin, errors.Trace(err)
	}

	seriesOrigin := origin
	seriesOrigin.Platform, err = p.normalizePlatform(origin.Platform)
	if err != nil {
		return DownloadResult{}, false, origin, errors.Trace(err)
	}

	// Charm is already in state, so we can exit out early.
	if ch.IsUploaded() {
		origin, err := p.store.DownloadOrigin(p.charmURL, seriesOrigin)
		if err != nil {
			return DownloadResult{}, false, Origin{}, errors.Trace(err)
		}

		p.logger.Debugf("Reusing charm: already uploaded to controller with origin %v", origin)
		return DownloadResult{}, true, origin, nil
	}

	p.logger.Debugf("Downloading charm %q: %v", p.charmURL, seriesOrigin)

	// Get the charm and its information from the store.
	file, err := ioutil.TempFile("", p.charmURL.Name)
	if err != nil {
		return DownloadResult{}, false, Origin{}, errors.Trace(err)
	}

	p.deferFunc(func() error {
		_ = file.Close()
		return nil
	})
	p.deferFunc(func() error {
		_ = os.Remove(file.Name())
		return nil
	})

	archive, checksum, downloadOrigin, err := p.store.Download(p.charmURL, file.Name(), seriesOrigin)
	if err != nil {
		return DownloadResult{}, false, Origin{}, errors.Trace(err)
	}

	if err := version.Validate(archive.Meta()); err != nil {
		return DownloadResult{}, false, Origin{}, errors.Trace(err)
	}

	// Validate the charm lxd profile once we've downloaded it.
	if err := lxdprofile.ValidateLXDProfile(makeStoreCharmLXDProfiler(archive)); err != nil && !p.force {
		return DownloadResult{}, false, Origin{}, errors.Annotate(err, "cannot add charm")
	}

	result, err := p.downloadResult(file.Name(), checksum)
	if err != nil {
		return DownloadResult{}, false, Origin{}, errors.Trace(err)
	}

	return DownloadResult{
		Charm:  archive,
		Data:   result.Data,
		Size:   result.Size,
		SHA256: result.SHA256,
	}, false, downloadOrigin, nil
}

// Finish will attempt to close out the download procedure. Cleaning up any
// outstanding function tasks.
// If the function task errors out, then it prevents any further task from being
// executed.
// It is expected that each task correctly handles the error if they want to
// continue with finishing all the tasks.
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

func (p *Strategy) normalizePlatform(platform Platform) (Platform, error) {
	os := platform.OS
	if platform.Series != "" {
		sys, err := series.GetOSFromSeries(platform.Series)
		if err != nil {
			return Platform{}, errors.Trace(err)
		}
		// Values passed to the api are case sensitive: ubuntu succeeds and
		// Ubuntu returns `"code": "revision-not-found"`
		os = strings.ToLower(sys.String())
	}
	arc := platform.Architecture
	if platform.Architecture == "" || platform.Architecture == "all" {
		p.logger.Warningf("Received charm Architecture: %q, changing to %q, for charm %q", platform.Architecture, arch.DefaultArchitecture, p.charmURL)
		arc = arch.DefaultArchitecture
	}

	return Platform{
		Architecture: arc,
		OS:           os,
		Series:       platform.Series,
	}, nil
}

func (p *Strategy) downloadResult(file string, checksum ChecksumCheckFn) (DownloadResult, error) {
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

	p.logger.Tracef("downloadResult(%q) sha: %q, size: %d", tar.Name(), sha, size)

	if !checksum(sha) {
		return DownloadResult{}, errors.NotValidf("download checksum")
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
	repository DownloadRepo
	logger     Logger
}

// Validate checks to ensure that the schema is valid for the store.
func (StoreCharmStore) Validate(curl *charm.URL) error {
	if charm.CharmStore != charm.Schema(curl.Schema) {
		return errors.Errorf("only charm store charm URLs are supported, with cs: schema")
	}
	return nil
}

// Download the charm from the charm store.
func (s StoreCharmStore) Download(curl *charm.URL, file string, origin Origin) (StoreCharm, ChecksumCheckFn, Origin, error) {
	s.logger.Tracef("Download(%s) %s", curl)
	archive, err := s.repository.DownloadCharm(curl.String(), file)
	if err != nil {
		if cause := errors.Cause(err); httpbakery.IsDischargeError(cause) || httpbakery.IsInteractionError(cause) {
			return nil, nil, origin, errors.NewUnauthorized(err, "")
		}
		return nil, nil, origin, errors.Trace(err)
	}
	// Ignore the checksum for charm store, as there isn't any information
	// available to us to perform the downloaded checksum.
	return newStoreCharmShim(archive), AlwaysMatchChecksum, origin, nil
}

// DownloadOrigin returns the same origin provided.  This operation is required for CharmHub.
func (s StoreCharmStore) DownloadOrigin(_ *charm.URL, origin Origin) (Origin, error) {
	return origin, nil
}

// StoreCharmHub defines a type for interacting with the charm hub.
type StoreCharmHub struct {
	repository DownloadRepo
	platform   Platform
	logger     Logger
}

// Validate checks to ensure that the schema is valid for the store.
func (StoreCharmHub) Validate(curl *charm.URL) error {
	if charm.CharmHub != charm.Schema(curl.Schema) {
		return errors.Errorf("only charm hub charm URLs are supported")
	}
	return nil
}

// Download the charm from the charm hub.
func (s StoreCharmHub) Download(curl *charm.URL, file string, origin Origin) (StoreCharm, ChecksumCheckFn, Origin, error) {
	s.logger.Tracef("Download(%s) %s", curl)
	repositoryURL, downloadOrigin, err := s.repository.FindDownloadURL(curl, origin)
	if err != nil {
		return nil, nil, downloadOrigin, errors.Trace(err)
	}
	archive, err := s.repository.DownloadCharm(repositoryURL.String(), file)
	if err != nil {
		return nil, nil, downloadOrigin, errors.Trace(err)
	}
	return newStoreCharmShim(archive), MatchChecksum(downloadOrigin.Hash), downloadOrigin, nil
}

// DownloadOrigin returns an origin with the id and hash, without
// downloading the charm.
func (s StoreCharmHub) DownloadOrigin(curl *charm.URL, origin Origin) (Origin, error) {
	s.logger.Tracef("DownloadOrigin(%s) %s", curl, pretty.Sprint(origin))
	_, downloadOrigin, err := s.repository.FindDownloadURL(curl, origin)
	return downloadOrigin, errors.Trace(err)
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
