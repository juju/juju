// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"encoding/json"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/tools"
)

var _ = gc.Suite(&marshalSuite{})

type marshalSuite struct {
	streamMetadata map[string][]*tools.ToolsMetadata
}

func (s *marshalSuite) SetUpTest(c *gc.C) {
	s.streamMetadata = map[string][]*tools.ToolsMetadata{
		"released": releasedToolMetadataForTesting,
		"proposed": proposedToolMetadataForTesting,
	}
}

func (s *marshalSuite) TestLargeNumber(c *gc.C) {
	metadata := map[string][]*tools.ToolsMetadata{
		"released": {
			{
				Release:  "saucy",
				Version:  "1.2.3.4",
				Arch:     "arm",
				Size:     9223372036854775807,
				Path:     "/somewhere/over/the/rainbow.tar.gz",
				FileType: "tar.gz",
			}},
	}
	_, _, products, err := tools.MarshalToolsMetadataJSON(metadata, time.Now())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(products["released"]), jc.Contains, `"size": 9223372036854775807`)
}

var expectedIndex = `{
    "index": {
        "com.ubuntu.juju:proposed:tools": {
            "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
            "format": "products:1.0",
            "datatype": "content-download",
            "path": "streams/v1/com.ubuntu.juju-proposed-tools.json",
            "products": [
                "com.ubuntu.juju:14.04:arm64",
                "com.ubuntu.juju:14.10:ppc64el"            
            ]
        },
        "com.ubuntu.juju:released:tools": {
            "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
            "format": "products:1.0",
            "datatype": "content-download",
            "path": "streams/v1/com.ubuntu.juju-released-tools.json",
            "products": [
                "com.ubuntu.juju:12.04:amd64",
                "com.ubuntu.juju:12.04:arm",
                "com.ubuntu.juju:13.10:arm"            
            ]
        }
    },
    "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
    "format": "index:1.0"
}`

var expectedLegacyIndex = `{
    "index": {
        "com.ubuntu.juju:released:tools": {
            "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
            "format": "products:1.0",
            "datatype": "content-download",
            "path": "streams/v1/com.ubuntu.juju-released-tools.json",
            "products": [
                "com.ubuntu.juju:12.04:amd64",
                "com.ubuntu.juju:12.04:arm",
                "com.ubuntu.juju:13.10:arm"            
            ]
        }
    },
    "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
    "format": "index:1.0"
}`

var expectedReleasedProducts = `{
    "products": {
        "com.ubuntu.juju:12.04:amd64": {
            "version": "4.3.2.1",
            "arch": "amd64",
            "versions": {
                "19700101": {
                    "items": {
                        "4.3.2.1-precise-amd64": {
                            "release": "precise",
                            "version": "4.3.2.1",
                            "arch": "amd64",
                            "size": 0,
                            "path": "whatever.tar.gz",
                            "ftype": "tar.gz",
                            "sha256": "afb14e65c794464e378def12cbad6a96f9186d69"
                        }
                    }
                }
            }
        },
        "com.ubuntu.juju:12.04:arm": {
            "version": "1.2.3.4",
            "arch": "arm",
            "versions": {
                "19700101": {
                    "items": {
                        "1.2.3.4-precise-arm": {
                            "release": "precise",
                            "version": "1.2.3.4",
                            "arch": "arm",
                            "size": 42,
                            "path": "toenlightenment.tar.gz",
                            "ftype": "tar.gz",
                            "sha256": ""
                        }
                    }
                }
            }
        },
        "com.ubuntu.juju:13.10:arm": {
            "version": "1.2.3.4",
            "arch": "arm",
            "versions": {
                "19700101": {
                    "items": {
                        "1.2.3.4-saucy-arm": {
                            "release": "saucy",
                            "version": "1.2.3.4",
                            "arch": "arm",
                            "size": 9223372036854775807,
                            "path": "/somewhere/over/the/rainbow.tar.gz",
                            "ftype": "tar.gz",
                            "sha256": ""
                        }
                    }
                }
            }
        }
    },
    "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
    "format": "products:1.0",
    "content_id": "com.ubuntu.juju:released:tools"
}`

var expectedProposedProducts = `{
    "products": {
        "com.ubuntu.juju:14.04:arm64": {
            "version": "1.2-beta1",
            "arch": "arm64",
            "versions": {
                "19700101": {
                    "items": {
                        "1.2-beta1-trusty-arm64": {
                            "release": "trusty",
                            "version": "1.2-beta1",
                            "arch": "arm64",
                            "size": 42,
                            "path": "gotham.tar.gz",
                            "ftype": "tar.gz",
                            "sha256": ""
                        }
                    }
                }
            }
        },
        "com.ubuntu.juju:14.10:ppc64el": {
            "version": "1.2-alpha1",
            "arch": "ppc64el",
            "versions": {
                "19700101": {
                    "items": {
                        "1.2-alpha1-utopic-ppc64el": {
                            "release": "utopic",
                            "version": "1.2-alpha1",
                            "arch": "ppc64el",
                            "size": 9223372036854775807,
                            "path": "/funkytown.tar.gz",
                            "ftype": "tar.gz",
                            "sha256": ""
                        }
                    }
                }
            }
        }
    },
    "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
    "format": "products:1.0",
    "content_id": "com.ubuntu.juju:proposed:tools"
}`

var releasedToolMetadataForTesting = []*tools.ToolsMetadata{
	{
		Release:  "saucy",
		Version:  "1.2.3.4",
		Arch:     "arm",
		Size:     9223372036854775807,
		Path:     "/somewhere/over/the/rainbow.tar.gz",
		FileType: "tar.gz",
	},
	{
		Release:  "precise",
		Version:  "1.2.3.4",
		Arch:     "arm",
		Size:     42,
		Path:     "toenlightenment.tar.gz",
		FileType: "tar.gz",
	},
	{
		Release:  "precise",
		Version:  "4.3.2.1",
		Arch:     "amd64",
		Path:     "whatever.tar.gz",
		FileType: "tar.gz",
		SHA256:   "afb14e65c794464e378def12cbad6a96f9186d69",
	},
	{
		Release:  "xuanhuaceratops",
		Version:  "4.3.2.1",
		Arch:     "amd64",
		Size:     42,
		Path:     "dinodance.tar.gz",
		FileType: "tar.gz",
	},
	{
		Release:  "xuanhanosaurus",
		Version:  "5.4.3.2",
		Arch:     "amd64",
		Size:     42,
		Path:     "dinodisco.tar.gz",
		FileType: "tar.gz",
	},
}

var proposedToolMetadataForTesting = []*tools.ToolsMetadata{
	{
		Release:  "utopic",
		Version:  "1.2-alpha1",
		Arch:     "ppc64el",
		Size:     9223372036854775807,
		Path:     "/funkytown.tar.gz",
		FileType: "tar.gz",
	},
	{
		Release:  "trusty",
		Version:  "1.2-beta1",
		Arch:     "arm64",
		Size:     42,
		Path:     "gotham.tar.gz",
		FileType: "tar.gz",
	},
	{
		Release:  "xuanhuaceratops",
		Version:  "4.3.2.1",
		Arch:     "amd64",
		Size:     42,
		Path:     "dinodance.tar.gz",
		FileType: "tar.gz",
	},
	{
		Release:  "xuanhanosaurus",
		Version:  "5.4.3.2",
		Arch:     "amd64",
		Size:     42,
		Path:     "dinodisco.tar.gz",
		FileType: "tar.gz",
	},
}

func (s *marshalSuite) TestMarshalIndex(c *gc.C) {
	index, legacyIndex, err := tools.MarshalToolsMetadataIndexJSON(s.streamMetadata, time.Unix(0, 0).UTC())
	c.Assert(err, jc.ErrorIsNil)
	assertIndex(c, index, expectedIndex)
	assertIndex(c, legacyIndex, expectedLegacyIndex)
}

func assertIndex(c *gc.C, obtainedIndex []byte, expectedIndex string) {
	// Unmarshall into objects so an order independent comparison can be done.
	var obtained interface{}
	err := json.Unmarshal(obtainedIndex, &obtained)
	var expected interface{}
	err = json.Unmarshal([]byte(expectedIndex), &expected)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *marshalSuite) TestMarshalProducts(c *gc.C) {
	products, err := tools.MarshalToolsMetadataProductsJSON(s.streamMetadata, time.Unix(0, 0).UTC())
	c.Assert(err, jc.ErrorIsNil)
	assertProducts(c, products)
}

func assertProducts(c *gc.C, obtainedProducts map[string][]byte) {
	c.Assert(obtainedProducts, gc.HasLen, 2)
	c.Assert(string(obtainedProducts["released"]), gc.Equals, expectedReleasedProducts)
	c.Assert(string(obtainedProducts["proposed"]), gc.Equals, expectedProposedProducts)
}

func (s *marshalSuite) TestMarshal(c *gc.C) {
	index, legacyIndex, products, err := tools.MarshalToolsMetadataJSON(s.streamMetadata, time.Unix(0, 0).UTC())
	c.Assert(err, jc.ErrorIsNil)
	assertIndex(c, index, expectedIndex)
	assertIndex(c, legacyIndex, expectedLegacyIndex)
	assertProducts(c, products)
}

func (s *marshalSuite) TestMarshalNoReleaseStream(c *gc.C) {
	metadata := s.streamMetadata
	delete(metadata, "released")
	index, legacyIndex, products, err := tools.MarshalToolsMetadataJSON(s.streamMetadata, time.Unix(0, 0).UTC())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(legacyIndex, gc.IsNil)
	c.Assert(index, gc.NotNil)
	c.Assert(products, gc.NotNil)
}
