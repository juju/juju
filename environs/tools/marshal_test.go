// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/tools"
	jc "launchpad.net/juju-core/testing/checkers"
)

var _ = gc.Suite(&marshalSuite{})

type marshalSuite struct{}

func (s *marshalSuite) TestLargeNumber(c *gc.C) {
	metadata := []*tools.ToolsMetadata{
		&tools.ToolsMetadata{
			Release:  "saucy",
			Version:  "1.2.3.4",
			Arch:     "arm",
			Size:     9223372036854775807,
			Path:     "/somewhere/over/the/rainbow.tar.gz",
			FileType: "tar.gz",
		},
	}
	_, products, err := tools.MarshalToolsMetadataJSON(metadata, time.Now())
	c.Assert(err, gc.IsNil)
	c.Assert(string(products), jc.Contains, `"size": 9223372036854775807`)
}

var expectedIndex = `{
    "index": {
        "com.ubuntu.juju:released:tools": {
            "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
            "format": "products:1.0",
            "datatype": "content-download",
            "path": "streams/v1/com.ubuntu.juju:released:tools.json",
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

var expectedProducts = `{
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

var toolMetadataForTesting = []*tools.ToolsMetadata{
	&tools.ToolsMetadata{
		Release:  "saucy",
		Version:  "1.2.3.4",
		Arch:     "arm",
		Size:     9223372036854775807,
		Path:     "/somewhere/over/the/rainbow.tar.gz",
		FileType: "tar.gz",
	},
	&tools.ToolsMetadata{
		Release:  "precise",
		Version:  "1.2.3.4",
		Arch:     "arm",
		Size:     42,
		Path:     "toenlightenment.tar.gz",
		FileType: "tar.gz",
	},
	&tools.ToolsMetadata{
		Release:  "precise",
		Version:  "4.3.2.1",
		Arch:     "amd64",
		Path:     "whatever.tar.gz",
		FileType: "tar.gz",
		SHA256:   "afb14e65c794464e378def12cbad6a96f9186d69",
	},
}

func (s *marshalSuite) TestMarshalIndex(c *gc.C) {
	index, err := tools.MarshalToolsMetadataIndexJSON(toolMetadataForTesting, time.Unix(0, 0).UTC())
	c.Assert(err, gc.IsNil)
	c.Assert(string(index), gc.Equals, expectedIndex)
}

func (s *marshalSuite) TestMarshalProducts(c *gc.C) {
	products, err := tools.MarshalToolsMetadataProductsJSON(toolMetadataForTesting, time.Unix(0, 0).UTC())
	c.Assert(err, gc.IsNil)
	c.Assert(string(products), gc.Equals, expectedProducts)
}

func (s *marshalSuite) TestMarshal(c *gc.C) {
	index, products, err := tools.MarshalToolsMetadataJSON(toolMetadataForTesting, time.Unix(0, 0).UTC())
	c.Assert(err, gc.IsNil)
	c.Assert(string(index), gc.Equals, expectedIndex)
	c.Assert(string(products), gc.Equals, expectedProducts)
}
