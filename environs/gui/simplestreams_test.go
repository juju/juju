// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui_test

import (
	"net/http"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/gui"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/juju/keys"
	coretesting "github.com/juju/juju/testing"
)

type simplestreamsSuite struct {
	sstesting.LocalLiveSimplestreamsSuite
}

var _ = gc.Suite(&simplestreamsSuite{
	LocalLiveSimplestreamsSuite: sstesting.LocalLiveSimplestreamsSuite{
		Source:          sstesting.VerifyDefaultCloudDataSource("test", "test:"),
		RequireSigned:   false,
		DataType:        gui.DownloadType,
		StreamsVersion:  gui.StreamsVersion,
		ValidConstraint: gui.NewConstraint(gui.ReleasedStream, 2),
	},
})

func (s *simplestreamsSuite) SetUpSuite(c *gc.C) {
	s.LocalLiveSimplestreamsSuite.SetUpSuite(c)
	sstesting.TestRoundTripper.Sub = coretesting.NewCannedRoundTripper(
		guiData, map[string]int{"test://unauth": http.StatusUnauthorized})
}

func (s *simplestreamsSuite) TearDownSuite(c *gc.C) {
	sstesting.TestRoundTripper.Sub = nil
	s.LocalLiveSimplestreamsSuite.TearDownSuite(c)
}

func (s *simplestreamsSuite) TestNewDataSource(c *gc.C) {
	source := gui.NewDataSource("https://1.2.3.4/streams")
	c.Assert(source.Description(), gc.Equals, "dashboard simplestreams")

	url, err := source.URL("/my/path")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, "https://1.2.3.4/streams//my/path")

	c.Assert(source.RequireSigned(), jc.IsTrue)
	c.Assert(source.PublicSigningKey(), gc.Equals, keys.JujuPublicKey)
}

var fetchMetadataTests = []struct {
	// about describes the test.
	about string
	// stream holds the stream name to use for the test.
	stream string
	// jujuVersion holds the current Juju version to be used during the test.
	jujuVersion string
	// expectedMetadata holds the list of metadata information returned.
	// The following fields are automatically pre-populated by the test:
	// "FullPath", "Source", "DashboardVersion" and "MinJujuVersion"
	expectedMetadata []*gui.Metadata
	// expectedError optionally holds the expected error returned while trying
	// to retrieve Dashboard metadata information.
	expectedError string
}{{
	about:       "released version 2.8.2",
	stream:      gui.ReleasedStream,
	jujuVersion: "2.8.2",
	expectedMetadata: []*gui.Metadata{{
		Size:           6140774,
		SHA256:         "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
		Path:           "dashboard/2.1.1/jujudashboard-2.1.1.tar.bz2",
		MinJujuVersion: "2.8",
		Version:        version.MustParse("2.1.1"),
	}, {
		Version:        version.MustParse("2.1.0"),
		MinJujuVersion: "2.7",
		Path:           "dashboard/2.1.0/jujudashboard-2.1.0.tar.bz2",
		Size:           6098111,
		SHA256:         "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
	}},
}, {
	about:       "released version 2.0",
	stream:      gui.ReleasedStream,
	jujuVersion: "2.0.0",
}, {
	about:       "released version 47",
	stream:      gui.ReleasedStream,
	jujuVersion: "47.0.0",
}, {
	about:       "devel version 2.8.0",
	stream:      gui.DevelStream,
	jujuVersion: "2.8.0",
	expectedMetadata: []*gui.Metadata{{
		Version:        version.MustParse("2.4.0"),
		MinJujuVersion: "2.8",
		Path:           "dashboard/2.4.0/jujudashboard-2.4.0.tar.bz2",
		Size:           6098111,
		SHA256:         "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
	}, {
		Version:        version.MustParse("2.1.1"),
		MinJujuVersion: "2.7",
		Path:           "dashboard/2.1.1/jujudashboard-2.1.1.tar.bz2",
		Size:           474747,
		SHA256:         "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
	}},
}, {
	about:       "devel version 2.9.0",
	stream:      gui.DevelStream,
	jujuVersion: "2.9.0",
	expectedMetadata: []*gui.Metadata{{
		Version:        version.MustParse("2.5.0"),
		MinJujuVersion: "2.9",
		Path:           "dashboard/2.5.0/jujudashboard-2.5.0.tar.bz2",
		Size:           6198111,
		SHA256:         "123458b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
	}, {
		Version:        version.MustParse("2.4.0"),
		MinJujuVersion: "2.8",
		Path:           "dashboard/2.4.0/jujudashboard-2.4.0.tar.bz2",
		Size:           6098111,
		SHA256:         "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
	}, {
		Version:        version.MustParse("2.1.1"),
		MinJujuVersion: "2.7",
		Path:           "dashboard/2.1.1/jujudashboard-2.1.1.tar.bz2",
		Size:           474747,
		SHA256:         "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
	}},
}, {
	about:       "devel version 42",
	stream:      gui.DevelStream,
	jujuVersion: "42.0.0",
}, {
	about:         "error: invalid stream",
	stream:        "invalid",
	jujuVersion:   "2.0.0",
	expectedError: `error fetching simplestreams metadata: cannot unmarshal JSON metadata at URL "test:/streams/v1/com.canonical.streams-invalid-dashboard.json": .*`,
}, {
	about:         "error: invalid metadata",
	stream:        "errors",
	jujuVersion:   "2.0.0",
	expectedError: `error fetching simplestreams metadata: cannot parse metadata version: invalid version "bad-wolf"`,
}, {
	about:         "error: stream not found",
	stream:        "no-stream",
	jujuVersion:   "2.0.0",
	expectedError: `error fetching simplestreams metadata: "content-download" data not found`,
}, {
	about:         "error: stream file not found",
	stream:        "no-such",
	jujuVersion:   "2.0.0",
	expectedError: `error fetching simplestreams metadata: cannot read product data: "test:/streams/v1/com.canonical.streams-no-such-dashboard.json" not found`,
}}

func (s *simplestreamsSuite) TestFetchMetadata(c *gc.C) {
	for i, test := range fetchMetadataTests {
		c.Logf("\ntest %d: %s", i, test.about)
		jujuVersion := version.MustParse(test.jujuVersion)

		// Add invalid datasource and check later that resolveInfo is correct.
		invalidSource := sstesting.InvalidDataSource(s.RequireSigned)

		// Fetch the Juju Dashboard archives.
		allMeta, err := gui.FetchMetadata(test.stream, jujuVersion.Major, jujuVersion.Minor, invalidSource, s.Source)
		for i, meta := range allMeta {
			c.Logf("metadata %d:\n%#v", i, meta)
		}
		if test.expectedError != "" {
			c.Check(err, gc.ErrorMatches, test.expectedError)
			c.Check(allMeta, gc.IsNil)
			continue
		}

		// Populate the expected metadata with missing fields.
		for _, meta := range test.expectedMetadata {
			meta.FullPath = "test:/" + meta.Path
			meta.Source = s.Source
			meta.DashboardVersion = meta.Version.String()
		}
		c.Check(err, jc.ErrorIsNil)
		c.Check(allMeta, jc.DeepEquals, test.expectedMetadata)
	}
}

func (s *simplestreamsSuite) TestConstraint(c *gc.C) {
	constraint := gui.NewConstraint("test-stream", 42)
	c.Assert(constraint.IndexIds(), jc.DeepEquals, []string{"com.canonical.streams:test-stream:dashboard"})

	ids, err := constraint.ProductIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, jc.DeepEquals, []string{"com.canonical.streams:dashboard"})

	c.Assert(constraint.Arches, jc.DeepEquals, []string{})
	c.Assert(constraint.Series, jc.DeepEquals, []string{})

	c.Assert(constraint.Endpoint, gc.Equals, "")
	c.Assert(constraint.Region, gc.Equals, "")
	c.Assert(constraint.Stream, gc.Equals, "test-stream")
}

var guiData = map[string]string{
	"/streams/v1/index.json": `{
        "format": "index:1.0",
        "index": {
            "com.canonical.streams:devel:dashboard": {
                "datatype": "content-download",
                "format": "products:1.0",
                "path": "streams/v1/com.canonical.streams-devel-dashboard.json",
                "products": [
                    "com.canonical.streams:dashboard"
                ],
                "updated": "Tue, 05 Apr 2016 16:19:03 +0000"
            },
            "com.canonical.streams:released:dashboard": {
                "datatype": "content-download",
                "format": "products:1.0",
                "path": "streams/v1/com.canonical.streams-released-dashboard.json",
                "products": [
                    "com.canonical.streams:dashboard"
                ],
                "updated": "Fri, 01 Apr 2016 15:47:41 +0000"
            },
            "com.canonical.streams:invalid:dashboard": {
                "datatype": "content-download",
                "format": "products:1.0",
                "path": "streams/v1/com.canonical.streams-invalid-dashboard.json",
                "products": [
                    "com.canonical.streams:dashboard"
                ],
                "updated": "Fri, 01 Apr 2016 15:47:41 +0000"
            },
            "com.canonical.streams:errors:dashboard": {
                "datatype": "content-download",
                "format": "products:1.0",
                "path": "streams/v1/com.canonical.streams-errors-dashboard.json",
                "products": [
                    "com.canonical.streams:dashboard"
                ],
                "updated": "Fri, 01 Apr 2016 15:47:41 +0000"
            },
            "com.canonical.streams:no-such:dashboard": {
                "datatype": "content-download",
                "format": "products:1.0",
                "path": "streams/v1/com.canonical.streams-no-such-dashboard.json",
                "products": [
                    "com.canonical.streams:dashboard"
                ],
                "updated": "Fri, 01 Apr 2016 15:47:41 +0000"
            }
        },
    "updated": "Fri, 01 Apr 2016 15:47:41 +0000"
    }`,
	"/streams/v1/com.canonical.streams-devel-dashboard.json": `{
        "content_id": "com.canonical.streams:devel:dashboard",
        "datatype": "content-download",
        "format": "products:1.0",
        "products": {
            "com.canonical.streams:dashboard": {
                "format": "products:1.0",
                "ftype": "tar.bz2",
                "versions": {
                    "20160404": {
                        "items": {
                            "2.5.0": {
                                "juju-version": 2,
                                "min-juju-version": "2.9",
                                "md5": "5af3cb9f2625afbaff904cbd5c65772f",
                                "path": "dashboard/2.5.0/jujudashboard-2.5.0.tar.bz2",
                                "sha1": "1234a4236a132c75241a75d6ab1c96788c6f38b0",
                                "sha256": "123458b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
                                "size": 6198111,
                                "version": "2.5.0"
                            },
                            "2.4.0": {
                                "juju-version": 2,
                                "min-juju-version": "2.8",
                                "md5": "5af3cb9f2625afbaff904cbd5c65772f",
                                "path": "dashboard/2.4.0/jujudashboard-2.4.0.tar.bz2",
                                "sha1": "b364a4236a132c75241a75d6ab1c96788c6f38b0",
                                "sha256": "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
                                "size": 6098111,
                                "version": "2.4.0"
                            },
                            "2.1.1": {
                                "juju-version": 2,
                                "min-juju-version": "2.7",
                                "md5": "c49f1707078347cab31b0ff98bfb8dca",
                                "path": "dashboard/2.1.1/jujudashboard-2.1.1.tar.bz2",
                                "sha1": "1300d555f79b3de3bf334702d027701f69563849",
                                "sha256": "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
                                "size": 474747,
                                "version": "2.1.1"
                            }
                        }
                    }
                }
            }
        },
        "updated": "Mon, 04 Apr 2016 17:14:58 +0000"
    }`,
	"/streams/v1/com.canonical.streams-released-dashboard.json": `{
        "content_id": "com.canonical.streams:released:dashboard",
        "datatype": "content-download",
        "format": "products:1.0",
        "products": {
            "com.canonical.streams:dashboard": {
                "format": "products:1.0",
                "ftype": "tar.bz2",
                "versions": {
                    "20160404": {
                        "items": {
                            "2.1.0": {
                                "juju-version": 2,
                                "min-juju-version": "2.7",
                                "md5": "5af3cb9f2625afbaff904cbd5c65772f",
                                "path": "dashboard/2.1.0/jujudashboard-2.1.0.tar.bz2",
                                "sha1": "b364a4236a132c75241a75d6ab1c96788c6f38b0",
                                "sha256": "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
                                "size": 6098111,
                                "version": "2.1.0"
                            },
                            "2.1.1": {
                                "juju-version": 2,
                                "min-juju-version": "2.8",
                                "md5": "c49f1707078347cab31b0ff98bfb8dca",
                                "path": "dashboard/2.1.1/jujudashboard-2.1.1.tar.bz2",
                                "sha1": "1300d555f79b3de3bf334702d027701f69563849",
                                "sha256": "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
                                "size": 6140774,
                                "version": "2.1.1"
                            },
                            "3.0.0": {
                                "juju-version": 3,
                                "min-juju-version": "3.0",
                                "md5": "c49f1707078347cab31b0ff98bfb8dca",
                                "path": "dashboard/3.0.0/jujudashboard-3.0.0.tar.bz2",
                                "sha1": "1300d555f79b3de3bf334702d027701f69563849",
                                "sha256": "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
                                "size": 42424242,
                                "version": "3.0.0"
                            }
                        }
                    }
                }
            }
        },
        "updated": "Mon, 04 Apr 2016 17:14:58 +0000"
    }`,
	"/streams/v1/com.canonical.streams-invalid-dashboard.json": `
        bad: wolf
    `,
	"/streams/v1/com.canonical.streams-errors-dashboard.json": `{
        "content_id": "com.canonical.streams:devel:dashboard",
        "datatype": "content-download",
        "format": "products:1.0",
        "products": {
            "com.canonical.streams:dashboard": {
                "format": "products:1.0",
                "ftype": "tar.bz2",
                "versions": {
                    "20160404": {
                        "items": {
                            "2.0.0": {
                                "juju-version": 2,
                                "min-juju-version": "2.0",
                                "md5": "5af3cb9f2625afbaff904cbd5c65772f",
                                "path": "dashboard/2.0.0/jujudashboard-2.0.0.tar.bz2",
                                "sha1": "b364a4236a132c75241a75d6ab1c96788c6f38b0",
                                "sha256": "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
                                "size": 6098111,
                                "version": "bad-wolf"
                            }
                        }
                    }
                }
            }
        },
        "updated": "Mon, 04 Apr 2016 17:14:58 +0000"
    }`,
}
