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
	"github.com/juju/juju/domain/charm"
	charmerrors "github.com/juju/juju/domain/charm/errors"
	internalcharm "github.com/juju/juju/internal/charm"
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
}

// State describes retrieval and persistence methods for charms.
type State interface {
	// GetCharmIDByRevision returns the charm ID by the natural key, for a
	// specific revision.
	// If the charm does not exist, a NotFound error is returned.
	GetCharmIDByRevision(ctx context.Context, name string, revision int) (corecharm.ID, error)

	// GetCharmIDByLatestRevision returns the charm ID by the natural key, for
	// the latest revision.
	// If the charm does not exist, a NotFound error is returned.
	GetCharmIDByLatestRevision(ctx context.Context, name string) (corecharm.ID, error)

	// IsControllerCharm returns whether the charm is a controller charm.
	// If the charm does not exist, a NotFound error is returned.
	IsControllerCharm(ctx context.Context, id corecharm.ID) (bool, error)

	// IsSubordinateCharm returns whether the charm is a subordinate charm.
	// If the charm does not exist, a NotFound error is returned.
	IsSubordinateCharm(ctx context.Context, charmID corecharm.ID) (bool, error)

	// SupportsContainers returns whether the charm supports containers.
	// If the charm does not exist, a NotFound error is returned.
	SupportsContainers(ctx context.Context, charmID corecharm.ID) (bool, error)

	// GetCharmMetadata returns the metadata for the charm using the charm ID.
	// If the charm does not exist, a NotFound error is returned.
	GetCharmMetadata(ctx context.Context, charmID corecharm.ID) (charm.Metadata, error)

	// GetCharmManifest returns the manifest for the charm using the charm ID.
	// If the charm does not exist, a NotFound error is returned.
	GetCharmManifest(ctx context.Context, charmID corecharm.ID) (charm.Manifest, error)

	// GetCharmActions returns the actions for the charm using the charm ID.
	// If the charm does not exist, a NotFound error is returned.
	GetCharmActions(ctx context.Context, charmID corecharm.ID) (charm.Actions, error)

	// GetCharmConfig returns the config for the charm using the charm ID.
	// If the charm does not exist, a NotFound error is returned.
	GetCharmConfig(ctx context.Context, charmID corecharm.ID) (charm.Config, error)

	// GetCharmLXDProfile returns the LXD profile for the charm using the
	// charm ID.
	// If the charm does not exist, a NotFound error is returned.
	GetCharmLXDProfile(ctx context.Context, charmID corecharm.ID) ([]byte, error)

	// IsCharmAvailable returns whether the charm is available for use.
	// If the charm does not exist, a NotFound error is returned.
	IsCharmAvailable(ctx context.Context, charmID corecharm.ID) (bool, error)

	// SetCharmAvailable sets the charm as available for use.
	// If the charm does not exist, a NotFound error is returned.
	SetCharmAvailable(ctx context.Context, charmID corecharm.ID) error

	// ReserveCharmRevision defines a placeholder for a new charm revision.
	// The original charm will need to exist, the returning charm ID will be
	// the new charm ID for the revision.
	ReserveCharmRevision(ctx context.Context, id corecharm.ID, revision int) (corecharm.ID, error)

	// SetCharm persists the charm metadata, actions, config and manifest to
	// state.
	SetCharm(ctx context.Context, charm charm.Charm) (corecharm.ID, error)
}

// Service provides the API for working with charms.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// GetCharmID returns a charm ID by name. It returns an error if the charm
// can not be found by the name.
// This can also be used as a cheap way to see if a charm exists without
// needing to load the charm metadata.
func (s *Service) GetCharmID(ctx context.Context, args charm.GetCharmArgs) (corecharm.ID, error) {
	if !charmNameRegExp.MatchString(args.Name) {
		return "", charmerrors.NameNotValid
	}

	if rev := args.Revision; rev != nil && *rev >= 0 {
		return s.st.GetCharmIDByRevision(ctx, args.Name, *rev)
	}

	return s.st.GetCharmIDByLatestRevision(ctx, args.Name)
}

// IsControllerCharm returns whether the charm is a controller charm.
// This will return true if the charm is a controller charm, and false
// otherwise.
// If the charm does not exist, a NotFound error is returned.
func (s *Service) IsControllerCharm(ctx context.Context, id corecharm.ID) (bool, error) {
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
// If the charm does not exist, a NotFound error is returned.
func (s *Service) SupportsContainers(ctx context.Context, id corecharm.ID) (bool, error) {
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
// If the charm does not exist, a NotFound error is returned.
func (s *Service) IsSubordinateCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	if err := id.Validate(); err != nil {
		return false, fmt.Errorf("charm id: %w", err)
	}
	b, err := s.st.IsSubordinateCharm(ctx, id)
	if err != nil {
		return false, errors.Trace(err)
	}
	return b, nil
}

// GetCharmMetadata returns the metadata for the charm using the charm ID.
// If the charm does not exist, a NotFound error is returned.
func (s *Service) GetCharmMetadata(ctx context.Context, id corecharm.ID) (internalcharm.Meta, error) {
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

// GetCharmManifest returns the manifest for the charm using the charm ID.
// If the charm does not exist, a NotFound error is returned.
func (s *Service) GetCharmManifest(ctx context.Context, id corecharm.ID) (internalcharm.Manifest, error) {
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
// If the charm does not exist, a NotFound error is returned.
func (s *Service) GetCharmActions(ctx context.Context, id corecharm.ID) (internalcharm.Actions, error) {
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
// If the charm does not exist, a NotFound error is returned.
func (s *Service) GetCharmConfig(ctx context.Context, id corecharm.ID) (internalcharm.Config, error) {
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

// GetCharmLXDProfile returns the LXD profile for the charm using the charm ID.
// If the charm does not exist, a NotFound error is returned.
func (s *Service) GetCharmLXDProfile(ctx context.Context, id corecharm.ID) (internalcharm.LXDProfile, error) {
	if err := id.Validate(); err != nil {
		return internalcharm.LXDProfile{}, fmt.Errorf("charm id: %w", err)
	}

	profile, err := s.st.GetCharmLXDProfile(ctx, id)
	if err != nil {
		return internalcharm.LXDProfile{}, errors.Trace(err)
	}

	decoded, err := decodeLXDProfile(profile)
	if err != nil {
		return internalcharm.LXDProfile{}, errors.Trace(err)
	}
	return decoded, nil
}

// IsCharmAvailable returns whether the charm is available for use. This
// indicates if the charm has been uploaded to the controller.
// This will return true if the charm is available, and false otherwise.
// If the charm does not exist, a NotFound error is returned.
func (s *Service) IsCharmAvailable(ctx context.Context, id corecharm.ID) (bool, error) {
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
// If the charm does not exist, a NotFound error is returned.
func (s *Service) SetCharmAvailable(ctx context.Context, id corecharm.ID) error {
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
// If the charm does not exist, then a NotFound error is returned.
func (s *Service) ReserveCharmRevision(ctx context.Context, id corecharm.ID, revision int) (corecharm.ID, error) {
	if err := id.Validate(); err != nil {
		return "", fmt.Errorf("charm id: %w", err)
	}
	if revision < 0 {
		return "", charmerrors.RevisionNotValid
	}

	newID, err := s.st.ReserveCharmRevision(ctx, id, revision)
	if err != nil {
		return "", errors.Trace(err)
	}
	return newID, nil
}

// SetCharm persists the charm metadata, actions, config and manifest to
// state.
func (s *Service) SetCharm(ctx context.Context, charm internalcharm.Charm) (corecharm.ID, error) {
	ch, err := encodeCharm(charm)
	if err != nil {
		return "", fmt.Errorf("encode charm: %w", err)
	}

	charmID, err := s.st.SetCharm(ctx, ch)
	if err != nil {
		return "", errors.Trace(err)
	}

	return charmID, nil

}

// WatchableService provides the API for working with charms and the
// ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(st State, watcherFactory WatcherFactory, logger logger.Logger) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchCharms returns a watcher that observes changes to charms.
func (s *WatchableService) WatchCharms() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewUUIDsWatcher(
		"charm",
		changestream.All,
	)
}
