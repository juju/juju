// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"regexp"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
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
	PutMetadata(ctx context.Context, metadata objectstore.Metadata) (objectstore.UUID, error)

	// ListMetadata returns the persistence metadata for all paths.
	ListMetadata(ctx context.Context) ([]objectstore.Metadata, error)

	// RemoveMetadata removes the specified path for the persistence metadata.
	RemoveMetadata(ctx context.Context, path string) error

	// InitialWatchStatement returns the table and the initial watch statement
	// for the persistence metadata.
	InitialWatchStatement() (string, string)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. Change-log events will be emitted only if
	// the filter accepts them, and dispatching the notifications via the
	// Changes channel. A filter option is required, though additional filter
	// options can be provided.
	NewNamespaceWatcher(
		initialQuery eventsource.NamespaceQuery,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
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
func (s *Service) GetMetadata(ctx context.Context, path string) (_ objectstore.Metadata, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

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
func (s *Service) GetMetadataBySHA256(ctx context.Context, sha256 string) (_ objectstore.Metadata, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

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
func (s *Service) GetMetadataBySHA256Prefix(ctx context.Context, sha256Prefix string) (_ objectstore.Metadata, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

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
func (s *Service) ListMetadata(ctx context.Context) (_ []objectstore.Metadata, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

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
func (s *Service) PutMetadata(ctx context.Context, metadata objectstore.Metadata) (_ objectstore.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	// If you have one hash, you must have the other.
	if h1, h2 := metadata.SHA384, metadata.SHA256; h1 != "" && h2 == "" {
		return "", errors.Errorf("missing hash256: %w", objectstoreerrors.ErrMissingHash)
	} else if h1 == "" && h2 != "" {
		return "", errors.Errorf("missing hash384: %w", objectstoreerrors.ErrMissingHash)
	}

	uuid, err := s.st.PutMetadata(ctx, objectstore.Metadata{
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
		Path:   metadata.Path,
		Size:   metadata.Size,
	})
	if err != nil {
		return "", errors.Errorf("adding path %s: %w", metadata.Path, err)
	}

	return uuid, nil
}

// RemoveMetadata removes the specified path for the persistence metadata.
func (s *Service) RemoveMetadata(ctx context.Context, path string) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

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
func (s *WatchableService) Watch() (_ watcher.StringsWatcher, err error) {
	// TODO (stickupkid): Wire up context.Context to the watcher.
	table, stmt := s.st.InitialWatchStatement()
	return s.watcherFactory.NewNamespaceWatcher(
		eventsource.InitialNamespaceChanges(stmt),
		eventsource.NamespaceFilter(table, changestream.All),
	)
}
