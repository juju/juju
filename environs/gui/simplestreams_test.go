// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui_test

import (
	"net/http"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/gui"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/juju"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type simplestreamsSuite struct {
	sstesting.LocalLiveSimplestreamsSuite
}

var _ = gc.Suite(&simplestreamsSuite{
	LocalLiveSimplestreamsSuite: sstesting.LocalLiveSimplestreamsSuite{
		Source:          simplestreams.NewURLDataSource("test", "test:", utils.VerifySSLHostnames, simplestreams.DEFAULT_CLOUD_DATA, false),
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
	c.Assert(source.Description(), gc.Equals, "gui simplestreams")

	url, err := source.URL("/my/path")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, "https://1.2.3.4/streams//my/path")

	c.Assert(source.RequireSigned(), jc.IsTrue)
	c.Assert(source.PublicSigningKey(), gc.Equals, juju.JujuPublicKey)
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
	// "FullPath", "Source", "StringVersion" and "JujuMajorVersion"
	expectedMetadata []*gui.Metadata
	// expectedError optionally holds the expected error returned while trying
	// to retrieve GUI metadata information.
	expectedError string
}{{
	about:       "released version 2",
	stream:      gui.ReleasedStream,
	jujuVersion: "2.0.0",
	expectedMetadata: []*gui.Metadata{{
		Version: version.MustParse("2.1.1"),
		Path:    "gui/2.1.1/jujugui-2.1.1.tar.bz2",
		Size:    6140774,
		SHA256:  "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
	}, {
		Version: version.MustParse("2.1.0"),
		Path:    "gui/2.1.0/jujugui-2.1.0.tar.bz2",
		Size:    6098111,
		SHA256:  "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
	}},
}, {
	about:       "released version 2 beta",
	stream:      gui.ReleasedStream,
	jujuVersion: "2.0-beta1",
	expectedMetadata: []*gui.Metadata{{
		Version: version.MustParse("2.1.1"),
		Path:    "gui/2.1.1/jujugui-2.1.1.tar.bz2",
		Size:    6140774,
		SHA256:  "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
	}, {
		Version: version.MustParse("2.1.0"),
		Path:    "gui/2.1.0/jujugui-2.1.0.tar.bz2",
		Size:    6098111,
		SHA256:  "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
	}},
}, {
	about:       "released version 2.42",
	stream:      gui.ReleasedStream,
	jujuVersion: "2.42.0",
	expectedMetadata: []*gui.Metadata{{
		Version: version.MustParse("2.1.1"),
		Path:    "gui/2.1.1/jujugui-2.1.1.tar.bz2",
		Size:    6140774,
		SHA256:  "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
	}, {
		Version: version.MustParse("2.1.0"),
		Path:    "gui/2.1.0/jujugui-2.1.0.tar.bz2",
		Size:    6098111,
		SHA256:  "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
	}},
}, {
	about:       "released version 3",
	stream:      gui.ReleasedStream,
	jujuVersion: "3.0.0",
	expectedMetadata: []*gui.Metadata{{
		Version: version.MustParse("3.0.0"),
		Path:    "gui/3.0.0/jujugui-3.0.0.tar.bz2",
		Size:    42424242,
		SHA256:  "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
	}},
}, {
	about:       "released version 47",
	stream:      gui.ReleasedStream,
	jujuVersion: "47.0.0",
}, {
	about:       "devel version 2",
	stream:      gui.DevelStream,
	jujuVersion: "2.0.0",
	expectedMetadata: []*gui.Metadata{{
		Version: version.MustParse("2.4.0"),
		Path:    "gui/2.4.0/jujugui-2.4.0.tar.bz2",
		Size:    6098111,
		SHA256:  "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
	}, {
		Version: version.MustParse("2.1.1"),
		Path:    "gui/2.1.1/jujugui-2.1.1.tar.bz2",
		Size:    474747,
		SHA256:  "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
	}},
}, {
	about:       "devel version 42",
	stream:      gui.DevelStream,
	jujuVersion: "42.0.0",
}, {
	about:         "error: invalid stream",
	stream:        "invalid",
	jujuVersion:   "2.0.0",
	expectedError: `error fetching simplestreams metadata: cannot unmarshal JSON metadata at URL "test:/streams/v1/com.canonical.streams-invalid-gui.json": .*`,
}, {
	about:         "error: stream not found",
	stream:        "no-stream",
	jujuVersion:   "2.0.0",
	expectedError: `error fetching simplestreams metadata: "content-download" data not found`,
}, {
	about:         "error: stream file not found",
	stream:        "no-such",
	jujuVersion:   "2.0.0",
	expectedError: `error fetching simplestreams metadata: cannot read product data, invalid URL "test:/streams/v1/com.canonical.streams-no-such-gui.json" not found`,
}}

func (s *simplestreamsSuite) TestFetchMetadata(c *gc.C) {
	for i, test := range fetchMetadataTests {
		c.Logf("\ntest %d: %s", i, test.about)

		// Patch the current Juju version.
		jujuVersion := version.MustParse(test.jujuVersion)
		s.PatchValue(&jujuversion.Current, jujuVersion)

		// Add invalid datasource and check later that resolveInfo is correct.
		invalidSource := simplestreams.NewURLDataSource(
			"invalid", "file://invalid", utils.VerifySSLHostnames, simplestreams.DEFAULT_CLOUD_DATA, s.RequireSigned)

		// Fetch the Juju GUI archives.
		allMeta, err := gui.FetchMetadata(test.stream, invalidSource, s.Source)
		for i, meta := range allMeta {
			c.Logf("metadata %d:\n%#v", i, meta)
		}
		if test.expectedError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectedError)
			c.Assert(allMeta, gc.IsNil)
			continue
		}

		// Populate the expected metadata with missing fields.
		for _, meta := range test.expectedMetadata {
			meta.JujuMajorVersion = jujuVersion.Major
			meta.FullPath = "test:/" + meta.Path
			meta.Source = s.Source
			meta.StringVersion = meta.Version.String()
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(allMeta, jc.DeepEquals, test.expectedMetadata)
	}
}

func (s *simplestreamsSuite) TestConstraint(c *gc.C) {
	constraint := gui.NewConstraint("test-stream", 42)
	c.Assert(constraint.IndexIds(), jc.DeepEquals, []string{"com.canonical.streams:test-stream:gui"})

	ids, err := constraint.ProductIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, jc.DeepEquals, []string{"com.canonical.streams:gui"})

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
            "com.canonical.streams:devel:gui": {
                "datatype": "content-download",
                "format": "products:1.0",
                "path": "streams/v1/com.canonical.streams-devel-gui.json",
                "products": [
                    "com.canonical.streams:gui"
                ],
                "updated": "Tue, 05 Apr 2016 16:19:03 +0000"
            },
            "com.canonical.streams:released:gui": {
                "datatype": "content-download",
                "format": "products:1.0",
                "path": "streams/v1/com.canonical.streams-released-gui.json",
                "products": [
                    "com.canonical.streams:gui"
                ],
                "updated": "Fri, 01 Apr 2016 15:47:41 +0000"
            },
            "com.canonical.streams:invalid:gui": {
                "datatype": "content-download",
                "format": "products:1.0",
                "path": "streams/v1/com.canonical.streams-invalid-gui.json",
                "products": [
                    "com.canonical.streams:gui"
                ],
                "updated": "Fri, 01 Apr 2016 15:47:41 +0000"
            },
            "com.canonical.streams:no-such:gui": {
                "datatype": "content-download",
                "format": "products:1.0",
                "path": "streams/v1/com.canonical.streams-no-such-gui.json",
                "products": [
                    "com.canonical.streams:gui"
                ],
                "updated": "Fri, 01 Apr 2016 15:47:41 +0000"
            }
        },
    "updated": "Fri, 01 Apr 2016 15:47:41 +0000"
    }`,
	"/streams/v1/com.canonical.streams-devel-gui.json": `{
        "content_id": "com.canonical.streams:devel:gui",
        "datatype": "content-download",
        "format": "products:1.0",
        "products": {
            "com.canonical.streams:gui": {
                "format": "products:1.0",
                "ftype": "tar.bz2",
                "versions": {
                    "20160404": {
                        "items": {
                            "2.4.0": {
                                "juju-version": 2,
                                "md5": "5af3cb9f2625afbaff904cbd5c65772f",
                                "path": "gui/2.4.0/jujugui-2.4.0.tar.bz2",
                                "sha1": "b364a4236a132c75241a75d6ab1c96788c6f38b0",
                                "sha256": "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
                                "size": 6098111,
                                "version": "2.4.0"
                            },
                            "2.1.1": {
                                "juju-version": 2,
                                "md5": "c49f1707078347cab31b0ff98bfb8dca",
                                "path": "gui/2.1.1/jujugui-2.1.1.tar.bz2",
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
	"/streams/v1/com.canonical.streams-released-gui.json": `{
        "content_id": "com.canonical.streams:released:gui",
        "datatype": "content-download",
        "format": "products:1.0",
        "products": {
            "com.canonical.streams:gui": {
                "format": "products:1.0",
                "ftype": "tar.bz2",
                "versions": {
                    "20160404": {
                        "items": {
                            "2.1.0": {
                                "juju-version": 2,
                                "md5": "5af3cb9f2625afbaff904cbd5c65772f",
                                "path": "gui/2.1.0/jujugui-2.1.0.tar.bz2",
                                "sha1": "b364a4236a132c75241a75d6ab1c96788c6f38b0",
                                "sha256": "6cec58b36969590d3ff56279a2c63b4f5faf277b0dbeefe1106f666582575894",
                                "size": 6098111,
                                "version": "2.1.0"
                            },
                            "2.1.1": {
                                "juju-version": 2,
                                "md5": "c49f1707078347cab31b0ff98bfb8dca",
                                "path": "gui/2.1.1/jujugui-2.1.1.tar.bz2",
                                "sha1": "1300d555f79b3de3bf334702d027701f69563849",
                                "sha256": "5236f1b694a9a66dc4f86b740371408bf4ddf2354ebc6e5410587843a1e55743",
                                "size": 6140774,
                                "version": "2.1.1"
                            },
                            "3.0.0": {
                                "juju-version": 3,
                                "md5": "c49f1707078347cab31b0ff98bfb8dca",
                                "path": "gui/3.0.0/jujugui-3.0.0.tar.bz2",
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
	"/streams/v1/com.canonical.streams-invalid-gui.json": `
        bad: wolf
    `,
}
