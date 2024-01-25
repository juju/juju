// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/juju/juju/core/objectstore"
)

type (
	pruneListFunc   func(ctx context.Context) ([]objectstore.Metadata, []string, error)
	pruneLockFunc   func(ctx context.Context, hash string, f func(ctx context.Context) error) error
	pruneDeleteFunc func(ctx context.Context, hash string) error
)

type pruner struct {
	logger       Logger
	list         pruneListFunc
	withLock     pruneLockFunc
	deleteObject pruneDeleteFunc
}

func newPruner(logger Logger, list pruneListFunc, withLock pruneLockFunc, deleteObject pruneDeleteFunc) *pruner {
	return &pruner{
		logger:       logger,
		list:         list,
		withLock:     withLock,
		deleteObject: deleteObject,
	}
}

// Prune will remove any files that are no longer referenced by the metadata
// service.
func (p *pruner) Prune(ctx context.Context) error {
	p.logger.Debugf("pruning objects from storage")

	metadata, objects, err := p.list(ctx)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}

	// Create a map of all the hashes that we know about.
	hashes := make(map[string]struct{})
	for _, m := range metadata {
		hashes[m.Hash] = struct{}{}
	}

	// Remove any objects that we don't know about.
	for _, object := range objects {
		if _, ok := hashes[object]; ok {
			p.logger.Tracef("object %q is referenced", object)
			continue
		}

		p.logger.Debugf("attempting to remove unreferenced object %q", object)

		if err := p.pruneObjectWithLock(ctx, object); err != nil {
			p.logger.Infof("failed to remove unreferenced object %q: %v, will try again later", object, err)
			continue
		}

		p.logger.Debugf("removed unreferenced object %q", object)
	}

	return nil
}

func (t *pruner) pruneObjectWithLock(ctx context.Context, hash string) error {
	return t.withLock(ctx, hash, func(ctx context.Context) error {
		return t.deleteObject(ctx, hash)
	})
}

// jitter returns a random duration between 0.5 and 1 times the given duration.
func jitter(d time.Duration) time.Duration {
	h := float64(d) * 0.5
	r := rand.Float64() * h
	return time.Duration(r + h)
}
