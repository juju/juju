// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v4"
	"gopkg.in/juju/charmrepo.v4/csclient"
	csparams "gopkg.in/juju/charmrepo.v4/csclient/params"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	jujuversion "github.com/juju/juju/version"
)

//go:generate mockgen -package mocks -destination mocks/storage_mock.go github.com/juju/juju/state/storage Storage
//go:generate mockgen -package mocks -destination mocks/interface_mock.go gopkg.in/juju/charmrepo.v4 Interface
//go:generate mockgen -package mocks -destination mocks/charm_mock.go github.com/juju/juju/apiserver/facades/client/application StateCharm
//go:generate mockgen -package mocks -destination mocks/model_mock.go github.com/juju/juju/apiserver/facades/client/application StateModel
//go:generate mockgen -package mocks -destination mocks/charmstore_mock.go github.com/juju/juju/apiserver/facades/client/application State

// TODO - we really want to avoid this, which we can do by refactoring code requiring this
// to use interfaces.

var newStateStorage = storage.NewStorage

// StateCharm represents a Charm from the state package
type StateCharm interface {
	IsUploaded() bool
}

// StateModel represents a Model from the state package
type StateModel interface {
	ModelConfig() (*config.Config, error)
}

// CharmState represents directives for accessing charm methods
type CharmState interface {
	UpdateUploadedCharm(info state.CharmInfo) (*state.Charm, error)
	PrepareStoreCharmUpload(curl *charm.URL) (StateCharm, error)
}

// ModelState represents methods for accessing model definitions
type ModelState interface {
	Model() (StateModel, error)
	ModelUUID() string
}

// ControllerState represents information defined for accessing controller
// configuration
type ControllerState interface {
	ControllerConfig() (controller.Config, error)
}

// State represents the access patterns for the charm store methods.
type State interface {
	CharmState
	ModelState
	ControllerState
	state.MongoSessioner
}

// AddCharmWithAuthorizationAndRepo adds the given charm URL (which must include
// revision) to the environment, if it does not exist yet.
// Local charms are not supported, only charm store URLs.
// See also AddLocalCharm().
// Additionally a Repo (See charmrepo.Interface) function factory can be
// provided to help with overriding the source of downloading charms. The main
// benefit of this indirection is to help with testing (mocking)
//
// The authorization macaroon, args.CharmStoreMacaroon, may be
// omitted, in which case this call is equivalent to AddCharm.
func AddCharmWithAuthorizationAndRepo(st State, args params.AddCharmWithAuthorization, repoFn func() (charmrepo.Interface, error)) error {
	charmURL, err := charm.ParseURL(args.URL)
	if err != nil {
		return err
	}
	if charmURL.Schema != "cs" {
		return fmt.Errorf("only charm store charm URLs are supported, with cs: schema")
	}
	if charmURL.Revision < 0 {
		return fmt.Errorf("charm URL must include revision")
	}

	// First, check if a pending or a real charm exists in state.
	stateCharm, err := st.PrepareStoreCharmUpload(charmURL)
	if err != nil {
		return err
	}
	if stateCharm.IsUploaded() {
		// Charm already in state (it was uploaded already).
		return nil
	}

	// Get the repo from the constructor
	repo, err := repoFn()
	if err != nil {
		return errors.Trace(err)
	}

	// Get the charm and its information from the store.
	f, err := ioutil.TempFile("", charmURL.Name)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()
	downloadedCharm, err := repo.Get(charmURL, f.Name())
	if err != nil {
		cause := errors.Cause(err)
		if httpbakery.IsDischargeError(cause) || httpbakery.IsInteractionError(cause) {
			return errors.NewUnauthorized(err, "")
		}
		return errors.Trace(err)
	}

	if err := jujuversion.CheckJujuMinVersion(downloadedCharm.Meta().MinJujuVersion, jujuversion.Current); err != nil {
		return errors.Trace(err)
	}

	// Validate the charm lxd profile once we've downloaded it.
	if err := lxdprofile.ValidateLXDProfile(lxdCharmArchiveProfiler{
		CharmArchive: downloadedCharm,
	}); err != nil {
		if !args.Force {
			return errors.Annotate(err, "cannot add charm")
		}
	}

	// Clean up the downloaded charm - we don't need to cache it in
	// the filesystem as well as in blob storage.
	defer os.Remove(downloadedCharm.Path)

	// Open it and calculate the SHA256 hash.
	archive, err := os.Open(downloadedCharm.Path)
	if err != nil {
		return errors.Annotate(err, "cannot read downloaded charm")
	}
	defer archive.Close()
	bundleSHA256, size, err := utils.ReadSHA256(archive)
	if err != nil {
		return errors.Annotate(err, "cannot calculate SHA256 hash of charm")
	}
	if _, err := archive.Seek(0, 0); err != nil {
		return errors.Annotate(err, "cannot rewind charm archive")
	}

	ca := CharmArchive{
		ID:           charmURL,
		Charm:        downloadedCharm,
		Data:         archive,
		Size:         size,
		SHA256:       bundleSHA256,
		CharmVersion: downloadedCharm.Version(),
	}
	if args.CharmStoreMacaroon != nil {
		ca.Macaroon = macaroon.Slice{args.CharmStoreMacaroon}
	}

	// Store the charm archive in environment storage.
	return StoreCharmArchive(st, ca)
}

// AddCharmWithAuthorization adds the given charm URL (which must include revision) to
// the environment, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm().
//
// The authorization macaroon, args.CharmStoreMacaroon, may be
// omitted, in which case this call is equivalent to AddCharm.
func AddCharmWithAuthorization(st State, args params.AddCharmWithAuthorization, openCSRepo OpenCSRepoFunc) error {
	return AddCharmWithAuthorizationAndRepo(st, args, func() (charmrepo.Interface, error) {
		// determine which charmstore api url to use.
		controllerCfg, err := st.ControllerConfig()
		if err != nil {
			return nil, err
		}

		return openCSRepo(OpenCSRepoParams{
			CSURL:              controllerCfg.CharmStoreURL(),
			Channel:            args.Channel,
			CharmStoreMacaroon: args.CharmStoreMacaroon,
		})
	})
}

type OpenCSRepoFunc func(args OpenCSRepoParams) (charmrepo.Interface, error)

type OpenCSRepoParams struct {
	CSURL              string
	Channel            string
	CharmStoreMacaroon *macaroon.Macaroon
}

var OpenCSRepo = func(args OpenCSRepoParams) (charmrepo.Interface, error) {
	csClient, err := openCSClient(args)
	if err != nil {
		return nil, err
	}
	repo := charmrepo.NewCharmStoreFromClient(csClient)
	return repo, nil
}

func openCSClient(args OpenCSRepoParams) (*csclient.Client, error) {
	csURL, err := url.Parse(args.CSURL)
	if err != nil {
		return nil, err
	}
	csParams := csclient.Params{
		URL:          csURL.String(),
		BakeryClient: httpbakery.NewClient(),
	}

	if args.CharmStoreMacaroon != nil {
		// Set the provided charmstore authorizing macaroon
		// as a cookie in the HTTP client.
		// TODO(cmars) discharge any third party caveats in the macaroon.
		ms := []*macaroon.Macaroon{args.CharmStoreMacaroon}
		httpbakery.SetCookie(csParams.BakeryClient.Jar, csURL, charmstore.MacaroonNamespace, ms)
	}
	csClient := csclient.New(csParams)
	channel := csparams.Channel(args.Channel)
	if channel != csparams.NoChannel {
		csClient = csClient.WithChannel(channel)
	}
	return csClient, nil
}

func checkCAASMinVersion(ch charm.Charm, caasVersion *version.Number) (err error) {
	// check caas min version.
	charmDeployment := ch.Meta().Deployment
	if caasVersion == nil || charmDeployment == nil || charmDeployment.MinVersion == "" {
		return nil
	}
	if len(strings.Split(charmDeployment.MinVersion, ".")) == 2 {
		// append build number if it's not specified.
		charmDeployment.MinVersion += ".0"
	}
	minver, err := version.Parse(charmDeployment.MinVersion)
	if err != nil {
		return errors.Trace(err)
	}
	if minver != version.Zero && minver.Compare(*caasVersion) > 0 {
		return errors.NewNotValid(nil, fmt.Sprintf(
			"charm requires a minimum k8s version of %v but the cluster only runs version %v",
			minver, caasVersion,
		))
	}
	return nil
}

// CharmArchive is the data that needs to be stored for a charm archive in
// state.
type CharmArchive struct {
	// ID is the charm URL for which we're storing the archive.
	ID *charm.URL

	// Charm is the metadata about the charm for the archive.
	Charm charm.Charm

	// Data contains the bytes of the archive.
	Data io.Reader

	// Size is the number of bytes in Data.
	Size int64

	// SHA256 is the hash of the bytes in Data.
	SHA256 string

	// Macaroon is the authorization macaroon for accessing the charmstore.
	Macaroon macaroon.Slice

	// Charm Version contains semantic version of charm, typically the output of git describe.
	CharmVersion string
}

// StoreCharmArchive stores a charm archive in environment storage.
func StoreCharmArchive(st State, archive CharmArchive) error {
	storage := newStateStorage(st.ModelUUID(), st.MongoSession())
	storagePath, err := charmArchiveStoragePath(archive.ID)
	if err != nil {
		return errors.Annotate(err, "cannot generate charm archive name")
	}
	if err := storage.Put(storagePath, archive.Data, archive.Size); err != nil {
		return errors.Annotate(err, "cannot add charm to storage")
	}

	info := state.CharmInfo{
		Charm:       archive.Charm,
		ID:          archive.ID,
		StoragePath: storagePath,
		SHA256:      archive.SHA256,
		Macaroon:    archive.Macaroon,
		Version:     archive.CharmVersion,
	}

	// Now update the charm data in state and mark it as no longer pending.
	_, err = st.UpdateUploadedCharm(info)
	if err != nil {
		alreadyUploaded := err == state.ErrCharmRevisionAlreadyModified ||
			errors.Cause(err) == state.ErrCharmRevisionAlreadyModified ||
			state.IsCharmAlreadyUploadedError(err)
		if err := storage.Remove(storagePath); err != nil {
			if alreadyUploaded {
				logger.Errorf("cannot remove duplicated charm archive from storage: %v", err)
			} else {
				logger.Errorf("cannot remove unsuccessfully recorded charm archive from storage: %v", err)
			}
		}
		if alreadyUploaded {
			// Somebody else managed to upload and update the charm in
			// state before us. This is not an error.
			return nil
		}
		return errors.Trace(err)
	}
	return nil
}

// charmArchiveStoragePath returns a string that is suitable as a
// storage path, using a random UUID to avoid colliding with concurrent
// uploads.
func charmArchiveStoragePath(curl *charm.URL) (string, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("charms/%s-%s", curl.String(), uuid), nil
}

// ResolveCharm resolves the best available charm URLs with series, for charm
// locations without a series specified.
func ResolveCharms(st State, args params.ResolveCharms, openCSRepo OpenCSRepoFunc) (params.ResolveCharmResults, error) {
	var results params.ResolveCharmResults

	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return params.ResolveCharmResults{}, errors.Trace(err)
	}
	repo, err := openCSRepo(OpenCSRepoParams{
		CSURL: controllerCfg.CharmStoreURL(),
	})
	if err != nil {
		return params.ResolveCharmResults{}, errors.Trace(err)
	}

	for _, ref := range args.References {
		result := params.ResolveCharmResult{}
		curl, err := charm.ParseURL(ref)
		if err != nil {
			result.Error = err.Error()
		} else {
			curl, err := resolveCharm(curl, repo)
			if err != nil {
				result.Error = err.Error()
			} else {
				result.URL = curl.String()
			}
		}
		results.URLs = append(results.URLs, result)
	}
	return results, nil
}

func resolveCharm(ref *charm.URL, repo charmrepo.Interface) (*charm.URL, error) {
	if ref.Schema != "cs" {
		return nil, errors.New("only charm store charm references are supported, with cs: schema")
	}

	// Resolve the charm location with the repository.
	resolved, _, err := repo.Resolve(ref)
	if err != nil {
		return nil, err
	}
	if resolved.Series == "" {
		return nil, errors.Errorf("no series found in charm URL %q", resolved)
	}
	return resolved.WithRevision(ref.Revision), nil
}

type csStateShim struct {
	*state.State
}

func NewStateShim(st *state.State) State {
	return csStateShim{
		State: st,
	}
}

func (s csStateShim) PrepareStoreCharmUpload(curl *charm.URL) (StateCharm, error) {
	charm, err := s.State.PrepareStoreCharmUpload(curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return csStateCharmShim{Charm: charm}, nil
}

func (s csStateShim) Model() (StateModel, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return csStateModelShim{Model: model}, nil
}

type csStateCharmShim struct {
	*state.Charm
}

func (s csStateCharmShim) IsUploaded() bool {
	return s.Charm.IsUploaded()
}

type csStateModelShim struct {
	*state.Model
}

func (s csStateModelShim) ModelConfig() (*config.Config, error) {
	return s.Model.ModelConfig()
}

// lxdCharmArchiveProfiler massages a *charm.CharmArchive into a LXDProfiler
// inside of the core package.
type lxdCharmArchiveProfiler struct {
	CharmArchive *charm.CharmArchive
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p lxdCharmArchiveProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.CharmArchive == nil {
		return nil
	}
	profile := p.CharmArchive.LXDProfile()
	if profile == nil {
		return nil
	}
	return profile
}
