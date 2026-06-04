// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"strings"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// stubDataSource implements simplestreams.DataSource for testing purposes.
type stubDataSource struct {
	description string
	priority    int
}

func (s *stubDataSource) Description() string { return s.description }
func (s *stubDataSource) Priority() int       { return s.priority }
func (s *stubDataSource) RequireSigned() bool { return false }
func (s *stubDataSource) PublicSigningKey() string {
	return ""
}

func (s *stubDataSource) Fetch(_ context.Context, _ string) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader("")), "", nil
}

func (s *stubDataSource) URL(_ string) (string, error) {
	return "", nil
}

// TestNewImageMetadataFetcher verifies the constructor returns a non-nil
// fetcher implementing the interface.
func (s *serviceSuite) TestNewImageMetadataFetcher(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fetcher := NewImageMetadataFetcher(
		func(ctx context.Context) (ProviderForImageMetadata, error) {
			return nil, errors.New("not called")
		},
		nil,
		loggertesting.WrapCheckLog(c),
	)
	c.Check(fetcher, tc.Not(tc.IsNil))
}

// TestFetchImageMetadataProviderGetterError verifies that if the provider
// getter returns an error, FetchImageMetadata returns a wrapped error.
func (s *serviceSuite) TestFetchImageMetadataProviderGetterError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fetcher := NewImageMetadataFetcher(
		func(ctx context.Context) (ProviderForImageMetadata, error) {
			return nil, errors.New("provider unavailable")
		},
		nil,
		loggertesting.WrapCheckLog(c),
	)

	_, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Arches:   []string{"amd64"},
		Stream:   "released",
		Region:   "us-east-1",
	})
	c.Assert(err, tc.ErrorMatches, `getting bootstrap environ: provider unavailable`)
}

// TestFetchImageMetadataSourcesError verifies that if ImageMetadataSources
// fails, the error is propagated.
func (s *serviceSuite) TestFetchImageMetadataSourcesError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockSource := NewMockimageMetadataSource(ctrl)
	mockProvider := NewMockProviderForImageMetadata(ctrl)

	mockSource.EXPECT().ImageMetadataSources(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("sources broken"))

	fetcher := &imageMetadataFetcher{
		providerGetter: func(ctx context.Context) (ProviderForImageMetadata, error) {
			return mockProvider, nil
		},
		source: mockSource,
		logger: loggertesting.WrapCheckLog(c),
	}

	_, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Stream:   "released",
	})
	c.Assert(err, tc.ErrorMatches, `getting image metadata sources: sources broken`)
}

// TestFetchImageMetadataEmptySources verifies that when no data sources are
// returned, the result is empty and metadata is not saved.
func (s *serviceSuite) TestFetchImageMetadataEmptySources(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockSource := NewMockimageMetadataSource(ctrl)
	mockProvider := NewMockProviderForImageMetadata(ctrl)
	mockSaver := NewMockCloudImageMetadataSaver(ctrl)

	mockSource.EXPECT().ImageMetadataSources(gomock.Any(), gomock.Any()).
		Return([]simplestreams.DataSource{}, nil)

	fetcher := &imageMetadataFetcher{
		providerGetter: func(ctx context.Context) (ProviderForImageMetadata, error) {
			return mockProvider, nil
		},
		metadataSaver: mockSaver,
		source:        mockSource,
		logger:        loggertesting.WrapCheckLog(c),
	}

	result, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Arches:   []string{"amd64"},
		Stream:   "released",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
	// SaveMetadata should NOT be called when no metadata found.
}

// TestFetchImageMetadataFetchErrorContinues verifies that when one data
// source returns an error, the fetcher continues to the next source.
func (s *serviceSuite) TestFetchImageMetadataFetchErrorContinues(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockSource := NewMockimageMetadataSource(ctrl)
	mockProvider := NewMockProviderForImageMetadata(ctrl)
	mockSaver := NewMockCloudImageMetadataSaver(ctrl)

	source1 := &stubDataSource{description: "source-1", priority: 10}
	source2 := &stubDataSource{description: "source-2", priority: 20}

	mockSource.EXPECT().ImageMetadataSources(gomock.Any(), gomock.Any()).
		Return([]simplestreams.DataSource{source1, source2}, nil)

	// First source errors, second succeeds.
	gomock.InOrder(
		mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil, errors.New("network timeout")),
		mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]*imagemetadata.ImageMetadata{
				{Id: "ami-good", Arch: "amd64", RegionName: "us-east-1", Stream: "released", Version: "22.04"},
			}, &simplestreams.ResolveInfo{Source: "custom"}, nil),
	)

	mockSaver.EXPECT().SaveMetadata(gomock.Any(), gomock.Any()).Return(nil)

	fetcher := &imageMetadataFetcher{
		providerGetter: func(ctx context.Context) (ProviderForImageMetadata, error) {
			return mockProvider, nil
		},
		metadataSaver: mockSaver,
		source:        mockSource,
		logger:        loggertesting.WrapCheckLog(c),
	}

	result, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Arches:   []string{"amd64"},
		Stream:   "released",
		Region:   "us-east-1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].ImageID, tc.Equals, "ami-good")
	c.Check(result[0].Source, tc.Equals, "custom")
	c.Check(result[0].Priority, tc.Equals, 20)
}

// TestFetchImageMetadataMultipleSourcesOrdered verifies that results from
// multiple sources retain their source priority for correct ordering.
func (s *serviceSuite) TestFetchImageMetadataMultipleSourcesOrdered(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockSource := NewMockimageMetadataSource(ctrl)
	mockProvider := NewMockProviderForImageMetadata(ctrl)
	mockSaver := NewMockCloudImageMetadataSaver(ctrl)

	source1 := &stubDataSource{description: "high-priority", priority: 50}
	source2 := &stubDataSource{description: "low-priority", priority: 10}

	mockSource.EXPECT().ImageMetadataSources(gomock.Any(), gomock.Any()).
		Return([]simplestreams.DataSource{source1, source2}, nil)

	gomock.InOrder(
		mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]*imagemetadata.ImageMetadata{
				{Id: "ami-hi", Arch: "amd64", RegionName: "us-east-1", Stream: "released", Version: "22.04"},
			}, &simplestreams.ResolveInfo{Source: "official"}, nil),
		mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]*imagemetadata.ImageMetadata{
				{Id: "ami-lo", Arch: "amd64", RegionName: "us-east-1", Stream: "released", Version: "22.04"},
			}, &simplestreams.ResolveInfo{Source: "community"}, nil),
	)

	mockSaver.EXPECT().SaveMetadata(gomock.Any(), gomock.Any()).Return(nil)

	fetcher := &imageMetadataFetcher{
		providerGetter: func(ctx context.Context) (ProviderForImageMetadata, error) {
			return mockProvider, nil
		},
		metadataSaver: mockSaver,
		source:        mockSource,
		logger:        loggertesting.WrapCheckLog(c),
	}

	result, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Arches:   []string{"amd64"},
		Stream:   "released",
		Region:   "us-east-1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 2)
	// First result is from source1 (priority 50).
	c.Check(result[0].ImageID, tc.Equals, "ami-hi")
	c.Check(result[0].Priority, tc.Equals, 50)
	c.Check(result[0].Source, tc.Equals, "official")
	// Second result is from source2 (priority 10).
	c.Check(result[1].ImageID, tc.Equals, "ami-lo")
	c.Check(result[1].Priority, tc.Equals, 10)
	c.Check(result[1].Source, tc.Equals, "community")
}

// TestFetchImageMetadataStreamFallback verifies that when an image has an
// empty stream, the constraint's stream is used as fallback.
func (s *serviceSuite) TestFetchImageMetadataStreamFallback(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockSource := NewMockimageMetadataSource(ctrl)
	mockProvider := NewMockProviderForImageMetadata(ctrl)
	mockSaver := NewMockCloudImageMetadataSaver(ctrl)

	source1 := &stubDataSource{description: "source-1", priority: 10}

	mockSource.EXPECT().ImageMetadataSources(gomock.Any(), gomock.Any()).
		Return([]simplestreams.DataSource{source1}, nil)

	// Image metadata returned with empty stream.
	mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*imagemetadata.ImageMetadata{
			{Id: "ami-nostream", Arch: "amd64", RegionName: "us-east-1", Stream: "", Version: "22.04"},
		}, &simplestreams.ResolveInfo{Source: "official"}, nil)

	mockSaver.EXPECT().SaveMetadata(gomock.Any(), gomock.Eq([]cloudimagemetadata.Metadata{
		{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Region:  "us-east-1",
				Arch:    "amd64",
				Source:  "official",
				Stream:  "daily",
				Version: "22.04",
			},
			Priority: 10,
			ImageID:  "ami-nostream",
		},
	})).Return(nil)

	fetcher := &imageMetadataFetcher{
		providerGetter: func(ctx context.Context) (ProviderForImageMetadata, error) {
			return mockProvider, nil
		},
		metadataSaver: mockSaver,
		source:        mockSource,
		logger:        loggertesting.WrapCheckLog(c),
	}

	result, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Arches:   []string{"amd64"},
		Stream:   "daily",
		Region:   "us-east-1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	// Stream should be filled from the constraint.
	c.Check(result[0].Stream, tc.Equals, "daily")
}

// TestFetchImageMetadataStreamPreserved verifies that when an image has an
// explicit stream, it is preserved and not overridden by the constraint.
func (s *serviceSuite) TestFetchImageMetadataStreamPreserved(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockSource := NewMockimageMetadataSource(ctrl)
	mockProvider := NewMockProviderForImageMetadata(ctrl)
	mockSaver := NewMockCloudImageMetadataSaver(ctrl)

	source1 := &stubDataSource{description: "source-1", priority: 10}

	mockSource.EXPECT().ImageMetadataSources(gomock.Any(), gomock.Any()).
		Return([]simplestreams.DataSource{source1}, nil)

	// Image metadata returned with explicit "released" stream.
	mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*imagemetadata.ImageMetadata{
			{Id: "ami-released", Arch: "amd64", RegionName: "us-east-1", Stream: "released", Version: "22.04"},
		}, &simplestreams.ResolveInfo{Source: "official"}, nil)

	mockSaver.EXPECT().SaveMetadata(gomock.Any(), gomock.Any()).Return(nil)

	fetcher := &imageMetadataFetcher{
		providerGetter: func(ctx context.Context) (ProviderForImageMetadata, error) {
			return mockProvider, nil
		},
		metadataSaver: mockSaver,
		source:        mockSource,
		logger:        loggertesting.WrapCheckLog(c),
	}

	// Constraint says "daily" but image metadata says "released".
	result, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Arches:   []string{"amd64"},
		Stream:   "daily",
		Region:   "us-east-1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	// Stream from image metadata is preserved.
	c.Check(result[0].Stream, tc.Equals, "released")
}

// TestFetchImageMetadataSaveError verifies that a save error is logged but
// does not prevent results from being returned.
func (s *serviceSuite) TestFetchImageMetadataSaveError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockSource := NewMockimageMetadataSource(ctrl)
	mockProvider := NewMockProviderForImageMetadata(ctrl)
	mockSaver := NewMockCloudImageMetadataSaver(ctrl)

	source1 := &stubDataSource{description: "source-1", priority: 10}

	mockSource.EXPECT().ImageMetadataSources(gomock.Any(), gomock.Any()).
		Return([]simplestreams.DataSource{source1}, nil)

	mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*imagemetadata.ImageMetadata{
			{Id: "ami-123", Arch: "amd64", RegionName: "us-east-1", Stream: "released", Version: "22.04"},
		}, &simplestreams.ResolveInfo{Source: "official"}, nil)

	// Save fails — should not prevent results.
	mockSaver.EXPECT().SaveMetadata(gomock.Any(), gomock.Any()).
		Return(errors.New("database write failed"))

	fetcher := &imageMetadataFetcher{
		providerGetter: func(ctx context.Context) (ProviderForImageMetadata, error) {
			return mockProvider, nil
		},
		metadataSaver: mockSaver,
		source:        mockSource,
		logger:        loggertesting.WrapCheckLog(c),
	}

	result, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Arches:   []string{"amd64"},
		Stream:   "released",
		Region:   "us-east-1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].ImageID, tc.Equals, "ami-123")
}

// TestFetchImageMetadataAllFieldsMapped verifies that all fields from
// ImageMetadata are correctly mapped to domain CloudImageMetadata.
func (s *serviceSuite) TestFetchImageMetadataAllFieldsMapped(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockSource := NewMockimageMetadataSource(ctrl)
	mockProvider := NewMockProviderForImageMetadata(ctrl)
	mockSaver := NewMockCloudImageMetadataSaver(ctrl)

	source1 := &stubDataSource{description: "source-1", priority: 42}

	mockSource.EXPECT().ImageMetadataSources(gomock.Any(), gomock.Any()).
		Return([]simplestreams.DataSource{source1}, nil)

	mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*imagemetadata.ImageMetadata{
			{
				Id:         "ami-full",
				Arch:       "arm64",
				RegionName: "eu-west-1",
				Stream:     "daily",
				Version:    "24.04",
				VirtType:   "hvm",
				Storage:    "ssd",
			},
		}, &simplestreams.ResolveInfo{Source: "custom-source"}, nil)

	mockSaver.EXPECT().SaveMetadata(gomock.Any(), gomock.Any()).Return(nil)

	fetcher := &imageMetadataFetcher{
		providerGetter: func(ctx context.Context) (ProviderForImageMetadata, error) {
			return mockProvider, nil
		},
		metadataSaver: mockSaver,
		source:        mockSource,
		logger:        loggertesting.WrapCheckLog(c),
	}

	result, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"24.04"},
		Arches:   []string{"arm64"},
		Stream:   "daily",
		Region:   "eu-west-1",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].ImageID, tc.Equals, "ami-full")
	c.Check(result[0].Arch, tc.Equals, "arm64")
	c.Check(result[0].Region, tc.Equals, "eu-west-1")
	c.Check(result[0].Stream, tc.Equals, "daily")
	c.Check(result[0].Version, tc.Equals, "24.04")
	c.Check(result[0].VirtType, tc.Equals, "hvm")
	c.Check(result[0].RootStorageType, tc.Equals, "ssd")
	c.Check(result[0].Source, tc.Equals, "custom-source")
	c.Check(result[0].Priority, tc.Equals, 42)
}

// TestFetchImageMetadataConstraintPassedToFetch verifies that the constraint
// releases, arches, stream, region, and endpoint are passed to the underlying
// Fetch call via the ImageConstraint's CloudSpec.
func (s *serviceSuite) TestFetchImageMetadataConstraintPassedToFetch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockSource := NewMockimageMetadataSource(ctrl)
	mockProvider := NewMockProviderForImageMetadata(ctrl)
	mockSaver := NewMockCloudImageMetadataSaver(ctrl)

	source1 := &stubDataSource{description: "source-1", priority: 10}

	mockSource.EXPECT().ImageMetadataSources(gomock.Any(), gomock.Any()).
		Return([]simplestreams.DataSource{source1}, nil)

	// Verify the constraint parameters are correctly passed through.
	mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context,
			_ imagemetadata.SimplestreamsFetcher,
			sources []simplestreams.DataSource,
			cons *imagemetadata.ImageConstraint,
		) ([]*imagemetadata.ImageMetadata, *simplestreams.ResolveInfo, error) {
			c.Check(cons.Releases, tc.DeepEquals, []string{"22.04", "24.04"})
			c.Check(cons.Arches, tc.DeepEquals, []string{"amd64", "arm64"})
			c.Check(cons.Stream, tc.Equals, "daily")
			c.Check(cons.Region, tc.Equals, "eu-west-1")
			c.Check(cons.Endpoint, tc.Equals, "https://ec2.eu-west-1.amazonaws.com")
			c.Assert(sources, tc.HasLen, 1)
			return nil, nil, errors.New("expected")
		})

	fetcher := &imageMetadataFetcher{
		providerGetter: func(ctx context.Context) (ProviderForImageMetadata, error) {
			return mockProvider, nil
		},
		metadataSaver: mockSaver,
		source:        mockSource,
		logger:        loggertesting.WrapCheckLog(c),
	}

	_, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"22.04", "24.04"},
		Arches:   []string{"amd64", "arm64"},
		Stream:   "daily",
		Region:   "eu-west-1",
		Endpoint: "https://ec2.eu-west-1.amazonaws.com",
	})
	// Fetch error is swallowed (continues to next source), result is empty.
	c.Assert(err, tc.ErrorIsNil)
}

// TestFetchImageMetadataAllSourcesFail verifies that if all sources fail,
// the result is empty with no error.
func (s *serviceSuite) TestFetchImageMetadataAllSourcesFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockSource := NewMockimageMetadataSource(ctrl)
	mockProvider := NewMockProviderForImageMetadata(ctrl)
	mockSaver := NewMockCloudImageMetadataSaver(ctrl)

	source1 := &stubDataSource{description: "s1", priority: 10}
	source2 := &stubDataSource{description: "s2", priority: 20}

	mockSource.EXPECT().ImageMetadataSources(gomock.Any(), gomock.Any()).
		Return([]simplestreams.DataSource{source1, source2}, nil)

	mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil, errors.New("fail 1"))
	mockSource.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil, errors.New("fail 2"))

	fetcher := &imageMetadataFetcher{
		providerGetter: func(ctx context.Context) (ProviderForImageMetadata, error) {
			return mockProvider, nil
		},
		metadataSaver: mockSaver,
		source:        mockSource,
		logger:        loggertesting.WrapCheckLog(c),
	}

	result, err := fetcher.FetchImageMetadata(c.Context(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Stream:   "released",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}
