// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"math/rand/v2"
	"regexp"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	domainobjectstore "github.com/juju/juju/domain/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
)

const (
	hashLength = 64

	// minHashPrefixLength is the minimum length of the hash prefix. We allow
	// either 7 or 8 characters.
	minHashPrefixLength = 7
)

var (
	// The hashRegexp is used to validate the SHA256 hash.
	hashRegexp = regexp.MustCompile(`^[a-f0-9]{64}$`)

	// The hashPrefixRegexp is used to validate the SHA256 hash prefix.
	// Note: this should include the length of the hash prefix.
	hashPrefixRegexp = regexp.MustCompile(`^[a-f0-9]{7,8}$`)
)

// State describes retrieval and persistence methods for the objectstore.
type State interface {
	// GetMetadata returns the persistence metadata for the specified path.
	GetMetadata(ctx context.Context, path string) (objectstore.Metadata, error)

	// GetMetadataBySHA256 returns the persistence metadata for the object
	// with SHA256.
	GetMetadataBySHA256(ctx context.Context, sha256 string) (objectstore.Metadata, error)

	// GetMetadataBySHA256Prefix returns the persistence metadata for the object
	// with SHA256 starting with the provided prefix.
	GetMetadataBySHA256Prefix(ctx context.Context, sha256 string) (objectstore.Metadata, error)

	// PutMetadata adds a new specified path for the persistence metadata.
	PutMetadata(ctx context.Context, uuid string, metadata objectstore.Metadata) (string, error)

	// GetControllerIDHints returns the controller ID hints for the specified
	// SHA384. This is used to indicate which controller might have the object
	// with the specified SHA384, which can be used for optimization in certain
	// scenarios.
	GetControllerIDHints(ctx context.Context, sha384 string) ([]string, error)

	// PutMetadataWithControllerIDHint adds a new specified path for the
	// persistence metadata with a controller ID hint. This is used to route the
	// request to the correct controller in a multi-controller environment.
	PutMetadataWithControllerIDHint(ctx context.Context, uuid string, metadata objectstore.Metadata, controllerIDHint string) (string, error)

	// AddControllerIDHint adds a controller ID hint for the specified SHA384.
	// This is used to indicate that a controller might have the object with the
	// specified SHA384, which can be used for optimization in certain
	// scenarios.
	AddControllerIDHint(ctx context.Context, sha384 string, controllerIDHint string) error

	// ListMetadata returns the persistence metadata for all paths.
	ListMetadata(ctx context.Context) ([]objectstore.Metadata, error)

	// RemoveMetadata removes the specified path for the persistence metadata.
	RemoveMetadata(ctx context.Context, path string) error

	// InitialWatchStatement returns the table and the initial watch statement
	// for the persistence metadata.
	InitialWatchStatement() (string, string)
}

// DrainingState describes retrieval and persistence methods for the draining
// phase of the object store.
type DrainingState interface {
	State

	// GetActiveDrainingInfo returns the active draining info of the object
	// store.
	GetActiveDrainingInfo(ctx context.Context) (domainobjectstore.DrainingInfo, error)

	// StartDraining initiates the draining process for the object store.
	StartDraining(ctx context.Context, uuid string) error

	// SetDrainingPhase sets the phase of the object store to draining.
	SetDrainingPhase(ctx context.Context, uuid string, phase objectstore.Phase) error

	// InitialWatchDrainingTable returns the table for the draining phase.
	InitialWatchDrainingTable() string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. Change-log events will be emitted only if
	// the filter accepts them, and dispatching the notifications via the
	// Changes channel. A filter option is required, though additional filter
	// options can be provided.
	NewNamespaceWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		summary string,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNotifyWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// Service provides the API for working with the objectstore.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// GetMetadata returns the persistence metadata for the specified path.
func (s *Service) GetMetadata(ctx context.Context, path string) (objectstore.Metadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	metadata, err := s.st.GetMetadata(ctx, path)
	if err != nil {
		return objectstore.Metadata{}, errors.Errorf("retrieving metadata %s: %w", path, err)
	}
	return objectstore.Metadata{
		Path:   metadata.Path,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
		Size:   metadata.Size,
	}, nil
}

// GetMetadataBySHA256 returns the persistence metadata for the object
// with SHA256 starting with the provided prefix.
func (s *Service) GetMetadataBySHA256(ctx context.Context, sha256 string) (objectstore.Metadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(sha256) != hashLength {
		return objectstore.Metadata{}, objectstoreerrors.ErrInvalidHashLength
	} else if !hashRegexp.MatchString(sha256) {
		return objectstore.Metadata{}, objectstoreerrors.ErrInvalidHash
	}

	metadata, err := s.st.GetMetadataBySHA256(ctx, sha256)
	if err != nil {
		return objectstore.Metadata{}, errors.Errorf("retrieving metadata with sha256 %s: %w", sha256, err)
	}
	return objectstore.Metadata{
		Path:   metadata.Path,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
		Size:   metadata.Size,
	}, nil
}

// GetMetadataBySHA256Prefix returns the persistence metadata for the object
// with SHA256 starting with the provided prefix.
func (s *Service) GetMetadataBySHA256Prefix(ctx context.Context, sha256Prefix string) (objectstore.Metadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(sha256Prefix) < minHashPrefixLength {
		return objectstore.Metadata{}, errors.Errorf("minimum has prefix length is %d: %w", minHashPrefixLength, objectstoreerrors.ErrHashPrefixTooShort)
	} else if !hashPrefixRegexp.MatchString(sha256Prefix) {
		return objectstore.Metadata{}, errors.Errorf("%s: %w", sha256Prefix, objectstoreerrors.ErrInvalidHashPrefix)
	}

	metadata, err := s.st.GetMetadataBySHA256Prefix(ctx, sha256Prefix)
	if err != nil {
		return objectstore.Metadata{}, errors.Errorf("retrieving metadata with sha256 %s: %w", sha256Prefix, err)
	}
	return objectstore.Metadata{
		Path:   metadata.Path,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
		Size:   metadata.Size,
	}, nil
}

// ListMetadata returns the persistence metadata for all paths.
func (s *Service) ListMetadata(ctx context.Context) ([]objectstore.Metadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	metadata, err := s.st.ListMetadata(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving metadata: %w", err)
	}
	m := make([]objectstore.Metadata, len(metadata))
	for i, v := range metadata {
		m[i] = objectstore.Metadata{
			Path:   v.Path,
			SHA256: v.SHA256,
			SHA384: v.SHA384,
			Size:   v.Size,
		}
	}
	return m, nil
}

// PutMetadata adds a new specified path for the persistence metadata. If any
// hash is missing, a [objectstoreerrors.ErrMissingHash] error is returned. It
// is expected that the caller supplies both hashes or none and they should be
// consistent with the object. That's the caller's responsibility.
func (s *Service) PutMetadata(ctx context.Context, metadata objectstore.Metadata) (objectstore.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// If you have one hash, you must have the other.
	if h1, h2 := metadata.SHA384, metadata.SHA256; h1 != "" && h2 == "" {
		return "", errors.Errorf("missing hash256: %w", objectstoreerrors.ErrMissingHash)
	} else if h1 == "" && h2 != "" {
		return "", errors.Errorf("missing hash384: %w", objectstoreerrors.ErrMissingHash)
	}

	uuid, err := objectstore.NewUUID()
	if err != nil {
		return "", err
	}

	resultUUID, err := s.st.PutMetadata(ctx, uuid.String(), objectstore.Metadata{
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
		Path:   metadata.Path,
		Size:   metadata.Size,
	})
	if err != nil {
		return "", errors.Errorf("adding path %s: %w", metadata.Path, err)
	}

	return objectstore.UUID(resultUUID), nil
}

// GetControllerIDHints returns the controller ID hints for the specified
// SHA384. This is used to indicate which controllers might have the object with
// the specified SHA384, which can be used for optimization in certain
// scenarios.
//
// The hints are returned in random order to ensure that no particular
// controller is favored, which helps to distribute the load more evenly across
// controllers. If there are no hints, an
// [objectstoreerrors.ErrNoHints] error is returned, and the caller
// can decide how to handle this case, for example by trying to retrieve from
// any controller.
func (s *Service) GetControllerIDHints(ctx context.Context, sha384 string) ([]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if sha384 == "" {
		return nil, errors.Errorf("missing hash384").Add(objectstoreerrors.ErrMissingHash)
	}

	hints, err := s.st.GetControllerIDHints(ctx, sha384)
	if err != nil {
		return nil, errors.Errorf("getting controller ID hint for sha384 %s: %w", sha384, err)
	}

	// Handle the case where there are no hints.
	if len(hints) == 0 {
		return nil, objectstoreerrors.ErrNoHints
	}

	// Shuffle them if we have multiple hints to help distribute the load more
	// evenly across controllers.
	rand.Shuffle(len(hints), func(i, j int) {
		hints[i], hints[j] = hints[j], hints[i]
	})

	return hints, nil
}

// PutMetadataWithControllerIDHint adds a new specified path for the persistence
// metadata, along with the controller ID hint. If any hash is missing, a
// [objectstoreerrors.ErrMissingHash] error is returned. It is expected that the
// caller supplies both hashes or none and they should be consistent with the
// object. That's the caller's responsibility.
func (s *Service) PutMetadataWithControllerIDHint(
	ctx context.Context,
	metadata objectstore.Metadata,
	controllerID string,
) (objectstore.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// If you have one hash, you must have the other.
	if h1, h2 := metadata.SHA384, metadata.SHA256; h1 != "" && h2 == "" {
		return "", errors.Errorf("missing hash256").Add(objectstoreerrors.ErrMissingHash)
	} else if h1 == "" && h2 != "" {
		return "", errors.Errorf("missing hash384").Add(objectstoreerrors.ErrMissingHash)
	}

	if controllerID == "" {
		return "", errors.Errorf("missing controller ID hint").Add(objectstoreerrors.ErrMissingControllerID)
	}

	uuid, err := objectstore.NewUUID()
	if err != nil {
		return "", err
	}

	pUUID, err := s.st.PutMetadataWithControllerIDHint(ctx, uuid.String(), objectstore.Metadata{
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
		Path:   metadata.Path,
		Size:   metadata.Size,
	}, controllerID)
	if err != nil {
		return "", errors.Errorf("adding path %s: %w", metadata.Path, err)
	}

	return objectstore.UUID(pUUID), nil
}

// AddControllerIDHint adds a controller ID hint for the specified SHA384.
// This is used to indicate that a controller might have the object with the
// specified SHA384, which can be used for optimization in certain
// scenarios.
func (s *Service) AddControllerIDHint(ctx context.Context, sha384 string, controllerID string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if sha384 == "" {
		return errors.Errorf("missing hash384").Add(objectstoreerrors.ErrMissingHash)
	}
	if controllerID == "" {
		return errors.Errorf("missing controller ID hint").Add(objectstoreerrors.ErrMissingControllerID)
	}

	if err := s.st.AddControllerIDHint(ctx, sha384, controllerID); err != nil {
		return errors.Errorf("adding controller ID hint for sha384 %s: %w", sha384, err)
	}
	return nil
}

// RemoveMetadata removes the specified path for the persistence metadata.
func (s *Service) RemoveMetadata(ctx context.Context, path string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.RemoveMetadata(ctx, path); err != nil {
		return errors.Errorf("removing path %s: %w", path, err)
	}
	return nil
}

// WatchableService provides the API for working with the objectstore
// and the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(st State, watcherFactory WatcherFactory) *WatchableService {
	return &WatchableService{
		Service: Service{
			st: st,
		},
		watcherFactory: watcherFactory,
	}
}

// Watch returns a watcher that emits the path changes that either have been
// added or removed.
func (s *WatchableService) Watch(ctx context.Context) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	table, stmt := s.st.InitialWatchStatement()
	return s.watcherFactory.NewNamespaceWatcher(
		ctx,
		eventsource.InitialNamespaceChanges(stmt),
		"objectstore watcher",
		eventsource.NamespaceFilter(table, changestream.All),
	)
}

// WatchableDrainingService provides the API for working with the objectstore
// and the ability to create watchers and drain the object store.
type WatchableDrainingService struct {
	WatchableService
	st DrainingState
}

// NewWatchableDrainingService returns a new service reference wrapping the
// input state.
func NewWatchableDrainingService(st DrainingState, watcherFactory WatcherFactory) *WatchableDrainingService {
	return &WatchableDrainingService{
		WatchableService: WatchableService{
			Service: Service{
				st: st,
			},
			watcherFactory: watcherFactory,
		},
		st: st,
	}
}

// SetDrainingPhase sets the phase of the object store to draining.
func (s *WatchableDrainingService) SetDrainingPhase(ctx context.Context, phase objectstore.Phase) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !phase.IsValid() {
		return errors.Errorf("invalid phase %q", phase)
	}

	phaseInfo, err := s.st.GetActiveDrainingInfo(ctx)
	if errors.Is(err, objectstoreerrors.ErrDrainingPhaseNotFound) {
		uuid, err := objectstore.NewUUID()
		if err != nil {
			return errors.Errorf("creating new uuid: %w", err)
		}

		return s.st.StartDraining(ctx, uuid.String())
	} else if err != nil {
		return errors.Errorf("getting active draining phase: %w", err)
	}

	current := objectstore.Phase(phaseInfo.Phase)
	if _, err := current.TransitionTo(phase); errors.Is(err, objectstore.ErrTerminalPhase) {
		return nil
	} else if err != nil {
		return errors.Errorf("transitioning phase: %w", err)
	}

	// Set the phase in the state.
	if err := s.st.SetDrainingPhase(ctx, phaseInfo.UUID, phase); err != nil {
		return errors.Errorf("setting draining phase: %w", err)
	}
	return nil
}

// GetDrainingPhase returns the phase of the object store.
func (s *WatchableDrainingService) GetDrainingPhase(ctx context.Context) (objectstore.Phase, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	info, err := s.st.GetActiveDrainingInfo(ctx)
	if errors.Is(err, objectstoreerrors.ErrDrainingPhaseNotFound) {
		return objectstore.PhaseUnknown, nil
	} else if err != nil {
		return "", errors.Errorf("getting draining phase: %w", err)
	}
	return objectstore.Phase(info.Phase), nil
}

// WatchDraining returns a watcher that watches the draining phase of the
// object store. The watcher emits the phase changes that either have been
// added or removed.
func (s *WatchableDrainingService) WatchDraining(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	table := s.st.InitialWatchDrainingTable()
	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"objectstore draining watcher",
		eventsource.NamespaceFilter(table, changestream.All),
	)
}
