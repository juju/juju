// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/testing"
)

var _ = tc.Suite(&marshalSuite{})

type marshalSuite struct {
	testing.BaseSuite
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
            "path": "streams/v1/com.ubuntu.cloud-released-imagemetadata.json",
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
                            "id": "abcd",
                            "root_store": "root",
                            "virt": "virt"
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
	{
		Id:      "1234",
		Version: "13.10",
		Arch:    "arm",
	},
	{
		Id:      "5678",
		Version: "12.04",
		Arch:    "arm",
	},
	{
		Id:       "abcd",
		Version:  "12.04",
		Arch:     "amd64",
		VirtType: "virt",
		Storage:  "root",
	},
}

func (s *marshalSuite) TestMarshalIndex(c *tc.C) {
	cloudSpec := []simplestreams.CloudSpec{{Region: "region", Endpoint: "endpoint"}}
	index, err := imagemetadata.MarshalImageMetadataIndexJSON(imageMetadataForTesting, cloudSpec, time.Unix(0, 0).UTC())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(index), tc.Equals, expectedIndex)
}

func (s *marshalSuite) TestMarshalProducts(c *tc.C) {
	products, err := imagemetadata.MarshalImageMetadataProductsJSON(imageMetadataForTesting, time.Unix(0, 0).UTC())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(products), tc.Equals, expectedProducts)
}

func (s *marshalSuite) TestMarshal(c *tc.C) {
	cloudSpec := []simplestreams.CloudSpec{{Region: "region", Endpoint: "endpoint"}}
	index, products, err := imagemetadata.MarshalImageMetadataJSON(imageMetadataForTesting, cloudSpec, time.Unix(0, 0).UTC())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(index), tc.Equals, expectedIndex)
	c.Assert(string(products), tc.Equals, expectedProducts)
}
