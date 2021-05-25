// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	"github.com/juju/charmrepo/v6/csclient"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/utils/v2"
	"github.com/juju/version/v2"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/storage"
	jujuversion "github.com/juju/juju/version"
)

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
	PrepareCharmUpload(curl *charm.URL) (StateCharm, error)
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

// Repository represents the necessary methods to resolve and download
// charms from a repository where they reside.
type Repository interface {
	// FindDownloadURL returns a url from which a charm can be downloaded
	// based on the given charm url and charm origin.  A charm origin
	// updated with the ID and hash for the download is also returned.
	FindDownloadURL(curl *charm.URL, origin corecharm.Origin) (*url.URL, corecharm.Origin, error)

	// DownloadCharm reads the charm referenced by curl or downloadURL into
	// a file with the given path, which will be created if needed. Note
	// that the path's parent directory must already exist.
	DownloadCharm(resourceURL string, archivePath string) (*charm.CharmArchive, error)

	// Resolve a canonical URL for retrieving the charm includes the most
	// current revision, if none was provided and a slice  of series supported
	// by this charm.
	Resolve(ref *charm.URL) (canonRef *charm.URL, supportedSeries []string, err error)
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
func AddCharmWithAuthorizationAndRepo(st State, args params.AddCharmWithAuthorization, repoFn func() (Repository, error)) error {
	// Get the repo from the constructor
	repo, err := repoFn()
	if err != nil {
		return errors.Trace(err)
	}

	// TODO (stickupkid): This should be abstracted out in the future to
	// accommodate the charmhub adapter.
	strategy, err := corecharm.DownloadFromCharmStore(logger.Child("strategy"), repo, args.URL, args.Force)
	if err != nil {
		return errors.Trace(err)
	}

	// Validate the strategy before running the download procedure.
	if err := strategy.Validate(); err != nil {
		return errors.Trace(err)
	}

	defer func() {
		// Ensure we sign up any required clean ups.
		_ = strategy.Finish()
	}()

	// Run the strategy.
	result, alreadyExists, _, err := strategy.Run(makeCharmStateShim(st), versionValidator{}, corecharm.Origin{})
	if err != nil {
		return errors.Trace(err)
	} else if alreadyExists {
		// Nothing to do here, as it already exists in state.
		return nil
	}

	ca := CharmArchive{
		ID:           strategy.CharmURL(),
		Charm:        result.Charm,
		Data:         result.Data,
		Size:         result.Size,
		SHA256:       result.SHA256,
		CharmVersion: result.Charm.Version(),
	}

	if args.CharmStoreMacaroon != nil {
		ca.Macaroon = macaroon.Slice{args.CharmStoreMacaroon}
	}

	// Store the charm archive in environment storage.
	return StoreCharmArchive(st, ca)
}

type versionValidator struct{}

func (versionValidator) Validate(meta *charm.Meta) error {
	minver := meta.MinJujuVersion
	return jujuversion.CheckJujuMinVersion(minver, jujuversion.Current)
}

type charmStateShim struct {
	st State
}

func makeCharmStateShim(st State) charmStateShim {
	return charmStateShim{
		st: st,
	}
}

func (s charmStateShim) PrepareCharmUpload(curl *charm.URL) (corecharm.StateCharm, error) {
	return s.st.PrepareCharmUpload(curl)
}

// AddCharmWithAuthorization adds the given charm URL (which must include revision) to
// the environment, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm().
//
// The authorization macaroon, args.CharmStoreMacaroon, may be
// omitted, in which case this call is equivalent to AddCharm.
//
// NOTE: AddCharmWithAuthorization is deprecated as of juju 2.9 and charms
// facade version 3. Please discontinue use and move to the charms facade
// version.
//
// TODO: remove in juju 3.0
func AddCharmWithAuthorization(st State, args params.AddCharmWithAuthorization, openCSRepo OpenCSRepoFunc) error {
	return AddCharmWithAuthorizationAndRepo(st, args, func() (Repository, error) {
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

type OpenCSRepoFunc func(args OpenCSRepoParams) (Repository, error)

type OpenCSRepoParams struct {
	CSURL              string
	Channel            string
	CharmStoreMacaroon *macaroon.Macaroon
}

var OpenCSRepo = func(args OpenCSRepoParams) (Repository, error) {
	csClient, err := openCSClient(args)
	if err != nil {
		return nil, err
	}
	repo := charmrepo.NewCharmStoreFromClient(csClient)
	return &charmRepoShim{repo}, nil
}

func openCSClient(args OpenCSRepoParams) (*csclient.Client, error) {
	csURL, err := url.Parse(args.CSURL)
	if err != nil {
		return nil, err
	}
	csParams := csclient.Params{
		URL:            csURL.String(),
		BakeryClient:   httpbakery.NewClient(),
		UserAgentValue: jujuversion.UserAgentVersion,
	}

	if args.CharmStoreMacaroon != nil {
		// Set the provided charmstore authorizing macaroon
		// as a cookie in the HTTP client.
		// TODO(cmars) discharge any third party caveats in the macaroon.
		ms := []*macaroon.Macaroon{args.CharmStoreMacaroon}
		_ = httpbakery.SetCookie(csParams.BakeryClient.Jar, csURL, charmstore.MacaroonNamespace, ms)
	}
	csClient := csclient.New(csParams)
	channel := csparams.Channel(args.Channel)
	if channel != csparams.NoChannel {
		csClient = csClient.WithChannel(channel)
	}
	return csClient, nil
}

// charmRepoShim helps a CharmRepo *client to a fit the local
// Repository interface.
type charmRepoShim struct {
	charmStore *charmrepo.CharmStore
}

// DownloadCharm calls the charmrepo Get method to return a charm archive.
// It requires a charm url and an archive path to, the url url is ignored
// in this case.
func (c *charmRepoShim) DownloadCharm(resourceURL string, archivePath string) (*charm.CharmArchive, error) {
	curl, err := charm.ParseURL(resourceURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.charmStore.Get(curl, archivePath)
}

// FindDownloadURL is a placeholder required to implement the
// Repository interface.
func (c *charmRepoShim) FindDownloadURL(_ *charm.URL, origin corecharm.Origin) (*url.URL, corecharm.Origin, error) {
	return nil, origin, nil
}

// Resolve calls the charmrepo Resolve method to return a resolved charm url
// and a slice of supported series.
func (c *charmRepoShim) Resolve(ref *charm.URL) (*charm.URL, []string, error) {
	return c.charmStore.Resolve(ref)
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
//
// TODO: (hml) 2020-09-01
// Move use of this function to the charms facade.  A private version
// is currently in use there.  It should be made public.
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
		alreadyUploaded := err == stateerrors.ErrCharmRevisionAlreadyModified ||
			errors.Cause(err) == stateerrors.ErrCharmRevisionAlreadyModified ||
			stateerrors.IsCharmAlreadyUploadedError(err)
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

// ResolveCharms resolves the best available charm URLs with series, for charm
// locations without a series specified.
//
// NOTE: ResolveCharms is deprecated as of juju 2.9 and charms facade
// version 3. Please discontinue use and move to the charms facade version.
//
// TODO: remove in juju 3.0
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

func resolveCharm(ref *charm.URL, repo Repository) (*charm.URL, error) {
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

func (s csStateShim) PrepareCharmUpload(curl *charm.URL) (StateCharm, error) {
	charm, err := s.State.PrepareCharmUpload(curl)
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
