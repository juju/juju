// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/testing/testbase"
)

var _ = gc.Suite(&marshalSuite{})

type marshalSuite struct {
	testbase.LoggingSuite
}

var expectedIndex = `{
    "index": {
        "com.ubuntu.cloud:custom": {
            "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
            "format": "products:1.0",
            "datatype": "image-ids",
            "cloudname": "custom",
            "clouds": [
                {
                    "region": "region",
                    "endpoint": "endpoint"
                }
            ],
            "path": "streams/v1/com.ubuntu.cloud:released:imagemetadata.json",
            "products": [
                "com.ubuntu.cloud:server:12.04:amd64",
                "com.ubuntu.cloud:server:12.04:arm",
                "com.ubuntu.cloud:server:13.10:arm"
            ]
        }
    },
    "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
    "format": "index:1.0"
}`

var expectedProducts = `{
    "products": {
        "com.ubuntu.cloud:server:12.04:amd64": {
            "version": "12.04",
            "arch": "amd64",
            "versions": {
                "19700101": {
                    "items": {
                        "abcd": {
                            "id": "abcd"
                        }
                    }
                }
            }
        },
        "com.ubuntu.cloud:server:12.04:arm": {
            "version": "12.04",
            "arch": "arm",
            "versions": {
                "19700101": {
                    "items": {
                        "5678": {
                            "id": "5678"
                        }
                    }
                }
            }
        },
        "com.ubuntu.cloud:server:13.10:arm": {
            "version": "13.10",
            "arch": "arm",
            "versions": {
                "19700101": {
                    "items": {
                        "1234": {
                            "id": "1234"
                        }
                    }
                }
            }
        }
    },
    "updated": "Thu, 01 Jan 1970 00:00:00 +0000",
    "format": "products:1.0",
    "content_id": "com.ubuntu.cloud:custom"
}`

var imageMetadataForTesting = []*imagemetadata.ImageMetadata{
	&imagemetadata.ImageMetadata{
		Id:      "1234",
		Version: "13.10",
		Arch:    "arm",
	},
	&imagemetadata.ImageMetadata{
		Id:      "5678",
		Version: "12.04",
		Arch:    "arm",
	},
	&imagemetadata.ImageMetadata{
		Id:      "abcd",
		Version: "12.04",
		Arch:    "amd64",
	},
}

func (s *marshalSuite) TestMarshalIndex(c *gc.C) {
	cloudSpec := []simplestreams.CloudSpec{{Region: "region", Endpoint: "endpoint"}}
	index, err := imagemetadata.MarshalImageMetadataIndexJSON(imageMetadataForTesting, cloudSpec, time.Unix(0, 0).UTC())
	c.Assert(err, gc.IsNil)
	c.Assert(string(index), gc.Equals, expectedIndex)
}

func (s *marshalSuite) TestMarshalProducts(c *gc.C) {
	products, err := imagemetadata.MarshalImageMetadataProductsJSON(imageMetadataForTesting, time.Unix(0, 0).UTC())
	c.Assert(err, gc.IsNil)
	c.Assert(string(products), gc.Equals, expectedProducts)
}

func (s *marshalSuite) TestMarshal(c *gc.C) {
	cloudSpec := []simplestreams.CloudSpec{{Region: "region", Endpoint: "endpoint"}}
	index, products, err := imagemetadata.MarshalImageMetadataJSON(imageMetadataForTesting, cloudSpec, time.Unix(0, 0).UTC())
	c.Assert(err, gc.IsNil)
	c.Assert(string(index), gc.Equals, expectedIndex)
	c.Assert(string(products), gc.Equals, expectedProducts)
}
