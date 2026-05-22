// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/errors"
)

// ProviderForImageMetadata defines the provider interface needed for
// image metadata lookups.
type ProviderForImageMetadata interface {
	environs.BootstrapEnviron
}

// CloudImageMetadataSaver persists cloud image metadata.
type CloudImageMetadataSaver interface {
	// SaveMetadata saves the provided cloud image metadata to persistent
	// storage.
	SaveMetadata(ctx context.Context, metadata []cloudimagemetadata.Metadata) error
}

// imageMetadataSource abstracts the simplestreams data source operations so
// that the fetcher can be tested without real network calls.
type imageMetadataSource interface {
	// ImageMetadataSources returns the data sources for the given environ.
	ImageMetadataSources(
		env environs.BootstrapEnviron,
		factory simplestreams.DataSourceFactory,
	) ([]simplestreams.DataSource, error)

	// Fetch retrieves image metadata from the given data sources matching
	// the constraint.
	Fetch(
		ctx context.Context,
		fetcher imagemetadata.SimplestreamsFetcher,
		sources []simplestreams.DataSource,
		cons *imagemetadata.ImageConstraint,
	) ([]*imagemetadata.ImageMetadata, *simplestreams.ResolveInfo, error)
}

// defaultImageMetadataSource is the production implementation that delegates
// to the environs and imagemetadata packages.
type defaultImageMetadataSource struct{}

func (defaultImageMetadataSource) ImageMetadataSources(
	env environs.BootstrapEnviron,
	factory simplestreams.DataSourceFactory,
) ([]simplestreams.DataSource, error) {
	return environs.ImageMetadataSources(env, factory)
}

func (defaultImageMetadataSource) Fetch(
	ctx context.Context,
	fetcher imagemetadata.SimplestreamsFetcher,
	sources []simplestreams.DataSource,
	cons *imagemetadata.ImageConstraint,
) ([]*imagemetadata.ImageMetadata, *simplestreams.ResolveInfo, error) {
	return imagemetadata.Fetch(ctx, fetcher, sources, cons)
}

// imageMetadataFetcher is a concrete implementation of ImageMetadataFetcher
// that looks up image metadata from simplestreams data sources.
type imageMetadataFetcher struct {
	providerGetter providertracker.ProviderGetter[ProviderForImageMetadata]
	metadataSaver  CloudImageMetadataSaver
	source         imageMetadataSource
	logger         logger.Logger
}

// NewImageMetadataFetcher returns a new ImageMetadataFetcher that uses
// simplestreams to look up image metadata.
func NewImageMetadataFetcher(
	providerGetter providertracker.ProviderGetter[ProviderForImageMetadata],
	metadataSaver CloudImageMetadataSaver,
	logger logger.Logger,
) ImageMetadataFetcher {
	return &imageMetadataFetcher{
		providerGetter: providerGetter,
		metadataSaver:  metadataSaver,
		source:         defaultImageMetadataSource{},
		logger:         logger,
	}
}

// FetchImageMetadata fetches image metadata from simplestreams data sources
// for the given image constraint. It saves any fetched metadata via the
// metadata saver for future cache lookups.
func (f *imageMetadataFetcher) FetchImageMetadata(
	ctx context.Context,
	constraint provisioner.ImageConstraint,
) ([]provisioner.CloudImageMetadata, error) {
	environ, err := f.providerGetter(ctx)
	if err != nil {
		return nil, errors.Errorf("getting bootstrap environ: %w", err)
	}

	ssFactory := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	sources, err := f.source.ImageMetadataSources(environ, ssFactory)
	if err != nil {
		return nil, errors.Errorf("getting image metadata sources: %w", err)
	}

	// Build the simplestreams image constraint.
	ssConstraint, err := imagemetadata.NewImageConstraint(
		simplestreams.LookupParams{
			CloudSpec: simplestreams.CloudSpec{
				Region:   constraint.Region,
				Endpoint: constraint.Endpoint,
			},
			Releases: constraint.Releases,
			Arches:   constraint.Arches,
			Stream:   constraint.Stream,
		},
		constraint.ImageID,
	)
	if err != nil {
		return nil, errors.Errorf("building image constraint: %w", err)
	}

	var allMetadata []cloudimagemetadata.Metadata
	for _, source := range sources {
		f.logger.Debugf(ctx, "looking in data source %v", source.Description())
		found, info, err := f.source.Fetch(ctx, ssFactory, []simplestreams.DataSource{source}, ssConstraint)
		if err != nil {
			// Do not stop looking in other data sources if there is an
			// issue here.
			f.logger.Warningf(ctx, "encountered %v while getting published images metadata from %v", err, source.Description())
			continue
		}

		for _, m := range found {
			md := cloudimagemetadata.Metadata{
				MetadataAttributes: cloudimagemetadata.MetadataAttributes{
					Region:          m.RegionName,
					Arch:            m.Arch,
					VirtType:        m.VirtType,
					RootStorageType: m.Storage,
					Source:          info.Source,
					Stream:          m.Stream,
					Version:         m.Version,
				},
				Priority: source.Priority(),
				ImageID:  m.Id,
			}
			if md.Stream == "" {
				md.Stream = constraint.Stream
			}
			allMetadata = append(allMetadata, md)
		}
	}

	// Save fetched metadata for future cache lookups.
	if len(allMetadata) > 0 {
		if err := f.metadataSaver.SaveMetadata(ctx, allMetadata); err != nil {
			f.logger.Warningf(ctx, "failed to save published image metadata: %v", err)
		}
	}

	// Convert to domain types.
	result := make([]provisioner.CloudImageMetadata, len(allMetadata))
	for i, m := range allMetadata {
		result[i] = provisioner.CloudImageMetadata{
			ImageID:         m.ImageID,
			Region:          m.Region,
			Arch:            m.Arch,
			VirtType:        m.VirtType,
			RootStorageType: m.RootStorageType,
			Stream:          m.Stream,
			Version:         m.Version,
			Source:          m.Source,
			Priority:        m.Priority,
		}
	}
	return result, nil
}
