// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	stdtesting "testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/juju/keys"
)

var live = flag.Bool("live", false, "Include live simplestreams tests")
var vendor = flag.String("vendor", "", "The vendor representing the source of the simplestream data")

type liveTestData struct {
	baseURL        string
	requireSigned  bool
	validCloudSpec simplestreams.CloudSpec
}

func getLiveURLs() (map[string]liveTestData, error) {
	resolver := ec2.NewDefaultEndpointResolver()
	ep, err := resolver.ResolveEndpoint("us-east-1", ec2.EndpointResolverOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return map[string]liveTestData{
		"ec2": {
			baseURL:       imagemetadata.DefaultUbuntuBaseURL,
			requireSigned: true,
			validCloudSpec: simplestreams.CloudSpec{
				Region:   "us-east-1",
				Endpoint: ep.URL,
			},
		},
		"canonistack": {
			baseURL:       "https://swift.canonistack.canonical.com/v1/AUTH_a48765cc0e864be980ee21ae26aaaed4/simplestreams/data",
			requireSigned: false,
			validCloudSpec: simplestreams.CloudSpec{
				Region:   "lcy01",
				Endpoint: "https://keystone.canonistack.canonical.com:443/v1.0/",
			},
		},
	}, nil
}

func Test(t *stdtesting.T) {
	if *live {
		if *vendor == "" {
			t.Fatal("missing vendor")
		}
		var ok bool
		var testData liveTestData
		liveURLs, err := getLiveURLs()
		if err != nil {
			t.Fatal(err.Error())
		}
		if testData, ok = liveURLs[*vendor]; !ok {
			keys := reflect.ValueOf(liveURLs).MapKeys()
			t.Fatalf("Unknown vendor %s. Must be one of %s", *vendor, keys)
		}
		cons, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
			CloudSpec: testData.validCloudSpec,
			Releases:  []string{"12.10"},
			Arches:    []string{"amd64"},
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		registerLiveSimpleStreamsTests(testData.baseURL, cons, testData.requireSigned)
	}
	registerSimpleStreamsTests(t)
	tc.TestingT(t)
}

func registerSimpleStreamsTests(t *stdtesting.T) {
	cons, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{
			Region:   "us-east-1",
			Endpoint: "https://ec2.us-east-1.amazonaws.com",
		},
		Releases: []string{"12.04"},
		Arches:   []string{"amd64", "arm"},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	tc.Suite(&simplestreamsSuite{
		LocalLiveSimplestreamsSuite: sstesting.LocalLiveSimplestreamsSuite{
			Source:          sstesting.VerifyDefaultCloudDataSource("test roundtripper", "test:"),
			RequireSigned:   false,
			DataType:        imagemetadata.ImageIds,
			StreamsVersion:  imagemetadata.CurrentStreamsVersion,
			ValidConstraint: cons,
		},
	})
	tc.Suite(&signedSuite{})
}

func registerLiveSimpleStreamsTests(baseURL string, validImageConstraint simplestreams.LookupConstraint, requireSigned bool) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	tc.Suite(&sstesting.LocalLiveSimplestreamsSuite{
		Source: ss.NewDataSource(simplestreams.Config{
			Description:          "test",
			BaseURL:              baseURL,
			HostnameVerification: true,
			Priority:             simplestreams.DEFAULT_CLOUD_DATA,
			RequireSigned:        requireSigned,
		}),
		RequireSigned:   requireSigned,
		DataType:        imagemetadata.ImageIds,
		ValidConstraint: validImageConstraint,
	})
}

type simplestreamsSuite struct {
	sstesting.LocalLiveSimplestreamsSuite
	sstesting.TestDataSuite
}

func (s *simplestreamsSuite) SetUpSuite(c *tc.C) {
	s.LocalLiveSimplestreamsSuite.SetUpSuite(c)
	s.TestDataSuite.SetUpSuite(c)
}

func (s *simplestreamsSuite) TearDownSuite(c *tc.C) {
	s.TestDataSuite.TearDownSuite(c)
	s.LocalLiveSimplestreamsSuite.TearDownSuite(c)
}

func (s *simplestreamsSuite) TestOfficialSources(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	origKey := imagemetadata.SetSigningPublicKey(sstesting.SignedMetadataPublicKey)
	defer func() {
		imagemetadata.SetSigningPublicKey(origKey)
	}()
	ds, err := imagemetadata.OfficialDataSources(ss, "daily")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ds, tc.HasLen, 1)
	url, err := ds[0].URL("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, tc.Equals, "http://cloud-images.ubuntu.com/daily/")
	c.Assert(ds[0].PublicSigningKey(), tc.Equals, sstesting.SignedMetadataPublicKey)
}

var fetchTests = []struct {
	region  string
	version string
	arches  []string
	images  []*imagemetadata.ImageMetadata
}{
	{
		region:  "us-east-1",
		version: "12.04",
		arches:  []string{"amd64", "arm"},
		images: []*imagemetadata.ImageMetadata{
			{
				Id:         "ami-442ea674",
				VirtType:   "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
			{
				Id:         "ami-442ea684",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "instance",
			},
			{
				Id:         "ami-442ea699",
				VirtType:   "pv",
				Arch:       "arm",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
		},
	},
	{
		region:  "us-east-1",
		version: "12.04",
		arches:  []string{"amd64"},
		images: []*imagemetadata.ImageMetadata{
			{
				Id:         "ami-442ea674",
				VirtType:   "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
			{
				Id:         "ami-442ea684",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "instance",
			},
		},
	},
	{
		region:  "us-east-1",
		version: "12.04",
		arches:  []string{"arm"},
		images: []*imagemetadata.ImageMetadata{
			{
				Id:         "ami-442ea699",
				VirtType:   "pv",
				Arch:       "arm",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
		},
	},
	{
		region:  "us-east-1",
		version: "12.04",
		arches:  []string{"amd64"},
		images: []*imagemetadata.ImageMetadata{
			{
				Id:         "ami-442ea674",
				VirtType:   "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
			{
				Id:         "ami-442ea684",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "instance",
			},
		},
	},
	{
		version: "12.04",
		arches:  []string{"amd64"},
		images: []*imagemetadata.ImageMetadata{
			{
				Id:         "ami-26745463",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "au-east-2",
				Endpoint:   "https://somewhere-else",
				Storage:    "ebs",
			},
			{
				Id:         "ami-26745464",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "au-east-1",
				Endpoint:   "https://somewhere",
				Storage:    "ebs",
			},
			{
				Id:         "ami-442ea674",
				VirtType:   "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
			{
				Id:          "ami-442ea675",
				VirtType:    "hvm",
				Arch:        "amd64",
				RegionAlias: "uswest3",
				RegionName:  "us-west-3",
				Endpoint:    "https://ec2.us-west-3.amazonaws.com",
				Storage:     "ebs",
			},
			{
				Id:         "ami-442ea684",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "instance",
			},
		},
	},
}

func (s *simplestreamsSuite) TestFetch(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	for i, t := range fetchTests {
		c.Logf("test %d", i)
		cloudSpec := simplestreams.CloudSpec{
			Region:   t.region,
			Endpoint: "https://ec2.us-east-1.amazonaws.com",
		}
		if t.region == "" {
			cloudSpec = simplestreams.EmptyCloudSpec
		}
		imageConstraint, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
			CloudSpec: cloudSpec,
			Releases:  []string{"12.04"},
			Arches:    t.arches,
		})
		c.Assert(err, jc.ErrorIsNil)
		// Add invalid datasource and check later that resolveInfo is correct.
		invalidSource := sstesting.InvalidDataSource(s.RequireSigned)
		images, resolveInfo, err := imagemetadata.Fetch(context.Background(), ss,
			[]simplestreams.DataSource{invalidSource, s.Source}, imageConstraint)
		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}
		for _, testImage := range t.images {
			testImage.Version = t.version
		}
		c.Check(images, tc.DeepEquals, t.images)
		c.Check(resolveInfo, tc.DeepEquals, &simplestreams.ResolveInfo{
			Source:    "test roundtripper",
			Signed:    s.RequireSigned,
			IndexURL:  "test:/streams/v1/index.json",
			MirrorURL: "",
		})
	}
}

type productSpecSuite struct{}

var _ = tc.Suite(&productSpecSuite{})

func (s *productSpecSuite) TestIdWithDefaultStream(c *tc.C) {
	imageConstraint, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		Releases: []string{"12.04"},
		Arches:   []string{"amd64"},
	})
	c.Assert(err, jc.ErrorIsNil)
	for _, stream := range []string{"", "released"} {
		imageConstraint.Stream = stream
		ids, err := imageConstraint.ProductIds()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ids, tc.DeepEquals, []string{"com.ubuntu.cloud:server:12.04:amd64"})
	}
}

func (s *productSpecSuite) TestId(c *tc.C) {
	imageConstraint, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		Releases: []string{"12.04"},
		Arches:   []string{"amd64"},
		Stream:   "daily",
	})
	c.Assert(err, jc.ErrorIsNil)
	ids, err := imageConstraint.ProductIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, tc.DeepEquals, []string{"com.ubuntu.cloud.daily:server:12.04:amd64"})
}

func (s *productSpecSuite) TestIdMultiArch(c *tc.C) {
	imageConstraint, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		Releases: []string{"12.04"},
		Arches:   []string{"amd64", "arm64"},
		Stream:   "daily",
	})
	c.Assert(err, jc.ErrorIsNil)
	ids, err := imageConstraint.ProductIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, tc.DeepEquals, []string{
		"com.ubuntu.cloud.daily:server:12.04:amd64",
		"com.ubuntu.cloud.daily:server:12.04:arm64"})
}

type signedSuite struct {
	origKey string
	server  *httptest.Server
}

func (s *signedSuite) SetUpSuite(_ *tc.C) {
	s.origKey = imagemetadata.SetSigningPublicKey(sstesting.SignedMetadataPublicKey)
	s.server = httptest.NewServer(&sstreamsHandler{})
}

func (s *signedSuite) TearDownSuite(_ *tc.C) {
	s.server.Close()
	imagemetadata.SetSigningPublicKey(s.origKey)
}

func (s *signedSuite) TestSignedImageMetadata(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	signedSource := simplestreams.NewDataSource(
		simplestreams.Config{
			Description:          "test",
			BaseURL:              fmt.Sprintf("%s/signed", s.server.URL),
			PublicSigningKey:     sstesting.SignedMetadataPublicKey,
			HostnameVerification: true,
			Priority:             simplestreams.DEFAULT_CLOUD_DATA,
			RequireSigned:        true,
		},
	)
	imageConstraint, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{
			Region:   "us-east-1",
			Endpoint: "https://ec2.us-east-1.amazonaws.com",
		},
		Releases: []string{"12.04"},
		Arches:   []string{"amd64"},
	})
	c.Assert(err, jc.ErrorIsNil)
	images, resolveInfo, err := imagemetadata.Fetch(context.Background(), ss, []simplestreams.DataSource{signedSource}, imageConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(images), tc.Equals, 1)
	c.Assert(images[0].Id, tc.Equals, "ami-123456")
	c.Check(resolveInfo, tc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    true,
		IndexURL:  fmt.Sprintf("%s/signed/streams/v1/index.sjson", s.server.URL),
		MirrorURL: "",
	})
}

func (s *signedSuite) TestSignedImageMetadataInvalidSignature(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	signedSource := simplestreams.NewDataSource(simplestreams.Config{
		Description:          "test",
		BaseURL:              fmt.Sprintf("%s/signed", s.server.URL),
		HostnameVerification: true,
		Priority:             simplestreams.DEFAULT_CLOUD_DATA,
		RequireSigned:        true,
	})
	imageConstraint, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{
			Region:   "us-east-1",
			Endpoint: "https://ec2.us-east-1.amazonaws.com",
		},
		Releases: []string{"12.04"},
		Arches:   []string{"amd64"},
	})
	c.Assert(err, jc.ErrorIsNil)
	imagemetadata.SetSigningPublicKey(s.origKey)
	_, _, err = imagemetadata.Fetch(context.Background(), ss, []simplestreams.DataSource{signedSource}, imageConstraint)
	c.Assert(err, tc.ErrorMatches, "cannot read index data.*")
}

type sstreamsHandler struct{}

func (h *sstreamsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/unsigned/streams/v1/index.json":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, unsignedIndex)
	case "/unsigned/streams/v1/image_metadata.json":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, unsignedProduct)
	case "/signed/streams/v1/image_metadata.sjson":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		rawUnsignedProduct := strings.Replace(
			unsignedProduct, "ami-26745463", "ami-123456", -1)
		_, _ = io.WriteString(w, encode(rawUnsignedProduct))
		return
	case "/signed/streams/v1/index.sjson":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		rawUnsignedIndex := strings.Replace(
			unsignedIndex, "streams/v1/image_metadata.json", "streams/v1/image_metadata.sjson", -1)
		_, _ = io.WriteString(w, encode(rawUnsignedIndex))
		return
	default:
		http.Error(w, r.URL.Path, 404)
		return
	}
}

func encode(data string) string {
	reader := bytes.NewReader([]byte(data))
	signedData, _ := simplestreams.Encode(
		reader, sstesting.SignedMetadataPrivateKey, sstesting.PrivateKeyPassphrase)
	return string(signedData)
}

var unsignedIndex = `
{
 "index": {
  "com.ubuntu.cloud:released:precise": {
   "updated": "Wed, 01 May 2013 13:31:26 +0000",
   "clouds": [
	{
	 "region": "us-east-1",
	 "endpoint": "https://ec2.us-east-1.amazonaws.com"
	}
   ],
   "cloudname": "aws",
   "datatype": "image-ids",
   "format": "products:1.0",
   "products": [
	"com.ubuntu.cloud:server:12.04:amd64"
   ],
   "path": "streams/v1/image_metadata.json"
  }
 },
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "format": "index:1.0"
}
`
var unsignedProduct = `
{
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "content_id": "com.ubuntu.cloud:released:aws",
 "products": {
  "com.ubuntu.cloud:server:12.04:amd64": {
   "release": "12.04",
   "version": "12.04",
   "arch": "amd64",
   "region": "us-east-1",
   "endpoint": "https://somewhere",
   "versions": {
    "20121218": {
     "region": "us-east-1",
     "endpoint": "https://somewhere-else",
     "items": {
      "usww1pe": {
       "root_store": "ebs",
       "virt": "pv",
       "id": "ami-26745463"
      }
     },
     "pubname": "ubuntu-precise-12.04-amd64-server-20121218",
     "label": "release"
    }
   }
  }
 },
 "format": "products:1.0"
}
`
