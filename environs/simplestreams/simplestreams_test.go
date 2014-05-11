// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams_test

import (
	"bytes"
	"strings"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/simplestreams"
	sstesting "launchpad.net/juju-core/environs/simplestreams/testing"
	"launchpad.net/juju-core/utils"
)

func Test(t *testing.T) {
	registerSimpleStreamsTests()
	gc.Suite(&signingSuite{})
	gc.Suite(&jsonSuite{})
	gc.TestingT(t)
}

func registerSimpleStreamsTests() {
	gc.Suite(&simplestreamsSuite{
		LocalLiveSimplestreamsSuite: sstesting.LocalLiveSimplestreamsSuite{
			Source:        simplestreams.NewURLDataSource("test", "test:", utils.VerifySSLHostnames),
			RequireSigned: false,
			DataType:      "image-ids",
			ValidConstraint: sstesting.NewTestConstraint(simplestreams.LookupParams{
				CloudSpec: simplestreams.CloudSpec{
					Region:   "us-east-1",
					Endpoint: "https://ec2.us-east-1.amazonaws.com",
				},
				Series: []string{"precise"},
				Arches: []string{"amd64", "arm"},
			}),
		},
	})
}

type simplestreamsSuite struct {
	sstesting.TestDataSuite
	sstesting.LocalLiveSimplestreamsSuite
}

func (s *simplestreamsSuite) SetUpSuite(c *gc.C) {
	s.LocalLiveSimplestreamsSuite.SetUpSuite(c)
	s.TestDataSuite.SetUpSuite(c)
}

func (s *simplestreamsSuite) TearDownSuite(c *gc.C) {
	s.TestDataSuite.TearDownSuite(c)
	s.LocalLiveSimplestreamsSuite.TearDownSuite(c)
}

func (s *simplestreamsSuite) TestGetProductsPath(c *gc.C) {
	indexRef, err := s.GetIndexRef(sstesting.Index_v1)
	c.Assert(err, gc.IsNil)
	path, err := indexRef.GetProductsPath(s.ValidConstraint)
	c.Assert(err, gc.IsNil)
	c.Assert(path, gc.Equals, "streams/v1/image_metadata.json")
}

func (*simplestreamsSuite) TestExtractCatalogsForProductsAcceptsNil(c *gc.C) {
	empty := simplestreams.CloudMetadata{}
	c.Check(simplestreams.ExtractCatalogsForProducts(empty, nil), gc.HasLen, 0)
}

func (*simplestreamsSuite) TestExtractCatalogsForProductsReturnsMatch(c *gc.C) {
	metadata := simplestreams.CloudMetadata{
		Products: map[string]simplestreams.MetadataCatalog{
			"foo": {},
		},
	}
	c.Check(
		simplestreams.ExtractCatalogsForProducts(metadata, []string{"foo"}),
		gc.DeepEquals,
		[]simplestreams.MetadataCatalog{metadata.Products["foo"]})
}

func (*simplestreamsSuite) TestExtractCatalogsForProductsIgnoresNonMatches(c *gc.C) {
	metadata := simplestreams.CloudMetadata{
		Products: map[string]simplestreams.MetadataCatalog{
			"one-product": {},
		},
	}
	absentProducts := []string{"another-product"}
	c.Check(simplestreams.ExtractCatalogsForProducts(metadata, absentProducts), gc.HasLen, 0)
}

func (*simplestreamsSuite) TestExtractCatalogsForProductsPreservesOrder(c *gc.C) {
	products := map[string]simplestreams.MetadataCatalog{
		"1": {},
		"2": {},
		"3": {},
		"4": {},
	}

	metadata := simplestreams.CloudMetadata{Products: products}

	c.Check(
		simplestreams.ExtractCatalogsForProducts(metadata, []string{"1", "3", "4", "2"}),
		gc.DeepEquals,
		[]simplestreams.MetadataCatalog{
			products["1"],
			products["3"],
			products["4"],
			products["2"],
		})
}

func (*simplestreamsSuite) TestExtractIndexesAcceptsNil(c *gc.C) {
	ind := simplestreams.Indices{}
	c.Check(simplestreams.ExtractIndexes(ind), gc.HasLen, 0)
}

func (*simplestreamsSuite) TestExtractIndexesReturnsIndex(c *gc.C) {
	metadata := simplestreams.IndexMetadata{}
	ind := simplestreams.Indices{Indexes: map[string]*simplestreams.IndexMetadata{"foo": &metadata}}
	c.Check(simplestreams.ExtractIndexes(ind), gc.DeepEquals, simplestreams.IndexMetadataSlice{&metadata})
}

func (*simplestreamsSuite) TestExtractIndexesReturnsAllIndexes(c *gc.C) {
	ind := simplestreams.Indices{
		Indexes: map[string]*simplestreams.IndexMetadata{
			"foo": {},
			"bar": {},
		},
	}

	array := simplestreams.ExtractIndexes(ind)

	c.Assert(array, gc.HasLen, len(ind.Indexes))
	c.Check(array[0], gc.NotNil)
	c.Check(array[1], gc.NotNil)
	c.Check(array[0], gc.Not(gc.Equals), array[1])
	c.Check(
		(array[0] == ind.Indexes["foo"]),
		gc.Not(gc.Equals),
		(array[1] == ind.Indexes["foo"]))
	c.Check(
		(array[0] == ind.Indexes["bar"]),
		gc.Not(gc.Equals),
		(array[1] == ind.Indexes["bar"]))
}

func (*simplestreamsSuite) TestHasCloudAcceptsNil(c *gc.C) {
	metadata := simplestreams.IndexMetadata{Clouds: nil}
	c.Check(simplestreams.HasCloud(metadata, simplestreams.CloudSpec{}), jc.IsTrue)
}

func (*simplestreamsSuite) TestHasCloudFindsMatch(c *gc.C) {
	metadata := simplestreams.IndexMetadata{
		Clouds: []simplestreams.CloudSpec{
			{Region: "r1", Endpoint: "http://e1"},
			{Region: "r2", Endpoint: "http://e2"},
		},
	}
	c.Check(simplestreams.HasCloud(metadata, metadata.Clouds[1]), jc.IsTrue)
}

func (*simplestreamsSuite) TestHasCloudFindsMatchWithTrailingSlash(c *gc.C) {
	metadata := simplestreams.IndexMetadata{
		Clouds: []simplestreams.CloudSpec{
			{Region: "r1", Endpoint: "http://e1/"},
			{Region: "r2", Endpoint: "http://e2"},
		},
	}
	spec := simplestreams.CloudSpec{Region: "r1", Endpoint: "http://e1"}
	c.Check(simplestreams.HasCloud(metadata, spec), jc.IsTrue)
	spec = simplestreams.CloudSpec{Region: "r1", Endpoint: "http://e1/"}
	c.Check(simplestreams.HasCloud(metadata, spec), jc.IsTrue)
	spec = simplestreams.CloudSpec{Region: "r2", Endpoint: "http://e2/"}
	c.Check(simplestreams.HasCloud(metadata, spec), jc.IsTrue)
}

func (*simplestreamsSuite) TestHasCloudReturnsFalseIfCloudsDoNotMatch(c *gc.C) {
	metadata := simplestreams.IndexMetadata{
		Clouds: []simplestreams.CloudSpec{
			{Region: "r1", Endpoint: "http://e1"},
			{Region: "r2", Endpoint: "http://e2"},
		},
	}
	otherCloud := simplestreams.CloudSpec{Region: "r9", Endpoint: "http://e9"}
	c.Check(simplestreams.HasCloud(metadata, otherCloud), jc.IsFalse)
}

func (*simplestreamsSuite) TestHasCloudRequiresIdenticalRegion(c *gc.C) {
	metadata := simplestreams.IndexMetadata{
		Clouds: []simplestreams.CloudSpec{
			{Region: "around", Endpoint: "http://nearby"},
		},
	}
	similarCloud := metadata.Clouds[0]
	similarCloud.Region = "elsewhere"
	c.Assert(similarCloud, gc.Not(gc.Equals), metadata.Clouds[0])

	c.Check(simplestreams.HasCloud(metadata, similarCloud), jc.IsFalse)
}

func (*simplestreamsSuite) TestHasCloudRequiresIdenticalEndpoint(c *gc.C) {
	metadata := simplestreams.IndexMetadata{
		Clouds: []simplestreams.CloudSpec{
			{Region: "around", Endpoint: "http://nearby"},
		},
	}
	similarCloud := metadata.Clouds[0]
	similarCloud.Endpoint = "http://far"
	c.Assert(similarCloud, gc.Not(gc.Equals), metadata.Clouds[0])

	c.Check(simplestreams.HasCloud(metadata, similarCloud), jc.IsFalse)
}

func (*simplestreamsSuite) TestHasProductAcceptsNils(c *gc.C) {
	metadata := simplestreams.IndexMetadata{}
	c.Check(simplestreams.HasProduct(metadata, nil), jc.IsFalse)
}

func (*simplestreamsSuite) TestHasProductFindsMatchingProduct(c *gc.C) {
	metadata := simplestreams.IndexMetadata{ProductIds: []string{"x", "y", "z"}}
	c.Check(
		simplestreams.HasProduct(metadata, []string{"a", "b", metadata.ProductIds[1]}),
		gc.Equals,
		true)
}

func (*simplestreamsSuite) TestHasProductReturnsFalseIfProductsDoNotMatch(c *gc.C) {
	metadata := simplestreams.IndexMetadata{ProductIds: []string{"x", "y", "z"}}
	c.Check(simplestreams.HasProduct(metadata, []string{"a", "b", "c"}), jc.IsFalse)
}

func (*simplestreamsSuite) TestFilterReturnsNothingForEmptyArray(c *gc.C) {
	empty := simplestreams.IndexMetadataSlice{}
	c.Check(
		simplestreams.Filter(empty, func(*simplestreams.IndexMetadata) bool { return true }),
		gc.HasLen,
		0)
}

func (*simplestreamsSuite) TestFilterRemovesNonMatches(c *gc.C) {
	array := simplestreams.IndexMetadataSlice{&simplestreams.IndexMetadata{}}
	c.Check(
		simplestreams.Filter(array, func(*simplestreams.IndexMetadata) bool { return false }),
		gc.HasLen,
		0)
}

func (*simplestreamsSuite) TestFilterIncludesMatches(c *gc.C) {
	metadata := simplestreams.IndexMetadata{}
	array := simplestreams.IndexMetadataSlice{&metadata}
	c.Check(
		simplestreams.Filter(array, func(*simplestreams.IndexMetadata) bool { return true }),
		gc.DeepEquals,
		simplestreams.IndexMetadataSlice{&metadata})
}

func (*simplestreamsSuite) TestFilterLeavesOriginalUnchanged(c *gc.C) {
	item1 := simplestreams.IndexMetadata{CloudName: "aws"}
	item2 := simplestreams.IndexMetadata{CloudName: "openstack"}
	array := simplestreams.IndexMetadataSlice{&item1, &item2}

	result := simplestreams.Filter(array, func(metadata *simplestreams.IndexMetadata) bool {
		return metadata.CloudName == "aws"
	})
	// This exercises both the "leave out" and the "include" code paths.
	c.Assert(result, gc.HasLen, 1)

	// The original, however, has not changed.
	c.Assert(array, gc.HasLen, 2)
	c.Check(array, gc.DeepEquals, simplestreams.IndexMetadataSlice{&item1, &item2})
}

func (*simplestreamsSuite) TestFilterPreservesOrder(c *gc.C) {
	array := simplestreams.IndexMetadataSlice{
		&simplestreams.IndexMetadata{CloudName: "aws"},
		&simplestreams.IndexMetadata{CloudName: "maas"},
		&simplestreams.IndexMetadata{CloudName: "openstack"},
	}

	c.Check(
		simplestreams.Filter(array, func(metadata *simplestreams.IndexMetadata) bool { return true }),
		gc.DeepEquals,
		array)
}

func (*simplestreamsSuite) TestFilterCombinesMatchesAndNonMatches(c *gc.C) {
	array := simplestreams.IndexMetadataSlice{
		&simplestreams.IndexMetadata{Format: "1.0"},
		&simplestreams.IndexMetadata{Format: "1.1"},
		&simplestreams.IndexMetadata{Format: "2.0"},
		&simplestreams.IndexMetadata{Format: "2.1"},
	}

	dotOFormats := simplestreams.Filter(array, func(metadata *simplestreams.IndexMetadata) bool {
		return strings.HasSuffix(metadata.Format, ".0")
	})

	c.Check(dotOFormats, gc.DeepEquals, simplestreams.IndexMetadataSlice{array[0], array[2]})
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

func (s *simplestreamsSuite) TestGetMetadataNoMatching(c *gc.C) {
	source := &countingSource{
		DataSource: simplestreams.NewURLDataSource(
			"test", "test:/daily", utils.VerifySSLHostnames,
		),
	}
	sources := []simplestreams.DataSource{source, source, source}
	params := simplestreams.ValueParams{DataType: "image-ids"}
	constraint := sstesting.NewTestConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{
			Region:   "us-east-1",
			Endpoint: "https://ec2.us-east-1.amazonaws.com",
		},
		Series: []string{"precise"},
		Arches: []string{"not-a-real-arch"}, // never matches
	})

	items, resolveInfo, err := simplestreams.GetMetadata(
		sources,
		simplestreams.DefaultIndexPath,
		constraint,
		false,
		params,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(items, gc.HasLen, 0)
	c.Assert(resolveInfo, gc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    false,
		IndexURL:  "test:/daily/streams/v1/index.json",
		MirrorURL: "",
	})

	// There should be 2 calls to each data-source:
	// one for .sjson, one for .json.
	c.Assert(source.count, gc.Equals, 2*len(sources))
}

func (s *simplestreamsSuite) TestMetadataCatalog(c *gc.C) {
	metadata := s.AssertGetMetadata(c)
	c.Check(len(metadata.Products), gc.Equals, 2)
	c.Check(len(metadata.Aliases), gc.Equals, 1)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	c.Check(len(metadataCatalog.Items), gc.Equals, 2)
	c.Check(metadataCatalog.Series, gc.Equals, "precise")
	c.Check(metadataCatalog.Version, gc.Equals, "12.04")
	c.Check(metadataCatalog.Arch, gc.Equals, "amd64")
	c.Check(metadataCatalog.RegionName, gc.Equals, "au-east-1")
	c.Check(metadataCatalog.Endpoint, gc.Equals, "https://somewhere")
}

func (s *simplestreamsSuite) TestItemCollection(c *gc.C) {
	ic := s.AssertGetItemCollections(c, "20121218")
	c.Check(ic.RegionName, gc.Equals, "au-east-2")
	c.Check(ic.Endpoint, gc.Equals, "https://somewhere-else")
	c.Assert(len(ic.Items) > 0, jc.IsTrue)
	ti := ic.Items["usww2he"].(*sstesting.TestItem)
	c.Check(ti.Id, gc.Equals, "ami-442ea674")
	c.Check(ti.Storage, gc.Equals, "ebs")
	c.Check(ti.VirtType, gc.Equals, "hvm")
	c.Check(ti.RegionName, gc.Equals, "us-east-1")
	c.Check(ti.Endpoint, gc.Equals, "https://ec2.us-east-1.amazonaws.com")
}

func (s *simplestreamsSuite) TestDenormalisationFromCollection(c *gc.C) {
	ic := s.AssertGetItemCollections(c, "20121218")
	ti := ic.Items["usww1pe"].(*sstesting.TestItem)
	c.Check(ti.RegionName, gc.Equals, ic.RegionName)
	c.Check(ti.Endpoint, gc.Equals, ic.Endpoint)
}

func (s *simplestreamsSuite) TestDenormalisationFromCatalog(c *gc.C) {
	metadata := s.AssertGetMetadata(c)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	ic := metadataCatalog.Items["20111111"]
	ti := ic.Items["usww3pe"].(*sstesting.TestItem)
	c.Check(ti.RegionName, gc.Equals, metadataCatalog.RegionName)
	c.Check(ti.Endpoint, gc.Equals, metadataCatalog.Endpoint)
}

func (s *simplestreamsSuite) TestDealiasing(c *gc.C) {
	metadata := s.AssertGetMetadata(c)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	ic := metadataCatalog.Items["20121218"]
	ti := ic.Items["usww3he"].(*sstesting.TestItem)
	c.Check(ti.RegionName, gc.Equals, "us-west-3")
	c.Check(ti.Endpoint, gc.Equals, "https://ec2.us-west-3.amazonaws.com")
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

func (s *simplestreamsSuite) TestGetMirrorMetadata(c *gc.C) {
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
			MirrorContentId: "com.ubuntu.juju:released:tools",
		}
		indexRef, err := simplestreams.GetIndexWithFormat(
			s.Source, s.IndexPath(), sstesting.Index_v1, s.RequireSigned, cloud, params)
		if !c.Check(err, gc.IsNil) {
			continue
		}
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
			continue
		}
		if !c.Check(err, gc.IsNil) {
			continue
		}
		mirrorURL, err := indexRef.Source.URL("")
		if !c.Check(err, gc.IsNil) {
			continue
		}
		c.Check(mirrorURL, gc.Equals, t.mirrorURL)
		c.Check(indexRef.MirroredProductsPath, gc.Equals, t.path)
	}
}

var testSigningKey = `-----BEGIN PGP PRIVATE KEY BLOCK-----
Version: GnuPG v1.4.10 (GNU/Linux)

lQHYBE2rFNoBBADFwqWQIW/DSqcB4yCQqnAFTJ27qS5AnB46ccAdw3u4Greeu3Bp
idpoHdjULy7zSKlwR1EA873dO/k/e11Ml3dlAFUinWeejWaK2ugFP6JjiieSsrKn
vWNicdCS4HTWn0X4sjl0ZiAygw6GNhqEQ3cpLeL0g8E9hnYzJKQ0LWJa0QARAQAB
AAP/TB81EIo2VYNmTq0pK1ZXwUpxCrvAAIG3hwKjEzHcbQznsjNvPUihZ+NZQ6+X
0HCfPAdPkGDCLCb6NavcSW+iNnLTrdDnSI6+3BbIONqWWdRDYJhqZCkqmG6zqSfL
IdkJgCw94taUg5BWP/AAeQrhzjChvpMQTVKQL5mnuZbUCeMCAN5qrYMP2S9iKdnk
VANIFj7656ARKt/nf4CBzxcpHTyB8+d2CtPDKCmlJP6vL8t58Jmih+kHJMvC0dzn
gr5f5+sCAOOe5gt9e0am7AvQWhdbHVfJU0TQJx+m2OiCJAqGTB1nvtBLHdJnfdC9
TnXXQ6ZXibqLyBies/xeY2sCKL5qtTMCAKnX9+9d/5yQxRyrQUHt1NYhaXZnJbHx
q4ytu0eWz+5i68IYUSK69jJ1NWPM0T6SkqpB3KCAIv68VFm9PxqG1KmhSrQIVGVz
dCBLZXmIuAQTAQIAIgUCTasU2gIbAwYLCQgHAwIGFQgCCQoLBBYCAwECHgECF4AA
CgkQO9o98PRieSoLhgQAkLEZex02Qt7vGhZzMwuN0R22w3VwyYyjBx+fM3JFETy1
ut4xcLJoJfIaF5ZS38UplgakHG0FQ+b49i8dMij0aZmDqGxrew1m4kBfjXw9B/v+
eIqpODryb6cOSwyQFH0lQkXC040pjq9YqDsO5w0WYNXYKDnzRV0p4H1pweo2VDid
AdgETasU2gEEAN46UPeWRqKHvA99arOxee38fBt2CI08iiWyI8T3J6ivtFGixSqV
bRcPxYO/qLpVe5l84Nb3X71GfVXlc9hyv7CD6tcowL59hg1E/DC5ydI8K8iEpUmK
/UnHdIY5h8/kqgGxkY/T/hgp5fRQgW1ZoZxLajVlMRZ8W4tFtT0DeA+JABEBAAEA
A/0bE1jaaZKj6ndqcw86jd+QtD1SF+Cf21CWRNeLKnUds4FRRvclzTyUMuWPkUeX
TaNNsUOFqBsf6QQ2oHUBBK4VCHffHCW4ZEX2cd6umz7mpHW6XzN4DECEzOVksXtc
lUC1j4UB91DC/RNQqwX1IV2QLSwssVotPMPqhOi0ZLNY7wIA3n7DWKInxYZZ4K+6
rQ+POsz6brEoRHwr8x6XlHenq1Oki855pSa1yXIARoTrSJkBtn5oI+f8AzrnN0BN
oyeQAwIA/7E++3HDi5aweWrViiul9cd3rcsS0dEnksPhvS0ozCJiHsq/6GFmy7J8
QSHZPteedBnZyNp5jR+H7cIfVN3KgwH/Skq4PsuPhDq5TKK6i8Pc1WW8MA6DXTdU
nLkX7RGmMwjC0DBf7KWAlPjFaONAX3a8ndnz//fy1q7u2l9AZwrj1qa1iJ8EGAEC
AAkFAk2rFNoCGwwACgkQO9o98PRieSo2/QP/WTzr4ioINVsvN1akKuekmEMI3LAp
BfHwatufxxP1U+3Si/6YIk7kuPB9Hs+pRqCXzbvPRrI8NHZBmc8qIGthishdCYad
AHcVnXjtxrULkQFGbGvhKURLvS9WnzD/m1K2zzwxzkPTzT9/Yf06O6Mal5AdugPL
VrM0m72/jnpKo04=
=zNCn
-----END PGP PRIVATE KEY BLOCK-----
`

var validClearsignInput = `
-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA1

Hello world
line 2
`

var invalidClearsignInput = `
-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA1

Invalid
`

var testSig = `-----BEGIN PGP SIGNATURE-----
Version: GnuPG v1.4.10 (GNU/Linux)

iJwEAQECAAYFAk8kMuEACgkQO9o98PRieSpMsAQAhmY/vwmNpflrPgmfWsYhk5O8
pjnBUzZwqTDoDeINjZEoPDSpQAHGhjFjgaDx/Gj4fAl0dM4D0wuUEBb6QOrwflog
2A2k9kfSOMOtk0IH/H5VuFN1Mie9L/erYXjTQIptv9t9J7NoRBMU0QOOaFU0JaO9
MyTpno24AjIAGb+mH1U=
=hIJ6
-----END PGP SIGNATURE-----
`

type signingSuite struct{}

func (s *signingSuite) TestDecodeCheckValidSignature(c *gc.C) {
	r := bytes.NewReader([]byte(validClearsignInput + testSig))
	txt, err := simplestreams.DecodeCheckSignature(r, testSigningKey)
	c.Assert(err, gc.IsNil)
	c.Assert(txt, gc.DeepEquals, []byte("Hello world\nline 2\n"))
}

func (s *signingSuite) TestDecodeCheckInvalidSignature(c *gc.C) {
	r := bytes.NewReader([]byte(invalidClearsignInput + testSig))
	_, err := simplestreams.DecodeCheckSignature(r, testSigningKey)
	c.Assert(err, gc.Not(gc.IsNil))
	_, ok := err.(*simplestreams.NotPGPSignedError)
	c.Assert(ok, jc.IsFalse)
}

func (s *signingSuite) TestDecodeCheckMissingSignature(c *gc.C) {
	r := bytes.NewReader([]byte("foo"))
	_, err := simplestreams.DecodeCheckSignature(r, testSigningKey)
	_, ok := err.(*simplestreams.NotPGPSignedError)
	c.Assert(ok, jc.IsTrue)
}
