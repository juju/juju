// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"regexp"

	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

var (
	// charmNameRegExp is a regular expression representing charm name.
	// This is the same one from the names package.
	charmNameSnippet = "[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*"
	charmNameRegExp  = regexp.MustCompile("^" + charmNameSnippet + "$")
)

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	NewUUIDsWatcher(
		namespace string, changeMask changestream.ChangeType,
	) (watcher.StringsWatcher, error)
	NewValueMapperWatcher(string, string, changestream.ChangeType, eventsource.Mapper,
	) (watcher.NotifyWatcher, error)
	NewNamespaceMapperWatcher(
		namespace string, changeMask changestream.ChangeType,
		initialStateQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
	) (watcher.StringsWatcher, error)
}

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
	GetCharmMetadata(ctx context.Context, charmID corecharm.ID) (charm.Metadata, error)

	// GetCharmManifest returns the manifest for the charm using the charm ID.
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmManifest(ctx context.Context, charmID corecharm.ID) (charm.Manifest, error)

	// GetCharmMetadataName returns the name for the charm using the charm ID.
	GetCharmMetadataName(ctx context.Context, charmID corecharm.ID) (string, error)

	// GetCharmMetadataDescription returns the description for the charm using
	// the charm ID.
	GetCharmMetadataDescription(ctx context.Context, charmID corecharm.ID) (string, error)

	// GetCharmActions returns the actions for the charm using the charm ID. If
	// the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmActions(ctx context.Context, charmID corecharm.ID) (charm.Actions, error)

	// GetCharmConfig returns the config for the charm using the charm ID. If
	// the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmConfig(ctx context.Context, charmID corecharm.ID) (charm.Config, error)

	// GetCharmLXDProfile returns the LXD profile along with the revision of the
	// charm using the charm ID. The revision
	//
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmLXDProfile(ctx context.Context, charmID corecharm.ID) ([]byte, charm.Revision, error)

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
	// state.
	SetCharm(ctx context.Context, charm charm.Charm, downloadInfo *charm.DownloadInfo) (corecharm.ID, error)

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
	// charm ID.
	GetCharmDownloadInfo(ctx context.Context, id corecharm.ID) (*charm.DownloadInfo, error)
}

// CharmStore defines the interface for storing and retrieving charms archive
// blobs from the underlying storage.
type CharmStore interface {
	// GetCharm retrieves a ReadCloser for the charm archive at the give path
	// from the underlying storage.
	Get(ctx context.Context, archivePath string) (io.ReadCloser, error)
}

// GetCharmID returns a charm ID by name. It returns an error if the charm can
// not be found by the name. This can also be used as a cheap way to see if a
// charm exists without needing to load the charm metadata. Returns
// [applicationerrors.CharmNameNotValid] if the name is not valid, and
// [applicationerrors.CharmNotFound] if the charm is not found.
func (s *Service) GetCharmID(ctx context.Context, args charm.GetCharmArgs) (corecharm.ID, error) {
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
// return true if the charm is a controller charm, and false otherwise. If the
// charm does not exist, a [applicationerrors.CharmNotFound] error is returned.
func (s *Service) IsControllerCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	if err := id.Validate(); err != nil {
		return false, errors.Errorf("charm id: %w", err)
	}
	b, err := s.st.IsControllerCharm(ctx, id)
	if err != nil {
		return false, errors.Capture(err)
	}
	return b, nil
}

// SupportsContainers returns whether the charm supports containers. This
// currently means that the charm is a kubernetes charm. This will return true
// if the charm is a controller charm, and false otherwise.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) SupportsContainers(ctx context.Context, id corecharm.ID) (bool, error) {
	if err := id.Validate(); err != nil {
		return false, errors.Errorf("charm id: %w", err)
	}
	b, err := s.st.SupportsContainers(ctx, id)
	if err != nil {
		return false, errors.Capture(err)
	}
	return b, nil
}

// IsSubordinateCharm returns whether the charm is a subordinate charm.
// This will return true if the charm is a subordinate charm, and false
// otherwise.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) IsSubordinateCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	if err := id.Validate(); err != nil {
		return false, errors.Errorf("charm id: %w", err)
	}
	b, err := s.st.IsSubordinateCharm(ctx, id)
	if err != nil {
		return false, errors.Capture(err)
	}
	return b, nil
}

// GetCharm returns the charm using the charm ID. Calling this method will
// return all the data associated with the charm. It is not expected to call
// this method for all calls, instead use the move focused and specific methods.
// That's because this method is very expensive to call. This is implemented for
// the cases where all the charm data is needed; model migration, charm export,
// etc.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharm(ctx context.Context, id corecharm.ID) (internalcharm.Charm, charm.CharmLocator, error) {
	if err := id.Validate(); err != nil {
		return nil, charm.CharmLocator{}, errors.Errorf("charm id: %w", err)
	}

	ch, _, err := s.st.GetCharm(ctx, id)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	// The charm needs to be decoded into the internalcharm.Charm type.

	metadata, err := decodeMetadata(ch.Metadata)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	manifest, err := decodeManifest(ch.Manifest)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	actions, err := decodeActions(ch.Actions)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	config, err := decodeConfig(ch.Config)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	lxdProfile, err := decodeLXDProfile(ch.LXDProfile)
	if err != nil {
		return nil, charm.CharmLocator{}, errors.Capture(err)
	}

	locator := charm.CharmLocator{
		Name:         ch.ReferenceName,
		Revision:     ch.Revision,
		Source:       ch.Source,
		Architecture: ch.Architecture,
	}

	return internalcharm.NewCharmBase(
		&metadata,
		&manifest,
		&config,
		&actions,
		&lxdProfile,
	), locator, nil
}

// GetCharmMetadata returns the metadata for the charm using the charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmMetadata(ctx context.Context, id corecharm.ID) (internalcharm.Meta, error) {
	if err := id.Validate(); err != nil {
		return internalcharm.Meta{}, errors.Errorf("charm id: %w", err)
	}

	metadata, err := s.st.GetCharmMetadata(ctx, id)
	if err != nil {
		return internalcharm.Meta{}, errors.Capture(err)
	}

	decoded, err := decodeMetadata(metadata)
	if err != nil {
		return internalcharm.Meta{}, errors.Capture(err)
	}
	return decoded, nil
}

// GetCharmMetadataName returns the name for the charm using the
// charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmMetadataName(ctx context.Context, id corecharm.ID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", errors.Errorf("charm id: %w", err)
	}

	name, err := s.st.GetCharmMetadataName(ctx, id)
	if err != nil {
		return "", errors.Capture(err)
	}
	return name, nil
}

// GetCharmMetadataDescription returns the description for the charm using the
// charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmMetadataDescription(ctx context.Context, id corecharm.ID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", errors.Errorf("charm id: %w", err)
	}

	description, err := s.st.GetCharmMetadataDescription(ctx, id)
	if err != nil {
		return "", errors.Capture(err)
	}
	return description, nil
}

// GetCharmManifest returns the manifest for the charm using the charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmManifest(ctx context.Context, id corecharm.ID) (internalcharm.Manifest, error) {
	if err := id.Validate(); err != nil {
		return internalcharm.Manifest{}, errors.Errorf("charm id: %w", err)
	}

	manifest, err := s.st.GetCharmManifest(ctx, id)
	if err != nil {
		return internalcharm.Manifest{}, errors.Capture(err)
	}

	decoded, err := decodeManifest(manifest)
	if err != nil {
		return internalcharm.Manifest{}, errors.Capture(err)
	}
	return decoded, nil
}

// GetCharmActions returns the actions for the charm using the charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmActions(ctx context.Context, id corecharm.ID) (internalcharm.Actions, error) {
	if err := id.Validate(); err != nil {
		return internalcharm.Actions{}, errors.Errorf("charm id: %w", err)
	}

	actions, err := s.st.GetCharmActions(ctx, id)
	if err != nil {
		return internalcharm.Actions{}, errors.Capture(err)
	}

	decoded, err := decodeActions(actions)
	if err != nil {
		return internalcharm.Actions{}, errors.Capture(err)
	}
	return decoded, nil
}

// GetCharmConfig returns the config for the charm using the charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmConfig(ctx context.Context, id corecharm.ID) (internalcharm.Config, error) {
	if err := id.Validate(); err != nil {
		return internalcharm.Config{}, errors.Errorf("charm id: %w", err)
	}

	config, err := s.st.GetCharmConfig(ctx, id)
	if err != nil {
		return internalcharm.Config{}, errors.Capture(err)
	}

	decoded, err := decodeConfig(config)
	if err != nil {
		return internalcharm.Config{}, errors.Capture(err)
	}
	return decoded, nil
}

// GetCharmLXDProfile returns the LXD profile along with the revision of the
// charm using the charm ID. The revision
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmLXDProfile(ctx context.Context, id corecharm.ID) (internalcharm.LXDProfile, charm.Revision, error) {
	if err := id.Validate(); err != nil {
		return internalcharm.LXDProfile{}, -1, errors.Errorf("charm id: %w", err)
	}

	profile, revision, err := s.st.GetCharmLXDProfile(ctx, id)
	if err != nil {
		return internalcharm.LXDProfile{}, -1, errors.Capture(err)
	}

	decoded, err := decodeLXDProfile(profile)
	if err != nil {
		return internalcharm.LXDProfile{}, -1, errors.Capture(err)
	}
	return decoded, revision, nil
}

// GetCharmArchivePath returns the archive storage path for the charm using the
// charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmArchivePath(ctx context.Context, id corecharm.ID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", errors.Errorf("charm id: %w", err)
	}

	path, err := s.st.GetCharmArchivePath(ctx, id)
	if err != nil {
		return "", errors.Errorf("getting charm archive path: %w", err)
	}
	return path, nil
}

// GetCharmArchive returns a ReadCloser stream for the charm archive for a given
// charm id, along with the hash of the charm archive. Clients can use the hash
// to verify the integrity of the charm archive.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) GetCharmArchive(ctx context.Context, id corecharm.ID) (io.ReadCloser, string, error) {
	if err := id.Validate(); err != nil {
		return nil, "", errors.Errorf("charm id: %w", err)
	}

	archivePath, hash, err := s.st.GetCharmArchiveMetadata(ctx, id)
	if err != nil {
		return nil, "", errors.Errorf("getting charm archive metadata: %w", err)
	}

	reader, err := s.charmStore.Get(ctx, archivePath)
	if err != nil {
		return nil, "", errors.Errorf("getting charm archive: %w", err)
	}

	return reader, hash, nil
}

// IsCharmAvailable returns whether the charm is available for use. This
// indicates if the charm has been uploaded to the controller.
// This will return true if the charm is available, and false otherwise.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) IsCharmAvailable(ctx context.Context, id corecharm.ID) (bool, error) {
	if err := id.Validate(); err != nil {
		return false, errors.Errorf("charm id: %w", err)
	}
	b, err := s.st.IsCharmAvailable(ctx, id)
	if err != nil {
		return false, errors.Capture(err)
	}
	return b, nil
}

// SetCharmAvailable sets the charm as available for use.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *Service) SetCharmAvailable(ctx context.Context, id corecharm.ID) error {
	if err := id.Validate(); err != nil {
		return errors.Errorf("charm id: %w", err)
	}

	return errors.Capture(s.st.SetCharmAvailable(ctx, id))
}

// SetCharm persists the charm metadata, actions, config and manifest to
// state.
// If there are any non-blocking issues with the charm metadata, actions,
// config or manifest, a set of warnings will be returned.
func (s *Service) SetCharm(ctx context.Context, args charm.SetCharmArgs) (corecharm.ID, []string, error) {
	// We require a valid charm metadata.
	if meta := args.Charm.Meta(); meta == nil {
		return "", nil, applicationerrors.CharmMetadataNotValid
	} else if !isValidCharmName(meta.Name) {
		return "", nil, applicationerrors.CharmNameNotValid
	}

	// We require a valid charm manifest.
	if manifest := args.Charm.Manifest(); manifest == nil {
		return "", nil, applicationerrors.CharmManifestNotFound
	} else if len(manifest.Bases) == 0 {
		return "", nil, applicationerrors.CharmManifestNotValid
	}

	// If the reference name is provided, it must be valid.
	if !isValidReferenceName(args.ReferenceName) {
		return "", nil, errors.Errorf("reference name: %w", applicationerrors.CharmNameNotValid)
	}

	// If the origin is from charmhub, then we require the download info.
	if args.Source == corecharm.CharmHub {
		if args.DownloadInfo == nil {
			return "", nil, applicationerrors.CharmDownloadInfoNotFound
		}
		if err := args.DownloadInfo.Validate(); err != nil {
			return "", nil, errors.Errorf("download info: %w", err)
		}
	}

	source, err := encodeCharmSource(args.Source)
	if err != nil {
		return "", nil, errors.Errorf("encoding charm source: %w", err)
	}

	architecture := encodeArchitecture(args.Architecture)
	ch, warnings, err := encodeCharm(args.Charm)
	if err != nil {
		return "", warnings, errors.Errorf("encoding charm: %w", err)
	}

	ch.Source = source
	ch.ReferenceName = args.ReferenceName
	ch.Revision = args.Revision
	ch.Hash = args.Hash
	ch.ArchivePath = args.ArchivePath
	ch.Available = args.ArchivePath != ""
	ch.Architecture = architecture

	charmID, err := s.st.SetCharm(ctx, ch, args.DownloadInfo)
	if err != nil {
		return "", warnings, errors.Capture(err)
	}

	return charmID, warnings, nil
}

// DeleteCharm removes the charm from the state.
// Returns an error if the charm does not exist.
func (s *Service) DeleteCharm(ctx context.Context, id corecharm.ID) error {
	if err := id.Validate(); err != nil {
		return errors.Errorf("charm id: %w", err)
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
// charm ID.
func (s *Service) GetCharmDownloadInfo(ctx context.Context, id corecharm.ID) (*charm.DownloadInfo, error) {
	if err := id.Validate(); err != nil {
		return nil, errors.Errorf("charm id: %w", err)
	}
	return s.st.GetCharmDownloadInfo(ctx, id)
}

// WatchCharms returns a watcher that observes changes to charms.
func (s *WatchableService) WatchCharms() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewUUIDsWatcher(
		"charm",
		changestream.All,
	)
}

// encodeCharm encodes a charm to the service representation.
// Returns an error if the charm metadata cannot be encoded.
func encodeCharm(ch internalcharm.Charm) (charm.Charm, []string, error) {
	if ch == nil {
		return charm.Charm{}, nil, applicationerrors.CharmNotValid
	}

	metadata, err := encodeMetadata(ch.Meta())
	if err != nil {
		return charm.Charm{}, nil, errors.Errorf("encoding metadata: %w", err)
	}

	manifest, warnings, err := encodeManifest(ch.Manifest())
	if err != nil {
		return charm.Charm{}, warnings, errors.Errorf("encoding manifest: %w", err)
	}

	actions, err := encodeActions(ch.Actions())
	if err != nil {
		return charm.Charm{}, warnings, errors.Errorf("encoding actions: %w", err)
	}

	config, err := encodeConfig(ch.Config())
	if err != nil {
		return charm.Charm{}, warnings, errors.Errorf("encoding config: %w", err)
	}

	var profile []byte
	if lxdProfile, ok := ch.(internalcharm.LXDProfiler); ok && lxdProfile != nil {
		profile, err = encodeLXDProfile(lxdProfile.LXDProfile())
		if err != nil {
			return charm.Charm{}, warnings, errors.Errorf("encoding lxd profile: %w", err)
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
