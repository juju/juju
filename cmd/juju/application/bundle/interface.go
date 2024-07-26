// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"context"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// ModelExtractor provides everything we need to build a
// bundlechanges.Model from a model API connection.
type ModelExtractor interface {
	GetAnnotations(ctx context.Context, tags []string) ([]params.AnnotationsGetResult, error)
	GetConstraints(ctx context.Context, applications ...string) ([]constraints.Value, error)
	GetConfig(ctx context.Context, applications ...string) ([]map[string]interface{}, error)
	Sequences(ctx context.Context) (map[string]int, error)
}

// BundleDataSource is implemented by types that can parse bundle data into a
// list of composable parts.
type BundleDataSource interface {
	Parts() []*charm.BundleDataPart
	BundleBytes() []byte
	BasePath() string
	ResolveInclude(path string) ([]byte, error)
}
