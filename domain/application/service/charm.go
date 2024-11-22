// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"regexp"

	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/downloader"
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
	GetCharmArchivePath(ctx context.Context, charmID corecharm.ID) (string, error)

	// IsCharmAvailable returns whether the charm is available for use. If the
	// charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	IsCharmAvailable(ctx context.Context, charmID corecharm.ID) (bool, error)

	// SetCharmAvailable sets the charm as available for use. If the charm does
	// not exist, a [applicationerrors.CharmNotFound] error is returned.
	SetCharmAvailable(ctx context.Context, charmID corecharm.ID) error

	// ReserveCharmRevision defines a placeholder for a new charm revision.
	// The original charm will need to exist, the returning charm ID will be
	// the new charm ID for the revision.
	ReserveCharmRevision(ctx context.Context, id corecharm.ID, revision int) (corecharm.ID, error)

	// GetCharm returns the charm using the charm ID.
	GetCharm(ctx context.Context, id corecharm.ID) (charm.Charm, charm.CharmOrigin, error)

	// SetCharm persists the charm metadata, actions, config and manifest to
	// state.
	SetCharm(ctx context.Context, charm charm.Charm, state charm.SetStateArgs) (corecharm.ID, error)

	// DeleteCharm removes the charm from the state. If the charm does not
	// exist, a [applicationerrors.CharmNotFound]  error is returned.
	DeleteCharm(ctx context.Context, id corecharm.ID) error

	// ListCharmsWithOrigin returns a list of charms with the specified
	// origin. We require the origin, so we can reconstruct the charm URL for
	// the client response.
	ListCharmsWithOrigin(ctx context.Context) ([]charm.CharmWithOrigin, error)

	// ListCharmsWithOriginByNames returns a list of charms with the specified
	// origin. We require the origin, so we can reconstruct the charm URL for
	// the client response. If no names are provided, then nothing is returned.
	ListCharmsWithOriginByNames(ctx context.Context, names []string) ([]charm.CharmWithOrigin, error)
}

// CharmDownloader defines the interface for downloading charms.
type CharmDownloader interface {
	// DownloadAndStore downloads a charm and persists it to the object store.
	// The resulting UUID can be used as RI against the object store.
	DownloadAndStore(ctx context.Context, name string, origin corecharm.Origin, force bool) (downloader.DownloadUUID, corecharm.Origin, error)
}

// CharmService provides the API for working with charms.
type CharmService struct {
	st     CharmState
	logger logger.Logger
}

// NewCharmService returns a new service reference wrapping the input state.
func NewCharmService(st CharmState, logger logger.Logger) *CharmService {
	return &CharmService{
		st:     st,
		logger: logger,
	}
}

// GetCharmID returns a charm ID by name. It returns an error if the charm
// can not be found by the name.
// This can also be used as a cheap way to see if a charm exists without
// needing to load the charm metadata.
// Returns [applicationerrors.CharmNameNotValid] if the name is not valid, and
// [applicationerrors.CharmNotFound] if the charm is not found.
func (s *CharmService) GetCharmID(ctx context.Context, args charm.GetCharmArgs) (corecharm.ID, error) {
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
func (s *CharmService) IsControllerCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	if err := id.Validate(); err != nil {
		return false, fmt.Errorf("charm id: %w", err)
	}
	b, err := s.st.IsControllerCharm(ctx, id)
	if err != nil {
		return false, errors.Trace(err)
	}
	return b, nil
}

// SupportsContainers returns whether the charm supports containers. This
// currently means that the charm is a kubernetes charm.
// This will return true if the charm is a controller charm, and false
// otherwise.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) SupportsContainers(ctx context.Context, id corecharm.ID) (bool, error) {
	if err := id.Validate(); err != nil {
		return false, fmt.Errorf("charm id: %w", err)
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
func (s *CharmService) IsSubordinateCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	if err := id.Validate(); err != nil {
		return false, fmt.Errorf("charm id: %w", err)
	}
	b, err := s.st.IsSubordinateCharm(ctx, id)
	if err != nil {
		return false, errors.Trace(err)
	}
	return b, nil
}

// GetCharm returns the charm using the charm ID.
// Calling this method will return all the data associated with the charm.
// It is not expected to call this method for all calls, instead use the move
// focused and specific methods. That's because this method is very expensive
// to call. This is implemented for the cases where all the charm data is
// needed; model migration, charm export, etc.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) GetCharm(ctx context.Context, id corecharm.ID) (internalcharm.Charm, charm.CharmOrigin, error) {
	if err := id.Validate(); err != nil {
		return nil, charm.CharmOrigin{}, fmt.Errorf("charm id: %w", err)
	}

	resultCharm, resultOrigin, err := s.st.GetCharm(ctx, id)
	if err != nil {
		return nil, charm.CharmOrigin{}, errors.Trace(err)
	}

	// The charm needs to be decoded into the internalcharm.Charm type.

	metadata, err := decodeMetadata(resultCharm.Metadata)
	if err != nil {
		return nil, charm.CharmOrigin{}, errors.Trace(err)
	}

	manifest, err := decodeManifest(resultCharm.Manifest)
	if err != nil {
		return nil, charm.CharmOrigin{}, errors.Trace(err)
	}

	actions, err := decodeActions(resultCharm.Actions)
	if err != nil {
		return nil, charm.CharmOrigin{}, errors.Trace(err)
	}

	config, err := decodeConfig(resultCharm.Config)
	if err != nil {
		return nil, charm.CharmOrigin{}, errors.Trace(err)
	}

	lxdProfile, err := decodeLXDProfile(resultCharm.LXDProfile)
	if err != nil {
		return nil, charm.CharmOrigin{}, errors.Trace(err)
	}

	return internalcharm.NewCharmBase(
		&metadata,
		&manifest,
		&config,
		&actions,
		&lxdProfile,
	), resultOrigin, nil
}

// GetCharmMetadata returns the metadata for the charm using the charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) GetCharmMetadata(ctx context.Context, id corecharm.ID) (internalcharm.Meta, error) {
	if err := id.Validate(); err != nil {
		return internalcharm.Meta{}, fmt.Errorf("charm id: %w", err)
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
// charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) GetCharmMetadataName(ctx context.Context, id corecharm.ID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", fmt.Errorf("charm id: %w", err)
	}

	name, err := s.st.GetCharmMetadataName(ctx, id)
	if err != nil {
		return "", errors.Trace(err)
	}
	return name, nil
}

// GetCharmMetadataDescription returns the description for the charm using the
// charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) GetCharmMetadataDescription(ctx context.Context, id corecharm.ID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", fmt.Errorf("charm id: %w", err)
	}

	description, err := s.st.GetCharmMetadataDescription(ctx, id)
	if err != nil {
		return "", errors.Trace(err)
	}
	return description, nil
}

// GetCharmManifest returns the manifest for the charm using the charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) GetCharmManifest(ctx context.Context, id corecharm.ID) (internalcharm.Manifest, error) {
	if err := id.Validate(); err != nil {
		return internalcharm.Manifest{}, fmt.Errorf("charm id: %w", err)
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

// GetCharmActions returns the actions for the charm using the charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) GetCharmActions(ctx context.Context, id corecharm.ID) (internalcharm.Actions, error) {
	if err := id.Validate(); err != nil {
		return internalcharm.Actions{}, fmt.Errorf("charm id: %w", err)
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

// GetCharmConfig returns the config for the charm using the charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) GetCharmConfig(ctx context.Context, id corecharm.ID) (internalcharm.Config, error) {
	if err := id.Validate(); err != nil {
		return internalcharm.Config{}, fmt.Errorf("charm id: %w", err)
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
// charm using the charm ID. The revision
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) GetCharmLXDProfile(ctx context.Context, id corecharm.ID) (internalcharm.LXDProfile, charm.Revision, error) {
	if err := id.Validate(); err != nil {
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
// charm ID.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) GetCharmArchivePath(ctx context.Context, id corecharm.ID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", fmt.Errorf("charm id: %w", err)
	}

	path, err := s.st.GetCharmArchivePath(ctx, id)
	if err != nil {
		return "", errors.Trace(err)
	}
	return path, nil
}

// IsCharmAvailable returns whether the charm is available for use. This
// indicates if the charm has been uploaded to the controller.
// This will return true if the charm is available, and false otherwise.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) IsCharmAvailable(ctx context.Context, id corecharm.ID) (bool, error) {
	if err := id.Validate(); err != nil {
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
func (s *CharmService) SetCharmAvailable(ctx context.Context, id corecharm.ID) error {
	if err := id.Validate(); err != nil {
		return fmt.Errorf("charm id: %w", err)
	}

	return errors.Trace(s.st.SetCharmAvailable(ctx, id))
}

// ReserveCharmRevision defines a placeholder for a new charm revision. The
// original charm will need to exist, the returning charm ID will be the new
// charm ID for the revision.
// This is useful for when a new charm revision becomes available. The essential
// charm documents might be available, but the blob or associated non-essential
// documents will not be.
//
// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
// returned.
func (s *CharmService) ReserveCharmRevision(ctx context.Context, id corecharm.ID, revision int) (corecharm.ID, error) {
	if err := id.Validate(); err != nil {
		return "", fmt.Errorf("charm id: %w", err)
	}
	if revision < 0 {
		return "", applicationerrors.CharmRevisionNotValid
	}

	newID, err := s.st.ReserveCharmRevision(ctx, id, revision)
	if err != nil {
		return "", errors.Trace(err)
	}
	return newID, nil
}

// SetCharm persists the charm metadata, actions, config and manifest to
// state.
// If there are any non-blocking issues with the charm metadata, actions,
// config or manifest, a set of warnings will be returned.
func (s *CharmService) SetCharm(ctx context.Context, args charm.SetCharmArgs) (corecharm.ID, []string, error) {
	meta := args.Charm.Meta()
	if meta == nil {
		return "", nil, applicationerrors.CharmMetadataNotValid
	} else if meta.Name == "" {
		return "", nil, applicationerrors.CharmNameNotValid
	}

	source, err := charm.ParseCharmSchema(args.Source)
	if err != nil {
		return "", nil, fmt.Errorf("encode charm source: %w", err)
	}

	ch, warnings, err := encodeCharm(args.Charm)
	if err != nil {
		return "", warnings, fmt.Errorf("encode charm: %w", err)
	}

	charmID, err := s.st.SetCharm(ctx, ch, charm.SetStateArgs{
		Source:        source,
		ReferenceName: args.ReferenceName,
		Revision:      args.Revision,
		Hash:          args.Hash,
		ArchivePath:   args.ArchivePath,
		Version:       args.Version,
	})
	if err != nil {
		return "", warnings, errors.Trace(err)
	}

	return charmID, warnings, nil
}

// DeleteCharm removes the charm from the state.
// Returns an error if the charm does not exist.
func (s *CharmService) DeleteCharm(ctx context.Context, id corecharm.ID) error {
	return s.st.DeleteCharm(ctx, id)
}

// ListCharmsWithOriginByNames returns a list of charms with the specified
// origin. We require the origin, so we can reconstruct the charm URL for the
// client response. If no names are provided, then all charms are listed. If no
// names are matched against the charm names, then an empty list is returned.
func (s *CharmService) ListCharmsWithOriginByNames(ctx context.Context, names ...string) ([]charm.CharmWithOrigin, error) {
	if len(names) == 0 {
		return s.st.ListCharmsWithOrigin(ctx)
	}
	return s.st.ListCharmsWithOriginByNames(ctx, names)
}

// WatchableCharmService provides the API for working with charms and the
// ability to create watchers.
type WatchableCharmService struct {
	CharmService
	watcherFactory WatcherFactory
}

// NewWatchableCharmService returns a new service reference wrapping the input state.
func NewWatchableCharmService(st CharmState, watcherFactory WatcherFactory, logger logger.Logger) *WatchableCharmService {
	return &WatchableCharmService{
		CharmService: CharmService{
			st:     st,
			logger: logger,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchCharms returns a watcher that observes changes to charms.
func (s *WatchableCharmService) WatchCharms() (watcher.StringsWatcher, error) {
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
		return charm.Charm{}, nil, fmt.Errorf("encode metadata: %w", err)
	}

	manifest, warnings, err := encodeManifest(ch.Manifest())
	if err != nil {
		return charm.Charm{}, warnings, fmt.Errorf("encode manifest: %w", err)
	}

	actions, err := encodeActions(ch.Actions())
	if err != nil {
		return charm.Charm{}, warnings, fmt.Errorf("encode actions: %w", err)
	}

	config, err := encodeConfig(ch.Config())
	if err != nil {
		return charm.Charm{}, warnings, fmt.Errorf("encode config: %w", err)
	}

	var profile []byte
	if lxdProfile, ok := ch.(internalcharm.LXDProfiler); ok && lxdProfile != nil {
		profile, err = encodeLXDProfile(lxdProfile.LXDProfile())
		if err != nil {
			return charm.Charm{}, warnings, fmt.Errorf("encode lxd profile: %w", err)
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
