// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams_test

import (
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
)

func TestJsonSuite(t *testing.T) {
	tc.Run(t, &jsonSuite{})
}

func TestSimplestreamsSuite(t *testing.T) {
	tc.Run(t, &simplestreamsSuite{
		LocalLiveSimplestreamsSuite: sstesting.LocalLiveSimplestreamsSuite{
			Source:         sstesting.VerifyDefaultCloudDataSource("test", "test:"),
			RequireSigned:  false,
			DataType:       "image-ids",
			StreamsVersion: "v1",
			ValidConstraint: sstesting.NewTestConstraint(simplestreams.LookupParams{
				CloudSpec: simplestreams.CloudSpec{
					Region:   "us-east-1",
					Endpoint: "https://ec2.us-east-1.amazonaws.com",
				},
				Releases: []string{"12.04"},
				Arches:   []string{"amd64", "arm"},
			}),
		},
	})
}

type simplestreamsSuite struct {
	sstesting.TestDataSuite
	sstesting.LocalLiveSimplestreamsSuite
}

func (s *simplestreamsSuite) SetUpSuite(c *tc.C) {
	s.LocalLiveSimplestreamsSuite.SetUpSuite(c)
	s.TestDataSuite.SetUpSuite(c)
}

func (s *simplestreamsSuite) TearDownSuite(c *tc.C) {
	s.TestDataSuite.TearDownSuite(c)
	s.LocalLiveSimplestreamsSuite.TearDownSuite(c)
}

func (s *simplestreamsSuite) TestGetProductsPath(c *tc.C) {
	indexRef, err := s.GetIndexRef(c, sstesting.Index_v1)
	c.Assert(err, tc.ErrorIsNil)
	path, err := indexRef.GetProductsPath(s.ValidConstraint)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.Equals, "streams/v1/image_metadata.json")
}

func (*simplestreamsSuite) TestExtractCatalogsForProductsAcceptsNil(c *tc.C) {
	empty := simplestreams.CloudMetadata{}
	c.Check(simplestreams.ExtractCatalogsForProducts(empty, nil), tc.HasLen, 0)
}

func (*simplestreamsSuite) TestExtractCatalogsForProductsReturnsMatch(c *tc.C) {
	metadata := simplestreams.CloudMetadata{
		Products: map[string]simplestreams.MetadataCatalog{
			"foo": {},
		},
	}
	c.Check(
		simplestreams.ExtractCatalogsForProducts(metadata, []string{"foo"}),
		tc.DeepEquals,
		[]simplestreams.MetadataCatalog{metadata.Products["foo"]})
}

func (*simplestreamsSuite) TestExtractCatalogsForProductsIgnoresNonMatches(c *tc.C) {
	metadata := simplestreams.CloudMetadata{
		Products: map[string]simplestreams.MetadataCatalog{
			"one-product": {},
		},
	}
	absentProducts := []string{"another-product"}
	c.Check(simplestreams.ExtractCatalogsForProducts(metadata, absentProducts), tc.HasLen, 0)
}

func (*simplestreamsSuite) TestExtractCatalogsForProductsPreservesOrder(c *tc.C) {
	products := map[string]simplestreams.MetadataCatalog{
		"1": {},
		"2": {},
		"3": {},
		"4": {},
	}

	metadata := simplestreams.CloudMetadata{Products: products}

	c.Check(
		simplestreams.ExtractCatalogsForProducts(metadata, []string{"1", "3", "4", "2"}),
		tc.DeepEquals,
		[]simplestreams.MetadataCatalog{
			products["1"],
			products["3"],
			products["4"],
			products["2"],
		})
}

func (*simplestreamsSuite) TestExtractIndexesAcceptsEmpty(c *tc.C) {
	ind := simplestreams.Indices{}
	c.Check(simplestreams.ExtractIndexes(ind, nil), tc.HasLen, 0)
}

func (*simplestreamsSuite) TestExtractIndexesReturnsIndex(c *tc.C) {
	metadata := simplestreams.IndexMetadata{}
	ind := simplestreams.Indices{Indexes: map[string]*simplestreams.IndexMetadata{"foo": &metadata}}
	c.Check(simplestreams.ExtractIndexes(ind, nil), tc.DeepEquals, simplestreams.IndexMetadataSlice{&metadata})
}

func (*simplestreamsSuite) TestExtractIndexesReturnsAllIndexes(c *tc.C) {
	ind := simplestreams.Indices{
		Indexes: map[string]*simplestreams.IndexMetadata{
			"foo": {},
			"bar": {},
		},
	}

	array := simplestreams.ExtractIndexes(ind, nil)

	c.Assert(array, tc.HasLen, len(ind.Indexes))
	c.Check(array[0], tc.NotNil)
	c.Check(array[1], tc.NotNil)
	c.Check(array[0], tc.Not(tc.Equals), array[1])
	c.Check(
		(array[0] == ind.Indexes["foo"]),
		tc.Not(tc.Equals),
		(array[1] == ind.Indexes["foo"]))
	c.Check(
		(array[0] == ind.Indexes["bar"]),
		tc.Not(tc.Equals),
		(array[1] == ind.Indexes["bar"]))
}

func (*simplestreamsSuite) TestExtractIndexesReturnsSpecifiedIndexes(c *tc.C) {
	ind := simplestreams.Indices{
		Indexes: map[string]*simplestreams.IndexMetadata{
			"foo":    {},
			"bar":    {},
			"foobar": {},
		},
	}

	array := simplestreams.ExtractIndexes(ind, []string{"foobar"})
	c.Assert(array, tc.HasLen, 1)
	c.Assert(array[0], tc.Equals, ind.Indexes["foobar"])
}

func (*simplestreamsSuite) TestHasCloudAcceptsNil(c *tc.C) {
	metadata := simplestreams.IndexMetadata{Clouds: nil}
	c.Check(simplestreams.HasCloud(metadata, simplestreams.CloudSpec{}), tc.IsTrue)
}

func (*simplestreamsSuite) TestHasCloudFindsMatch(c *tc.C) {
	metadata := simplestreams.IndexMetadata{
		Clouds: []simplestreams.CloudSpec{
			{Region: "r1", Endpoint: "http://e1"},
			{Region: "r2", Endpoint: "http://e2"},
		},
	}
	c.Check(simplestreams.HasCloud(metadata, metadata.Clouds[1]), tc.IsTrue)
}

func (*simplestreamsSuite) TestHasCloudFindsMatchWithTrailingSlash(c *tc.C) {
	metadata := simplestreams.IndexMetadata{
		Clouds: []simplestreams.CloudSpec{
			{Region: "r1", Endpoint: "http://e1/"},
			{Region: "r2", Endpoint: "http://e2"},
		},
	}
	spec := simplestreams.CloudSpec{Region: "r1", Endpoint: "http://e1"}
	c.Check(simplestreams.HasCloud(metadata, spec), tc.IsTrue)
	spec = simplestreams.CloudSpec{Region: "r1", Endpoint: "http://e1/"}
	c.Check(simplestreams.HasCloud(metadata, spec), tc.IsTrue)
	spec = simplestreams.CloudSpec{Region: "r2", Endpoint: "http://e2/"}
	c.Check(simplestreams.HasCloud(metadata, spec), tc.IsTrue)
}

func (*simplestreamsSuite) TestHasCloudReturnsFalseIfCloudsDoNotMatch(c *tc.C) {
	metadata := simplestreams.IndexMetadata{
		Clouds: []simplestreams.CloudSpec{
			{Region: "r1", Endpoint: "http://e1"},
			{Region: "r2", Endpoint: "http://e2"},
		},
	}
	otherCloud := simplestreams.CloudSpec{Region: "r9", Endpoint: "http://e9"}
	c.Check(simplestreams.HasCloud(metadata, otherCloud), tc.IsFalse)
}

func (*simplestreamsSuite) TestHasCloudRequiresIdenticalRegion(c *tc.C) {
	metadata := simplestreams.IndexMetadata{
		Clouds: []simplestreams.CloudSpec{
			{Region: "around", Endpoint: "http://nearby"},
		},
	}
	similarCloud := metadata.Clouds[0]
	similarCloud.Region = "elsewhere"
	c.Assert(similarCloud, tc.Not(tc.Equals), metadata.Clouds[0])

	c.Check(simplestreams.HasCloud(metadata, similarCloud), tc.IsFalse)
}

func (*simplestreamsSuite) TestHasCloudRequiresIdenticalEndpoint(c *tc.C) {
	metadata := simplestreams.IndexMetadata{
		Clouds: []simplestreams.CloudSpec{
			{Region: "around", Endpoint: "http://nearby"},
		},
	}
	similarCloud := metadata.Clouds[0]
	similarCloud.Endpoint = "http://far"
	c.Assert(similarCloud, tc.Not(tc.Equals), metadata.Clouds[0])

	c.Check(simplestreams.HasCloud(metadata, similarCloud), tc.IsFalse)
}

func (*simplestreamsSuite) TestHasProductAcceptsNils(c *tc.C) {
	metadata := simplestreams.IndexMetadata{}
	c.Check(simplestreams.HasProduct(metadata, nil), tc.IsFalse)
}

func (*simplestreamsSuite) TestHasProductFindsMatchingProduct(c *tc.C) {
	metadata := simplestreams.IndexMetadata{ProductIds: []string{"x", "y", "z"}}
	c.Check(
		simplestreams.HasProduct(metadata, []string{"a", "b", metadata.ProductIds[1]}),
		tc.Equals,
		true)
}

func (*simplestreamsSuite) TestHasProductReturnsFalseIfProductsDoNotMatch(c *tc.C) {
	metadata := simplestreams.IndexMetadata{ProductIds: []string{"x", "y", "z"}}
	c.Check(simplestreams.HasProduct(metadata, []string{"a", "b", "c"}), tc.IsFalse)
}

func (*simplestreamsSuite) TestFilterReturnsNothingForEmptyArray(c *tc.C) {
	empty := simplestreams.IndexMetadataSlice{}
	c.Check(
		simplestreams.Filter(empty, func(*simplestreams.IndexMetadata) bool { return true }),
		tc.HasLen,
		0)
}

func (*simplestreamsSuite) TestFilterRemovesNonMatches(c *tc.C) {
	array := simplestreams.IndexMetadataSlice{&simplestreams.IndexMetadata{}}
	c.Check(
		simplestreams.Filter(array, func(*simplestreams.IndexMetadata) bool { return false }),
		tc.HasLen,
		0)
}

func (*simplestreamsSuite) TestFilterIncludesMatches(c *tc.C) {
	metadata := simplestreams.IndexMetadata{}
	array := simplestreams.IndexMetadataSlice{&metadata}
	c.Check(
		simplestreams.Filter(array, func(*simplestreams.IndexMetadata) bool { return true }),
		tc.DeepEquals,
		simplestreams.IndexMetadataSlice{&metadata})
}

func (*simplestreamsSuite) TestFilterLeavesOriginalUnchanged(c *tc.C) {
	item1 := simplestreams.IndexMetadata{CloudName: "aws"}
	item2 := simplestreams.IndexMetadata{CloudName: "openstack"}
	array := simplestreams.IndexMetadataSlice{&item1, &item2}

	result := simplestreams.Filter(array, func(metadata *simplestreams.IndexMetadata) bool {
		return metadata.CloudName == "aws"
	})
	// This exercises both the "leave out" and the "include" code paths.
	c.Assert(result, tc.HasLen, 1)

	// The original, however, has not changed.
	c.Assert(array, tc.HasLen, 2)
	c.Check(array, tc.DeepEquals, simplestreams.IndexMetadataSlice{&item1, &item2})
}

func (*simplestreamsSuite) TestFilterPreservesOrder(c *tc.C) {
	array := simplestreams.IndexMetadataSlice{
		&simplestreams.IndexMetadata{CloudName: "aws"},
		&simplestreams.IndexMetadata{CloudName: "maas"},
		&simplestreams.IndexMetadata{CloudName: "openstack"},
	}

	c.Check(
		simplestreams.Filter(array, func(metadata *simplestreams.IndexMetadata) bool { return true }),
		tc.DeepEquals,
		array)
}

func (*simplestreamsSuite) TestFilterCombinesMatchesAndNonMatches(c *tc.C) {
	array := simplestreams.IndexMetadataSlice{
		&simplestreams.IndexMetadata{Format: "1.0"},
		&simplestreams.IndexMetadata{Format: "1.1"},
		&simplestreams.IndexMetadata{Format: "2.0"},
		&simplestreams.IndexMetadata{Format: "2.1"},
	}

	dotOFormats := simplestreams.Filter(array, func(metadata *simplestreams.IndexMetadata) bool {
		return strings.HasSuffix(metadata.Format, ".0")
	})

	c.Check(dotOFormats, tc.DeepEquals, simplestreams.IndexMetadataSlice{array[0], array[2]})
}

// countingSource is used to check that a DataSource has been queried.
type countingSource struct {
	simplestreams.DataSource
	count int
}

func (s *countingSource) URL(path string) (string, error) {
	s.count++
	return s.DataSource.URL(path)
}

func (s *simplestreamsSuite) TestGetMetadataNoMatching(c *tc.C) {
	source := &countingSource{
		DataSource: sstesting.VerifyDefaultCloudDataSource("test", "test:/daily"),
	}
	sources := []simplestreams.DataSource{source, source, source}
	constraint := sstesting.NewTestConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{
			Region:   "us-east-1",
			Endpoint: "https://ec2.us-east-1.amazonaws.com",
		},
		Releases: []string{"12.04"},
		Arches:   []string{"not-a-real-arch"}, // never matches
	})
	params := simplestreams.GetMetadataParams{
		StreamsVersion:   s.StreamsVersion,
		LookupConstraint: constraint,
		ValueParams:      simplestreams.ValueParams{DataType: "image-ids"},
	}

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	items, resolveInfo, err := ss.GetMetadata(c.Context(), sources, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(items, tc.HasLen, 0)
	c.Assert(resolveInfo, tc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    false,
		IndexURL:  "test:/daily/streams/v1/index.json",
		MirrorURL: "",
	})

	// There should be 4 calls to each data-source:
	// one for .sjson, one for .json, repeated for legacy vs new index files.
	c.Assert(source.count, tc.Equals, 4*len(sources))
}

func (s *simplestreamsSuite) TestMetadataCatalog(c *tc.C) {
	metadata := s.AssertGetMetadata(c)
	c.Check(len(metadata.Products), tc.Equals, 6)
	c.Check(len(metadata.Aliases), tc.Equals, 1)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	c.Check(len(metadataCatalog.Items), tc.Equals, 2)
	c.Check(metadataCatalog.Version, tc.Equals, "12.04")
	c.Check(metadataCatalog.Release, tc.Equals, "precise")
	c.Check(metadataCatalog.Arch, tc.Equals, "amd64")
	c.Check(metadataCatalog.RegionName, tc.Equals, "au-east-1")
	c.Check(metadataCatalog.Endpoint, tc.Equals, "https://somewhere")
}

func (s *simplestreamsSuite) TestItemCollection(c *tc.C) {
	ic := s.AssertGetItemCollections(c, "20121218")
	c.Check(ic.RegionName, tc.Equals, "au-east-2")
	c.Check(ic.Endpoint, tc.Equals, "https://somewhere-else")
	c.Assert(len(ic.Items) > 0, tc.IsTrue)
	ti := ic.Items["usww2he"].(*sstesting.TestItem)
	c.Check(ti.Id, tc.Equals, "ami-442ea674")
	c.Check(ti.Storage, tc.Equals, "ebs")
	c.Check(ti.VirtType, tc.Equals, "hvm")
	c.Check(ti.RegionName, tc.Equals, "us-east-1")
	c.Check(ti.Endpoint, tc.Equals, "https://ec2.us-east-1.amazonaws.com")
}

func (s *simplestreamsSuite) TestDenormalisationFromCollection(c *tc.C) {
	ic := s.AssertGetItemCollections(c, "20121218")
	ti := ic.Items["usww1pe"].(*sstesting.TestItem)
	c.Check(ti.RegionName, tc.Equals, ic.RegionName)
	c.Check(ti.Endpoint, tc.Equals, ic.Endpoint)
}

func (s *simplestreamsSuite) TestDenormalisationFromCatalog(c *tc.C) {
	metadata := s.AssertGetMetadata(c)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	ic := metadataCatalog.Items["20111111"]
	ti := ic.Items["usww3pe"].(*sstesting.TestItem)
	c.Check(ti.RegionName, tc.Equals, metadataCatalog.RegionName)
	c.Check(ti.Endpoint, tc.Equals, metadataCatalog.Endpoint)
}

func (s *simplestreamsSuite) TestDenormalisationFromTopLevel(c *tc.C) {
	metadata := s.AssertGetMetadata(c)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:14.04:amd64"]
	ic := metadataCatalog.Items["20140118"]
	ti := ic.Items["nzww1pe"].(*sstesting.TestItem)
	c.Check(ti.RegionName, tc.Equals, metadata.RegionName)
	c.Check(ti.Endpoint, tc.Equals, metadata.Endpoint)
}

func (s *simplestreamsSuite) TestDealiasing(c *tc.C) {
	metadata := s.AssertGetMetadata(c)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	ic := metadataCatalog.Items["20121218"]
	ti := ic.Items["usww3he"].(*sstesting.TestItem)
	c.Check(ti.RegionName, tc.Equals, "us-west-3")
	c.Check(ti.Endpoint, tc.Equals, "https://ec2.us-west-3.amazonaws.com")
}

type storageVirtTest struct {
	product, coll, item, storage, virt string
}

func (s *simplestreamsSuite) TestStorageVirtFromTopLevel(c *tc.C) {
	s.assertImageMetadata(c,
		storageVirtTest{"com.ubuntu.cloud:server:13.04:amd64", "20160318", "nzww1pe", "ebs", "pv"},
	)
}

func (s *simplestreamsSuite) TestStorageVirtFromCatalog(c *tc.C) {
	s.assertImageMetadata(c,
		storageVirtTest{"com.ubuntu.cloud:server:14.10:amd64", "20160218", "nzww1pe", "ebs", "pv"},
	)
}

func (s *simplestreamsSuite) TestStorageVirtFromCollection(c *tc.C) {
	s.assertImageMetadata(c,
		storageVirtTest{"com.ubuntu.cloud:server:12.10:amd64", "20160118", "nzww1pe", "ebs", "pv"},
	)
}

func (s *simplestreamsSuite) TestStorageVirtFromItem(c *tc.C) {
	s.assertImageMetadata(c,
		storageVirtTest{"com.ubuntu.cloud:server:14.04:amd64", "20140118", "nzww1pe", "ssd", "hvm"},
	)
}

func (s *simplestreamsSuite) assertImageMetadata(c *tc.C, one storageVirtTest) {
	metadata := s.AssertGetMetadata(c)
	metadataCatalog := metadata.Products[one.product]
	ic := metadataCatalog.Items[one.coll]
	ti := ic.Items[one.item].(*sstesting.TestItem)
	c.Check(ti.Storage, tc.Equals, one.storage)
	c.Check(ti.VirtType, tc.Equals, one.virt)
}

var getMirrorTests = []struct {
	region    string
	endpoint  string
	err       string
	mirrorURL string
	path      string
}{{
	// defaults
	mirrorURL: "http://some-mirror/",
	path:      "com.ubuntu.juju:download.json",
}, {
	// default mirror index entry
	region:    "some-region",
	endpoint:  "https://some-endpoint.com",
	mirrorURL: "http://big-mirror/",
	path:      "big:download.json",
}, {
	// endpoint with trailing "/"
	region:    "some-region",
	endpoint:  "https://some-endpoint.com/",
	mirrorURL: "http://big-mirror/",
	path:      "big:download.json",
}}

func (s *simplestreamsSuite) TestGetMirrorMetadata(c *tc.C) {
	for i, t := range getMirrorTests {
		c.Logf("test %d", i)
		if t.region == "" {
			t.region = "us-east-2"
		}
		if t.endpoint == "" {
			t.endpoint = "https://ec2.us-east-2.amazonaws.com"
		}
		cloud := simplestreams.CloudSpec{t.region, t.endpoint}
		params := simplestreams.ValueParams{
			DataType:        "content-download",
			MirrorContentId: "com.ubuntu.juju:released:agents",
		}
		ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
		indexRef, err := ss.GetIndexWithFormat(
			c.Context(), s.Source, s.IndexPath(), sstesting.Index_v1,
			simplestreams.MirrorsPath("v1"), s.RequireSigned, cloud, params)
		if !c.Check(err, tc.ErrorIsNil) {
			continue
		}
		if t.err != "" {
			c.Check(err, tc.ErrorMatches, t.err)
			continue
		}
		if !c.Check(err, tc.ErrorIsNil) {
			continue
		}
		mirrorURL, err := indexRef.Source.URL("")
		if !c.Check(err, tc.ErrorIsNil) {
			continue
		}
		c.Check(mirrorURL, tc.Equals, t.mirrorURL)
		c.Check(indexRef.MirroredProductsPath, tc.Equals, t.path)
	}
}
