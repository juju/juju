// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"io"
	"regexp"

	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/charm/store"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	internalerrors "github.com/juju/juju/internal/errors"
)

var (
	// charmNameRegExp is a regular expression representing charm name.
	// This is the same one from the names package.
	charmNameSnippet = "[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*"
	charmNameRegExp  = regexp.MustCompile("^" + charmNameSnippet + "$")
)

// CharmState describes retrieval and persistence methods for charms.
type CharmState interface {
	// GetCharmID returns the charm ID by the natural key, for a
	// specific revision and source. If the charm does not exist, a
	// [applicationerrors.CharmNotFound] error is returned.
	GetCharmID(ctx context.Context, name string, revision int, source charm.CharmSource) (corecharm.ID, error)

	// IsControllerCharm returns whether the charm is a controller charm. If the
	// charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	IsControllerCharm(ctx context.Context, id corecharm.ID) (bool, error)

	// IsSubordinateCharm returns whether the charm is a subordinate charm. If
	// the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	IsSubordinateCharm(ctx context.Context, charmID corecharm.ID) (bool, error)

	// SupportsContainers returns whether the charm supports containers. If the
	// charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	SupportsContainers(ctx context.Context, charmID corecharm.ID) (bool, error)

	// GetCharmMetadata returns the metadata for the charm using the charm ID.
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmMetadata(context.Context, corecharm.ID) (charm.Metadata, error)

	// GetCharmMetadataName returns the name for the charm using the charm ID.
	GetCharmMetadataName(context.Context, corecharm.ID) (string, error)

	// GetCharmMetadataDescription returns the description for the charm using
	// the charm ID.
	GetCharmMetadataDescription(context.Context, corecharm.ID) (string, error)

	// GetCharmMetadataStorage returns the storage specification for the charm
	// using the charm ID.
	GetCharmMetadataStorage(context.Context, corecharm.ID) (map[string]charm.Storage, error)

	// GetCharmMetadataResources returns the specifications for the resources for
	// the charm using the charm ID.
	GetCharmMetadataResources(ctx context.Context, id corecharm.ID) (map[string]charm.Resource, error)

	// GetCharmManifest returns the manifest for the charm using the charm ID.
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmManifest(context.Context, corecharm.ID) (charm.Manifest, error)

	// GetCharmActions returns the actions for the charm using the charm ID. If
	// the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmActions(context.Context, corecharm.ID) (charm.Actions, error)

	// GetCharmConfig returns the config for the charm using the charm ID. If
	// the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmConfig(context.Context, corecharm.ID) (charm.Config, error)

	// GetCharmLXDProfile returns the LXD profile along with the revision of the
	// charm using the charm ID. The revision
	//
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmLXDProfile(context.Context, corecharm.ID) ([]byte, charm.Revision, error)

	// GetCharmArchivePath returns the archive storage path for the charm using
	// the charm ID. If the charm does not exist, a
	// [applicationerrors.CharmNotFound] error is returned.
	GetCharmArchivePath(context.Context, corecharm.ID) (string, error)

	// GetCharmArchiveMetadata returns the archive storage path and hash for the
	// charm using the charm ID.
	// If the charm does not exist, a [errors.CharmNotFound] error is returned.
	GetCharmArchiveMetadata(context.Context, corecharm.ID) (archivePath string, hash string, err error)

	// IsCharmAvailable returns whether the charm is available for use. If the
	// charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	IsCharmAvailable(ctx context.Context, charmID corecharm.ID) (bool, error)

	// SetCharmAvailable sets the charm as available for use. If the charm does
	// not exist, a [applicationerrors.CharmNotFound] error is returned.
	SetCharmAvailable(ctx context.Context, charmID corecharm.ID) error

	// GetCharm returns the charm using the charm ID.
	GetCharm(ctx context.Context, id corecharm.ID) (charm.Charm, *charm.DownloadInfo, error)

	// SetCharm persists the charm metadata, actions, config and manifest to
	// state. If the charm requires sequencing, the revision must be set to
	// -1 and the requiredSequencing flag must be set to true. If the charm
	// does not require sequencing, the revision must be set to the desired
	// revision and the requiredSequencing flag must be set to false.
	SetCharm(ctx context.Context, ch charm.Charm, downloadInfo *charm.DownloadInfo, requiresSequencing bool) (corecharm.ID, charm.CharmLocator, error)

	// DeleteCharm removes the charm from the state. If the charm does not
	// exist, a [applicationerrors.CharmNotFound]  error is returned.
	DeleteCharm(ctx context.Context, id corecharm.ID) error

	// ListCharmLocators returns a list of charm locators. The locator allows
	// the reconstruction of the charm URL for the client response.
	ListCharmLocators(ctx context.Context) ([]charm.CharmLocator, error)

	// ListCharmLocatorsByNames returns a list of charm locators for the
	// specified charm names. The locator allows the reconstruction of the charm
	// URL for the client response. If no names are provided, then nothing is
	// returned.
	ListCharmLocatorsByNames(ctx context.Context, names []string) ([]charm.CharmLocator, error)

	// GetCharmDownloadInfo returns the download info for the charm using the
	// charm ID. Returns [applicationerrors.CharmNotFound] if the charm is not
	// found.
	GetCharmDownloadInfo(ctx context.Context, id corecharm.ID) (*charm.DownloadInfo, error)

	// GetAvailableCharmArchiveSHA256 returns the SHA256 hash of the charm
	// archive for the given charm id. If the charm is not available,
	// [applicationerrors.CharmNotResolved] is returned. Returns
	// [applicationerrors.CharmNotFound] if the charm is not found.
	GetAvailableCharmArchiveSHA256(ctx context.Context, id corecharm.ID) (string, error)

	// ResolveMigratingUploadedCharm resolves the charm that is migrating from
	// the uploaded state to the available state. If the charm is not found, a
	// [applicationerrors.CharmNotFound] error is returned.
	ResolveMigratingUploadedCharm(context.Context, corecharm.ID, charm.ResolvedMigratingUploadedCharm) (charm.CharmLocator, error)

	// GetLatestPendingCharmhubCharm returns the latest charm that is pending
	// from the charmhub store. If there are no charms, returns is not found, as
	// [applicationerrors.CharmNotFound]. If there are multiple charms, then the
	// latest created at date is returned first.
	GetLatestPendingCharmhubCharm(ctx context.Context, name string, arch architecture.Architecture) (charm.CharmLocator, error)

	// GetCharmLocatorByCharmID returns a charm locator for the given charm ID.
	// The locator allows the reconstruction of the charm URL for the client
	// response.
	// If the charm does not exist, a [errors.CharmNotFound] error is returned.
	GetCharmLocatorByCharmID(ctx context.Context, id corecharm.ID) (charm.CharmLocator, error)

	// NamespaceForWatchCharm return the namespace used to listen charm changes
	NamespaceForWatchCharm() string
}

// CharmStore defines the interface for storing and retrieving charms archive
// blobs from the underlying storage.
type CharmStore interface {
	// Store the charm at the specified path into the object store. It is
	// expected that the archive already exists at the specified path. If the
	// file isn't found, a [ErrNotFound] is returned.
	Store(ctx context.Context, path string, size int64, hash string) (store.StoreResult, error)

	// StoreFromReader stores the charm from the provided reader into the object
	// store. The caller is expected to remove the temporary file after the
	// call.
	// sha256Prefix is the prefix characters of the SHA256 hash of the charm
	// archive.
	StoreFromReader(ctx context.Context, reader io.Reader, sha256Prefix string) (store.StoreFromReaderResult, store.Digest, error)

	// GetCharm retrieves a ReadCloser for the charm archive at the give path
	// from the underlying storage.
	Get(ctx context.Context, archivePath string) (io.ReadCloser, error)

	// GetBySHA256Prefix retrieves a ReadCloser for a charm archive who's SHA256
	// hash starts with the provided prefix.
	GetBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, error)
}

// Deprecated: This method is only here until we come back and use the charm
// locator in the db queries everywhere. For now it's just scaffolding.
func (s *Service) getCharmID(ctx context.Context, args charm.GetCharmArgs) (corecharm.ID, error) {
	if !isValidCharmName(args.Name) {
		return "", applicationerrors.CharmNameNotValid
	}

	// Validate the source, it can only be charmhub or local.
	if args.Source != charm.CharmHubSource && args.Source != charm.LocalSource {
		return "", applicationerrors.CharmSourceNotValid
	}

	if rev := args.Revision; rev != nil && *rev >= 0 {
		return s.st.GetCharmID(ctx, args.Name, *rev, args.Source)
	}

	return "", applicationerrors.CharmNotFound
}

// IsControllerCharm returns whether the charm is a controller charm. This will
// return true if the charm is a controller charm, and false otherwise.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) IsControllerCharm(ctx context.Context, locator charm.CharmLocator) (bool, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return false, errors.Trace(err)
	}
	b, err := s.st.IsControllerCharm(ctx, id)
	if err != nil {
		return false, errors.Trace(err)
	}
	return b, nil
}

// SupportsContainers returns whether the charm supports containers. This
// currently means that the charm is a kubernetes charm. This will return true
// if the charm is a controller charm, and false otherwise.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) SupportsContainers(ctx context.Context, locator charm.CharmLocator) (bool, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return false, errors.Trace(err)
	}
	b, err := s.st.SupportsContainers(ctx, id)
	if err != nil {
		return false, errors.Trace(err)
	}
	return b, nil
}

// IsSubordinateCharm returns whether the charm is a subordinate charm.
// This will return true if the charm is a subordinate charm, and false
// otherwise.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) IsSubordinateCharm(ctx context.Context, locator charm.CharmLocator) (bool, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return false, errors.Trace(err)
	}
	b, err := s.st.IsSubordinateCharm(ctx, id)
	if err != nil {
		return false, errors.Trace(err)
	}
	return b, nil
}

// GetCharm returns the charm by name, source and revision. Calling this method
// will return all the data associated with the charm. It is not expected to
// call this method for all calls, instead use the move focused and specific
// methods. That's because this method is very expensive to call. This is
// implemented for the cases where all the charm data is needed; model
// migration, charm export, etc.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharm(ctx context.Context, locator charm.CharmLocator) (internalcharm.Charm, charm.CharmLocator, bool, error) {
	// We stil retrieve the ID from state, in the future we should move
	// this to the state GetCharm method so it runs under the same
	// transaction.
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return nil, charm.CharmLocator{}, false, errors.Trace(err)
	}

	return s.getCharmAndLocator(ctx, id)
}

func (s *Service) getCharmLocatorByID(ctx context.Context, charmID corecharm.ID) (charm.CharmLocator, error) {
	locator, err := s.st.GetCharmLocatorByCharmID(ctx, charmID)
	return locator, errors.Trace(err)
}

func (s *Service) getCharmAndLocator(ctx context.Context, charmID corecharm.ID) (internalcharm.Charm, charm.CharmLocator, bool, error) {
	ch, _, err := s.st.GetCharm(ctx, charmID)
	if err != nil {
		return nil, charm.CharmLocator{}, false, errors.Trace(err)
	}

	// The charm needs to be decoded into the internalcharm.Charm type.

	metadata, err := decodeMetadata(ch.Metadata)
	if err != nil {
		return nil, charm.CharmLocator{}, false, errors.Trace(err)
	}

	manifest, err := decodeManifest(ch.Manifest)
	if err != nil {
		return nil, charm.CharmLocator{}, false, errors.Trace(err)
	}

	actions, err := decodeActions(ch.Actions)
	if err != nil {
		return nil, charm.CharmLocator{}, false, errors.Trace(err)
	}

	config, err := decodeConfig(ch.Config)
	if err != nil {
		return nil, charm.CharmLocator{}, false, errors.Trace(err)
	}

	lxdProfile, err := decodeLXDProfile(ch.LXDProfile)
	if err != nil {
		return nil, charm.CharmLocator{}, false, errors.Trace(err)
	}

	returnedLocator := charm.CharmLocator{
		Name:         ch.ReferenceName,
		Revision:     ch.Revision,
		Source:       ch.Source,
		Architecture: ch.Architecture,
	}

	charmBase := internalcharm.NewCharmBase(
		&metadata,
		&manifest,
		&config,
		&actions,
		&lxdProfile,
	)
	charmBase.SetVersion(ch.Version)

	return charmBase, returnedLocator, ch.Available, nil
}

// GetCharmMetadata returns the metadata for the charm using the charm name,
// source and revision.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmMetadata(ctx context.Context, locator charm.CharmLocator) (internalcharm.Meta, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return internalcharm.Meta{}, errors.Trace(err)
	}
	metadata, err := s.st.GetCharmMetadata(ctx, id)
	if err != nil {
		return internalcharm.Meta{}, errors.Trace(err)
	}

	decoded, err := decodeMetadata(metadata)
	if err != nil {
		return internalcharm.Meta{}, errors.Trace(err)
	}
	return decoded, nil
}

// GetCharmMetadataName returns the name for the charm using the
// charm name, source and revision.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmMetadataName(ctx context.Context, locator charm.CharmLocator) (string, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return "", errors.Trace(err)
	}
	name, err := s.st.GetCharmMetadataName(ctx, id)
	if err != nil {
		return "", errors.Trace(err)
	}
	return name, nil
}

// GetCharmMetadataDescription returns the description for the charm using the
// charm name, source and revision.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmMetadataDescription(ctx context.Context, locator charm.CharmLocator) (string, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return "", errors.Trace(err)
	}
	description, err := s.st.GetCharmMetadataDescription(ctx, id)
	if err != nil {
		return "", errors.Trace(err)
	}
	return description, nil
}

// GetCharmMetadataStorage returns the storage specification for the charm using
// the charm name, source and revision.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmMetadataStorage(ctx context.Context, locator charm.CharmLocator) (map[string]internalcharm.Storage, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storage, err := s.st.GetCharmMetadataStorage(ctx, id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	decoded, err := decodeMetadataStorage(storage)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return decoded, nil
}

// GetCharmMetadataResources returns the specifications for the resources for the
// charm using the charm name, source and revision.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmMetadataResources(ctx context.Context, locator charm.CharmLocator) (map[string]resource.Meta, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	resources, err := s.st.GetCharmMetadataResources(ctx, id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	decoded, err := decodeMetadataResources(resources)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return decoded, nil
}

// GetCharmManifest returns the manifest for the charm using the charm name,
// source and revision.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmManifest(ctx context.Context, locator charm.CharmLocator) (internalcharm.Manifest, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return internalcharm.Manifest{}, errors.Trace(err)
	}
	manifest, err := s.st.GetCharmManifest(ctx, id)
	if err != nil {
		return internalcharm.Manifest{}, errors.Trace(err)
	}

	decoded, err := decodeManifest(manifest)
	if err != nil {
		return internalcharm.Manifest{}, errors.Trace(err)
	}
	return decoded, nil
}

// GetCharmActions returns the actions for the charm using the charm name,
// source and revision.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmActions(ctx context.Context, locator charm.CharmLocator) (internalcharm.Actions, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return internalcharm.Actions{}, errors.Trace(err)
	}
	actions, err := s.st.GetCharmActions(ctx, id)
	if err != nil {
		return internalcharm.Actions{}, errors.Trace(err)
	}

	decoded, err := decodeActions(actions)
	if err != nil {
		return internalcharm.Actions{}, errors.Trace(err)
	}
	return decoded, nil
}

// GetCharmConfig returns the config for the charm using the charm name,
// source and revision.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmConfig(ctx context.Context, locator charm.CharmLocator) (internalcharm.Config, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return internalcharm.Config{}, errors.Trace(err)
	}
	config, err := s.st.GetCharmConfig(ctx, id)
	if err != nil {
		return internalcharm.Config{}, errors.Trace(err)
	}

	decoded, err := decodeConfig(config)
	if err != nil {
		return internalcharm.Config{}, errors.Trace(err)
	}
	return decoded, nil
}

// GetCharmLXDProfile returns the LXD profile along with the revision of the
// charm using the charm name, source and revision.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmLXDProfile(ctx context.Context, locator charm.CharmLocator) (internalcharm.LXDProfile, charm.Revision, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return internalcharm.LXDProfile{}, -1, fmt.Errorf("charm id: %w", err)
	}
	profile, revision, err := s.st.GetCharmLXDProfile(ctx, id)
	if err != nil {
		return internalcharm.LXDProfile{}, -1, errors.Trace(err)
	}

	decoded, err := decodeLXDProfile(profile)
	if err != nil {
		return internalcharm.LXDProfile{}, -1, errors.Trace(err)
	}
	return decoded, revision, nil
}

// GetCharmArchivePath returns the archive storage path for the charm using the
// charm name, source and revision.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmArchivePath(ctx context.Context, locator charm.CharmLocator) (string, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return "", internalerrors.Errorf("charm id: %w", err)
	}
	path, err := s.st.GetCharmArchivePath(ctx, id)
	if err != nil {
		return "", internalerrors.Errorf("getting charm archive path: %w", err)
	}
	return path, nil
}

// GetCharmArchive returns a ReadCloser stream for the charm archive for a given
// charm id, along with the hash of the charm archive. Clients can use the hash
// to verify the integrity of the charm archive.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmArchive(ctx context.Context, locator charm.CharmLocator) (io.ReadCloser, string, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return nil, "", internalerrors.Errorf("charm id: %w", err)
	}
	archivePath, hash, err := s.st.GetCharmArchiveMetadata(ctx, id)
	if err != nil {
		return nil, "", internalerrors.Errorf("getting charm archive metadata: %w", err)
	}

	reader, err := s.charmStore.Get(ctx, archivePath)
	if errors.Is(err, store.ErrNotFound) {
		return nil, "", applicationerrors.CharmNotFound
	} else if err != nil {
		return nil, "", internalerrors.Errorf("getting charm archive: %w", err)
	}

	return reader, hash, nil
}

// GetCharmArchiveBySHA256Prefix returns a ReadCloser stream for the charm
// archive who's SHA256 hash starts with the provided prefix.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmArchiveBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, error) {
	reader, err := s.charmStore.GetBySHA256Prefix(ctx, sha256Prefix)
	if errors.Is(err, store.ErrNotFound) {
		return nil, applicationerrors.CharmNotFound
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return reader, nil
}

// IsCharmAvailable returns whether the charm is available for use. This
// indicates if the charm has been uploaded to the controller.
// This will return true if the charm is available, and false otherwise.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) IsCharmAvailable(ctx context.Context, locator charm.CharmLocator) (bool, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return false, fmt.Errorf("charm id: %w", err)
	}
	b, err := s.st.IsCharmAvailable(ctx, id)
	if err != nil {
		return false, errors.Trace(err)
	}
	return b, nil
}

// SetCharmAvailable sets the charm as available for use.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) SetCharmAvailable(ctx context.Context, locator charm.CharmLocator) error {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return fmt.Errorf("charm id: %w", err)
	}
	return errors.Trace(s.st.SetCharmAvailable(ctx, id))
}

// SetCharm persists the charm metadata, actions, config and manifest to
// state.
// If there are any non-blocking issues with the charm metadata, actions,
// config or manifest, a set of warnings will be returned.
func (s *Service) SetCharm(ctx context.Context, args charm.SetCharmArgs) (corecharm.ID, []string, error) {
	result, warnings, err := s.setCharm(ctx, args)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	return result.ID, warnings, nil
}

// DeleteCharm removes the charm from the state.
// Returns an error if the charm does not exist.
func (s *Service) DeleteCharm(ctx context.Context, locator charm.CharmLocator) error {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return fmt.Errorf("charm id: %w", err)
	}
	return s.st.DeleteCharm(ctx, id)
}

// ListCharmLocators returns a list of charm locators. The locator allows you to
// reconstruct the charm URL. If no names are provided, then all charms are
// listed. If no names are matched against the charm names, then an empty list
// is returned.
func (s *Service) ListCharmLocators(ctx context.Context, names ...string) ([]charm.CharmLocator, error) {
	if len(names) == 0 {
		return s.st.ListCharmLocators(ctx)
	}
	return s.st.ListCharmLocatorsByNames(ctx, names)
}

// GetCharmDownloadInfo returns the download info for the charm using the
// charm name, source and revision.
func (s *Service) GetCharmDownloadInfo(ctx context.Context, locator charm.CharmLocator) (*charm.DownloadInfo, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.st.GetCharmDownloadInfo(ctx, id)
}

// GetAvailableCharmArchiveSHA256 returns the SHA256 hash of the charm archive
// for the given charm name, source and revision. If the charm is not available,
// [applicationerrors.CharmNotResolved] is returned.
func (s *Service) GetAvailableCharmArchiveSHA256(ctx context.Context, locator charm.CharmLocator) (string, error) {
	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return "", errors.Trace(err)
	}
	return s.st.GetAvailableCharmArchiveSHA256(ctx, id)
}

// ResolveUploadCharm resolves the upload of a charm archive. If the charm is
// being imported from a migration then it can returns
// [applicationerrors.CharmNotFound]. Returns
// [applicationerrors.CharmSourceNotValid] if the source is not valid. Returns
// [applicationerrors.CharmMetadataNotValid] if the charm metadata is not valid.
// Returns [applicationerrors.CharmNameNotValid] if the reference name is not
// valid. Returns [applicationerrors.CharmManifestNotFound] if the charm
// manifest is not found. Returns [applicationerrors.CharmDownloadInfoNotFound]
// if the download info is not found. Returns [applicationerrors.CharmNotFound]
// if the charm is not found.
func (s *Service) ResolveUploadCharm(ctx context.Context, args charm.ResolveUploadCharm) (charm.CharmLocator, error) {
	var localUploadCharm bool

	switch args.Source {
	case corecharm.CharmHub:
		if !args.Importing {
			return charm.CharmLocator{}, applicationerrors.NonLocalCharmImporting
		}
	case corecharm.Local:
		localUploadCharm = !args.Importing
	default:
		return charm.CharmLocator{}, applicationerrors.CharmSourceNotValid
	}

	// We're not importing a charm, this is a full blown upload.
	if localUploadCharm {
		return s.resolveLocalUploadedCharm(ctx, args)
	}

	// We're importing a charm, all the metadata of the charm will have been
	// migrated already, so this is pushing the charm to the object store and
	// setting the charm as available.
	return s.resolveMigratingUploadedCharm(ctx, args)
}

func (s *Service) resolveLocalUploadedCharm(ctx context.Context, args charm.ResolveUploadCharm) (charm.CharmLocator, error) {
	// Store the charm and validate it against the sha256 prefix.
	result, digest, err := s.charmStore.StoreFromReader(ctx, args.Reader, args.SHA256Prefix)
	if err != nil {
		return charm.CharmLocator{}, internalerrors.Errorf("resolving uploaded charm: %w", err)
	}

	// Ensure we close the charm reader.
	defer func() {
		if err := result.Charm.Close(); err != nil {
			s.logger.Errorf(ctx, "closing reader: %v", err)
		}
	}()

	// We must ensure that the objectstore UUID is valid.
	if err := result.ObjectStoreUUID.Validate(); err != nil {
		return charm.CharmLocator{}, internalerrors.Errorf("invalid object store UUID: %w", err)
	}

	// Note: we don't have a full SHA256 from the user so, we can't actually
	// check the integrity of the charm archive. We can only check the prefix
	// of the SHA256 hash (that's enough to prevent collisions).

	// Make sure it's actually a valid charm.
	ch, err := internalcharm.ReadCharmArchiveFromReader(result.Charm, digest.Size)
	if err != nil {
		return charm.CharmLocator{}, internalerrors.Errorf("reading charm archive: %w", err)
	}

	// This is a full blown upload, we need to set everything up.
	resolved, warnings, err := s.setCharm(ctx, charm.SetCharmArgs{
		Charm:           ch,
		Source:          args.Source,
		ReferenceName:   args.Name,
		Hash:            digest.SHA256,
		ObjectStoreUUID: result.ObjectStoreUUID,
		Version:         ch.Version(),
		Architecture:    args.Architecture,
		DownloadInfo: &charm.DownloadInfo{
			Provenance: charm.ProvenanceUpload,
		},

		// The revision is not set, we need to sequence a revision.
		Revision:           -1,
		RequiresSequencing: true,

		// This is correct, we want to use the unique name of the stored charm
		// as the archive path. Once every blob is storing the UUID, we can
		// remove the archive path, until, just use the unique name.
		ArchivePath: result.UniqueName,
	})
	if err != nil {
		return charm.CharmLocator{}, errors.Annotate(err, "setting charm")
	} else if len(warnings) > 0 {
		s.logger.Infof(ctx, "setting charm: %v", warnings)
	}

	return resolved.Locator, nil
}

func (s *Service) resolveMigratingUploadedCharm(ctx context.Context, args charm.ResolveUploadCharm) (charm.CharmLocator, error) {
	// If we're importing a charm from migration, there are a few things we
	// can rely on:
	//
	//    1. The charm metadata has already been verified.
	//    2. The charm metadata has already been inserted into the database
	//       during the migration.
	//    3. A local charm has already been sequenced, so we don't need to
	//       attempt to sequence the charm again.
	//
	// This means we need to locate the charm that's already stored in the
	// database and set it as available.
	source, err := encodeCharmSource(args.Source)
	if err != nil {
		return charm.CharmLocator{}, internalerrors.Errorf("encoding charm source: %w", err)
	}

	// Locale the existing charm.
	charmID, err := s.getCharmID(ctx, charm.GetCharmArgs{
		Source:   source,
		Name:     args.Name,
		Revision: ptr(args.Revision),
	})
	if err != nil {
		return charm.CharmLocator{}, errors.Annotate(err, "locating existing charm")
	}

	result, digest, err := s.charmStore.StoreFromReader(ctx, args.Reader, args.SHA256Prefix)
	if err != nil {
		return charm.CharmLocator{}, internalerrors.Errorf("resolving uploaded charm: %w", err)
	}

	// Ensure we close the charm reader.
	defer func() {
		if err := result.Charm.Close(); err != nil {
			s.logger.Errorf(ctx, "closing reader: %v", err)
		}
	}()

	// We must ensure that the objectstore UUID is valid.
	if err := result.ObjectStoreUUID.Validate(); err != nil {
		return charm.CharmLocator{}, internalerrors.Errorf("invalid object store UUID: %w", err)
	}

	// Officially, the source model should write the charm hashes into the
	// description package, and the destination model should verify the hashes
	// before accepting the charm. For now this is a check to ensure that the
	// charm is valid.
	// Work should be done to ensure the integrity of the charm archive.
	if _, err := internalcharm.ReadCharmArchiveFromReader(result.Charm, digest.Size); err != nil {
		return charm.CharmLocator{}, errors.Annotatef(err, "reading charm archive")
	}

	// This will correctly sequence the charm if it's a local charm.
	return s.st.ResolveMigratingUploadedCharm(ctx, charmID, charm.ResolvedMigratingUploadedCharm{
		ObjectStoreUUID: result.ObjectStoreUUID,
		Hash:            digest.SHA256,
		DownloadInfo: &charm.DownloadInfo{
			Provenance: charm.ProvenanceMigration,
		},

		// This is correct, we want to use the unique name of the stored charm
		// as the archive path. Once every blob is storing the UUID, we can
		// remove the archive path, until, just use the unique name.
		ArchivePath: result.UniqueName,
	})
}

func (s *Service) setCharm(ctx context.Context, args charm.SetCharmArgs) (setCharmResult, []string, error) {
	// We require a valid charm metadata.
	if meta := args.Charm.Meta(); meta == nil {
		return setCharmResult{}, nil, applicationerrors.CharmMetadataNotValid
	} else if !isValidCharmName(meta.Name) {
		return setCharmResult{}, nil, applicationerrors.CharmNameNotValid
	}

	// We require a valid charm manifest.
	if manifest := args.Charm.Manifest(); manifest == nil {
		return setCharmResult{}, nil, applicationerrors.CharmManifestNotFound
	} else if len(manifest.Bases) == 0 {
		return setCharmResult{}, nil, applicationerrors.CharmManifestNotValid
	}

	// If the reference name is provided, it must be valid.
	if !isValidReferenceName(args.ReferenceName) {
		return setCharmResult{}, nil, fmt.Errorf("reference name: %w", applicationerrors.CharmNameNotValid)
	}

	// If the origin is from charmhub, then we require the download info.
	if args.Source == corecharm.CharmHub {
		if args.DownloadInfo == nil {
			return setCharmResult{}, nil, applicationerrors.CharmDownloadInfoNotFound
		}
		if err := args.DownloadInfo.Validate(); err != nil {
			return setCharmResult{}, nil, fmt.Errorf("download info: %w", err)
		}
	}

	// Charm sequence validation.
	if args.RequiresSequencing && args.Revision != -1 {
		return setCharmResult{}, nil, applicationerrors.CharmRevisionNotValid
	}

	source, err := encodeCharmSource(args.Source)
	if err != nil {
		return setCharmResult{}, nil, fmt.Errorf("encoding charm source: %w", err)
	}

	architecture := encodeArchitecture(args.Architecture)
	ch, warnings, err := encodeCharm(args.Charm)
	if err != nil {
		return setCharmResult{}, warnings, fmt.Errorf("encoding charm: %w", err)
	}

	ch.Source = source
	ch.ReferenceName = args.ReferenceName
	ch.Revision = args.Revision
	ch.Hash = args.Hash
	ch.ArchivePath = args.ArchivePath
	ch.ObjectStoreUUID = args.ObjectStoreUUID
	ch.Available = args.ArchivePath != ""
	ch.Architecture = architecture

	charmID, locator, err := s.st.SetCharm(ctx, ch, args.DownloadInfo, args.RequiresSequencing)
	return setCharmResult{
		ID:      charmID,
		Locator: locator,
	}, warnings, errors.Trace(err)
}

// ReserveCharmRevision creates a new charm revision placeholder in state. This
// includes all the metadata, actions, config and manifest for the charm. The
// charm revision placeholder will include all the associated hash, and download
// information for the charm. Once the new charm revision place holder is linked
// to an application, the async charm downloader will download the charm archive
// and set the charm as available.
//
// If there are any non-blocking issues with the charm metadata, actions, config
// or manifest, a set of warnings will be returned.
//
// If the placeholder already exists, it is a noop but still return the
// corresponding charm ID.
func (s *Service) ReserveCharmRevision(ctx context.Context, args charm.ReserveCharmRevisionArgs) (corecharm.ID, []string, error) {
	result, warnings, err := s.setCharm(ctx, charm.SetCharmArgs{
		Charm:         args.Charm,
		Source:        args.Source,
		ReferenceName: args.ReferenceName,
		Hash:          args.Hash,
		Revision:      args.Revision,
		Architecture:  args.Architecture,
		DownloadInfo:  args.DownloadInfo,
	})
	if errors.Is(err, applicationerrors.CharmAlreadyExists) {
		var charmSource charm.CharmSource
		switch args.Source {
		case corecharm.CharmHub:
			charmSource = charm.CharmHubSource
		case corecharm.Local:
			charmSource = charm.LocalSource
		default:
			return "", nil, applicationerrors.CharmSourceNotValid
		}

		// retrieve the charm ID
		result.ID, err = s.st.GetCharmID(ctx, args.ReferenceName, args.Revision, charmSource)
	}
	return result.ID, warnings, errors.Trace(err)
}

// GetLatestPendingCharmhubCharm returns the latest charm that is pending from
// the charmhub store. If there are no charms, returns is not found, as
// [applicationerrors.CharmNotFound].
// If there are multiple charms, then the latest created at date is returned
// first.
func (s *Service) GetLatestPendingCharmhubCharm(ctx context.Context, name string, arch arch.Arch) (charm.CharmLocator, error) {
	if !isValidCharmName(name) {
		return charm.CharmLocator{}, applicationerrors.CharmNameNotValid
	}

	a := encodeArchitecture(arch)
	return s.st.GetLatestPendingCharmhubCharm(ctx, name, a)
}

// WatchCharms returns a watcher that observes changes to charms.
func (s *WatchableService) WatchCharms() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewUUIDsWatcher(
		s.st.NamespaceForWatchCharm(),
		changestream.All,
	)
}

type setCharmResult struct {
	ID      corecharm.ID
	Locator charm.CharmLocator
}

// encodeCharm encodes a charm to the service representation.
// Returns an error if the charm metadata cannot be encoded.
func encodeCharm(ch internalcharm.Charm) (charm.Charm, []string, error) {
	if ch == nil {
		return charm.Charm{}, nil, applicationerrors.CharmNotValid
	}

	metadata, err := encodeMetadata(ch.Meta())
	if err != nil {
		return charm.Charm{}, nil, fmt.Errorf("encoding metadata: %w", err)
	}

	manifest, warnings, err := encodeManifest(ch.Manifest())
	if err != nil {
		return charm.Charm{}, warnings, fmt.Errorf("encoding manifest: %w", err)
	}

	actions, err := encodeActions(ch.Actions())
	if err != nil {
		return charm.Charm{}, warnings, fmt.Errorf("encoding actions: %w", err)
	}

	config, err := encodeConfig(ch.Config())
	if err != nil {
		return charm.Charm{}, warnings, fmt.Errorf("encoding config: %w", err)
	}

	var profile []byte
	if lxdProfile, ok := ch.(internalcharm.LXDProfiler); ok && lxdProfile != nil {
		profile, err = encodeLXDProfile(lxdProfile.LXDProfile())
		if err != nil {
			return charm.Charm{}, warnings, fmt.Errorf("encoding lxd profile: %w", err)
		}
	}

	return charm.Charm{
		Metadata:   metadata,
		Manifest:   manifest,
		Actions:    actions,
		Config:     config,
		LXDProfile: profile,
	}, warnings, nil
}

// isValidCharmName returns whether name is a valid charm name.
func isValidCharmName(name string) bool {
	return charmNameRegExp.MatchString(name)
}

func argsFromLocator(locator charm.CharmLocator) charm.GetCharmArgs {
	return charm.GetCharmArgs{
		Name:     locator.Name,
		Revision: ptr(locator.Revision),
		Source:   locator.Source,
	}
}
