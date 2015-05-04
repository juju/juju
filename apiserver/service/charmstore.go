// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/charmrepo"
	"gopkg.in/juju/charmstore.v4/csclient"
	"gopkg.in/macaroon-bakery.v0/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// TODO - we really want to avoid this, which we can do by refactoring code requiring this
// to use interfaces.
// NewCharmStore instantiates a new charm store repository.
// It is defined at top level for testing purposes.
var NewCharmStore = charmrepo.NewCharmStore

// AddCharmWithAuthorization adds the given charm URL (which must include revision) to
// the environment, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm().
//
// The authorization macaroon, args.CharmStoreMacaroon, may be
// omitted, in which case this call is equivalent to AddCharm.
func AddCharmWithAuthorization(st *state.State, args params.AddCharmWithAuthorization) error {
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

	// Get the charm and its information from the store.
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return err
	}
	csURL, err := url.Parse(csclient.ServerURL)
	if err != nil {
		return err
	}
	csParams := charmrepo.NewCharmStoreParams{
		URL:        csURL.String(),
		HTTPClient: httpbakery.NewHTTPClient(),
	}
	if args.CharmStoreMacaroon != nil {
		// Set the provided charmstore authorizing macaroon
		// as a cookie in the HTTP client.
		// TODO discharge any third party caveats in the macaroon.
		ms := []*macaroon.Macaroon{args.CharmStoreMacaroon}
		httpbakery.SetCookie(csParams.HTTPClient.Jar, csURL, ms)
	}
	repo := config.SpecializeCharmRepo(
		NewCharmStore(csParams),
		envConfig,
	)
	downloadedCharm, err := repo.Get(charmURL)
	if err != nil {
		cause := errors.Cause(err)
		if httpbakery.IsDischargeError(cause) || httpbakery.IsInteractionError(cause) {
			return errors.NewUnauthorized(err, "")
		}
		return errors.Trace(err)
	}

	// Open it and calculate the SHA256 hash.
	downloadedBundle, ok := downloadedCharm.(*charm.CharmArchive)
	if !ok {
		return errors.Errorf("expected a charm archive, got %T", downloadedCharm)
	}
	archive, err := os.Open(downloadedBundle.Path)
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

	// Store the charm archive in environment storage.
	return StoreCharmArchive(
		st,
		charmURL,
		downloadedCharm,
		archive,
		size,
		bundleSHA256,
	)
}

// StoreCharmArchive stores a charm archive in environment storage.
func StoreCharmArchive(st *state.State, curl *charm.URL, ch charm.Charm, r io.Reader, size int64, sha256 string) error {
	storage := newStateStorage(st.EnvironUUID(), st.MongoSession())
	storagePath, err := charmArchiveStoragePath(curl)
	if err != nil {
		return errors.Annotate(err, "cannot generate charm archive name")
	}
	if err := storage.Put(storagePath, r, size); err != nil {
		return errors.Annotate(err, "cannot add charm to storage")
	}

	// Now update the charm data in state and mark it as no longer pending.
	_, err = st.UpdateUploadedCharm(ch, curl, storagePath, sha256)
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
func ResolveCharms(st *state.State, args params.ResolveCharms) (params.ResolveCharmResults, error) {
	var results params.ResolveCharmResults

	envConfig, err := st.EnvironConfig()
	if err != nil {
		return params.ResolveCharmResults{}, err
	}
	repo := config.SpecializeCharmRepo(
		NewCharmStore(charmrepo.NewCharmStoreParams{}),
		envConfig)

	for _, ref := range args.References {
		result := params.ResolveCharmResult{}
		curl, err := resolveCharm(&ref, repo)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.URL = curl
		}
		results.URLs = append(results.URLs, result)
	}
	return results, nil
}

func resolveCharm(ref *charm.Reference, repo charmrepo.Interface) (*charm.URL, error) {
	if ref.Schema != "cs" {
		return nil, fmt.Errorf("only charm store charm references are supported, with cs: schema")
	}

	// Resolve the charm location with the repository.
	curl, err := repo.Resolve(ref)
	if err != nil {
		return nil, err
	}
	return curl.WithRevision(ref.Revision), nil
}
