// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
)

// TODO: Apart from overriding different hardcoded hosts, these two test helpers are identical. Let's share.

// UseTestImageData causes the given content to be served
// when the ec2 client asks for image data.
func UseTestImageData(c *gc.C, files map[string]string) {
	if files != nil {
		sstesting.SetRoundTripperFiles(sstesting.AddSignedFiles(c, files), nil)
	} else {
		sstesting.SetRoundTripperFiles(nil, nil)
	}
}

// FabricateInstance creates a new fictitious instance
// given an existing instance and a new id.
func FabricateInstance(inst instances.Instance, newId string) instances.Instance {
	oldi := inst.(*sdkInstance)
	newi := &sdkInstance{
		e: oldi.e,
		i: types.Instance{},
	}
	newi.i = oldi.i
	newi.i.InstanceId = aws.String(newId)
	return newi
}

func makeImage(id, storage, virtType, arch, version, region string) *imagemetadata.ImageMetadata {
	return &imagemetadata.ImageMetadata{
		Id:         id,
		Storage:    storage,
		VirtType:   virtType,
		Arch:       arch,
		Version:    version,
		RegionName: region,
		Endpoint:   "https://ec2.endpoint.com",
		Stream:     "released",
	}
}

var TestImageMetadata = []*imagemetadata.ImageMetadata{
	// LTS-dependent requires new entries upon new LTS release.

	// 24.04:arm64
	makeImage("ami-02404133", "ssd", "hvm", "arm64", "24.04", "test"),

	// 24.04:amd64
	makeImage("ami-02404133", "ssd", "hvm", "amd64", "24.04", "test"),
	makeImage("ami-02404139", "ebs", "hvm", "amd64", "24.04", "test"),
	makeImage("ami-02404135", "ssd", "pv", "amd64", "24.04", "test"),

	// 22.04:arm64
	makeImage("ami-02204133", "ssd", "hvm", "arm64", "22.04", "test"),

	// 22.04:amd64
	makeImage("ami-02204133", "ssd", "hvm", "amd64", "22.04", "test"),
	makeImage("ami-02204139", "ebs", "hvm", "amd64", "22.04", "test"),
	makeImage("ami-02204135", "ssd", "pv", "amd64", "22.04", "test"),

	// 20.04:arm64
	makeImage("ami-02004133", "ssd", "hvm", "arm64", "20.04", "test"),

	// 20.04:amd64
	makeImage("ami-02004133", "ssd", "hvm", "amd64", "20.04", "test"),
	makeImage("ami-02004139", "ebs", "hvm", "amd64", "20.04", "test"),
	makeImage("ami-02004135", "ssd", "pv", "amd64", "20.04", "test"),

	// 18.04:arm64
	makeImage("ami-00002133", "ssd", "hvm", "arm64", "18.04", "test"),

	// 18.04:amd64
	makeImage("ami-00001133", "ssd", "hvm", "amd64", "18.04", "test"),
	makeImage("ami-00001139", "ebs", "hvm", "amd64", "18.04", "test"),
	makeImage("ami-00001135", "ssd", "pv", "amd64", "18.04", "test"),

	// 16.04:amd64
	makeImage("ami-00000133", "ssd", "hvm", "amd64", "16.04", "test"),
	makeImage("ami-00000139", "ebs", "hvm", "amd64", "16.04", "test"),
	makeImage("ami-00000135", "ssd", "pv", "amd64", "16.04", "test"),

	// 14.04:amd64
	makeImage("ami-00000033", "ssd", "hvm", "amd64", "14.04", "test"),

	// 12.10:amd64
	makeImage("ami-01000035", "ssd", "hvm", "amd64", "12.10", "test"),
}

func MakeTestImageStreamsData(region types.Region) map[string]string {
	testImageMetadataIndex := strings.Replace(testImageMetadataIndex, "$REGION", aws.ToString(region.RegionName), -1)
	testImageMetadataIndex = strings.Replace(testImageMetadataIndex, "$ENDPOINT", aws.ToString(region.Endpoint), -1)
	return map[string]string{
		"/streams/v1/index.json":                         testImageMetadataIndex,
		"/streams/v1/com.ubuntu.cloud:released:aws.json": testImageMetadataProduct,
	}
}

// LTS-dependent requires new/updated entries upon new LTS release.
const testImageMetadataIndex = `
{
 "index": {
  "com.ubuntu.cloud:released": {
   "updated": "Wed, 01 May 2013 13:31:26 +0000",
   "clouds": [
    {
     "region": "$REGION",
     "endpoint": "$ENDPOINT"
    }
   ],
   "cloudname": "aws",
   "datatype": "image-ids",
   "format": "products:1.0",
   "products": [
    "com.ubuntu.cloud:server:24.04:amd64",
    "com.ubuntu.cloud:server:22.04:amd64",
    "com.ubuntu.cloud:server:20.04:amd64",
    "com.ubuntu.cloud:server:18.04:amd64",
    "com.ubuntu.cloud:server:16.04:amd64",
    "com.ubuntu.cloud:server:14.04:amd64"
   ],
   "path": "streams/v1/com.ubuntu.cloud:released:aws.json"
  }
 },
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "format": "index:1.0"
}
`
const testImageMetadataProduct = `
{
 "content_id": "com.ubuntu.cloud:released:aws",
 "products": {
    "com.ubuntu.cloud:server:24.04:amd64": {
      "release": "noble",
      "version": "24.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "usee1pi": {
              "root_store": "instance",
              "virt": "pv",
              "region": "us-east-1",
              "id": "ami-02404111"
            },
            "usww1pe": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "eu-west-1",
              "id": "ami-02404116"
            },
            "apne1pe": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "ap-northeast-1",
              "id": "ami-02404126"
            },
            "apne1he": {
              "root_store": "ssd",
              "virt": "hvm",
              "region": "ap-northeast-1",
              "id": "ami-02404187"
            },
            "test1peebs": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "test",
              "id": "ami-02404133"
            },
            "test1pessd": {
              "root_store": "ebs",
              "virt": "pv",
              "region": "test",
              "id": "ami-02404139"
            },
            "test1he": {
              "root_store": "ssd",
              "virt": "hvm",
              "region": "test",
              "id": "ami-02404135"
            }
          },
          "pubname": "ubuntu-noble-24.04-amd64-server-20121218",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:22.04:amd64": {
      "release": "jammy",
      "version": "22.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "usee1pi": {
              "root_store": "instance",
              "virt": "pv",
              "region": "us-east-1",
              "id": "ami-02204111"
            },
            "usww1pe": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "eu-west-1",
              "id": "ami-02204116"
            },
            "apne1pe": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "ap-northeast-1",
              "id": "ami-02204126"
            },
            "apne1he": {
              "root_store": "ssd",
              "virt": "hvm",
              "region": "ap-northeast-1",
              "id": "ami-02204187"
            },
            "test1peebs": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "test",
              "id": "ami-02204133"
            },
            "test1pessd": {
              "root_store": "ebs",
              "virt": "pv",
              "region": "test",
              "id": "ami-02204139"
            },
            "test1he": {
              "root_store": "ssd",
              "virt": "hvm",
              "region": "test",
              "id": "ami-02204135"
            }
          },
          "pubname": "ubuntu-jammy-22.04-amd64-server-20121218",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:20.04:amd64": {
      "release": "focal",
      "version": "20.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "usee1pi": {
              "root_store": "instance",
              "virt": "pv",
              "region": "us-east-1",
              "id": "ami-02004111"
            },
            "usww1pe": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "eu-west-1",
              "id": "ami-02004116"
            },
            "apne1pe": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "ap-northeast-1",
              "id": "ami-02004126"
            },
            "apne1he": {
              "root_store": "ssd",
              "virt": "hvm",
              "region": "ap-northeast-1",
              "id": "ami-02004187"
            },
            "test1peebs": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "test",
              "id": "ami-02004133"
            },
            "test1pessd": {
              "root_store": "ebs",
              "virt": "pv",
              "region": "test",
              "id": "ami-02004139"
            },
            "test1he": {
              "root_store": "ssd",
              "virt": "hvm",
              "region": "test",
              "id": "ami-02004135"
            }
          },
          "pubname": "ubuntu-focal-20.04-amd64-server-20121218",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:18.04:amd64": {
      "release": "bionic",
      "version": "18.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "usee1pi": {
              "root_store": "instance",
              "virt": "pv",
              "region": "us-east-1",
              "id": "ami-00001111"
            },
            "usww1pe": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "eu-west-1",
              "id": "ami-00001116"
            },
            "apne1pe": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "ap-northeast-1",
              "id": "ami-00001126"
            },
            "apne1he": {
              "root_store": "ssd",
              "virt": "hvm",
              "region": "ap-northeast-1",
              "id": "ami-00001187"
            },
            "test1peebs": {
              "root_store": "ssd",
              "virt": "pv",
              "region": "test",
              "id": "ami-00001133"
            },
            "test1pessd": {
              "root_store": "ebs",
              "virt": "pv",
              "region": "test",
              "id": "ami-00001139"
            },
            "test1he": {
              "root_store": "ssd",
              "virt": "hvm",
              "region": "test",
              "id": "ami-00001135"
            }
          },
          "pubname": "ubuntu-bionic-18.04-amd64-server-20121218",
          "label": "release"
        }
      }
    },
   "com.ubuntu.cloud:server:16.04:amd64": {
     "release": "xenial",
     "version": "16.04",
     "arch": "amd64",
     "versions": {
       "20121218": {
         "items": {
           "usee1pi": {
             "root_store": "instance",
             "virt": "pv",
             "region": "us-east-1",
             "id": "ami-00000111"
           },
           "usww1pe": {
             "root_store": "ssd",
             "virt": "pv",
             "region": "eu-west-1",
             "id": "ami-00000116"
           },
           "apne1pe": {
             "root_store": "ssd",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-00000126"
           },
           "apne1he": {
             "root_store": "ssd",
             "virt": "hvm",
             "region": "ap-northeast-1",
             "id": "ami-00000187"
           },
           "test1peebs": {
             "root_store": "ssd",
             "virt": "pv",
             "region": "test",
             "id": "ami-00000133"
           },
           "test1pessd": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "test",
             "id": "ami-00000139"
           },
           "test1he": {
             "root_store": "ssd",
             "virt": "hvm",
             "region": "test",
             "id": "ami-00000135"
           }
         },
         "pubname": "ubuntu-xenial-16.04-amd64-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:14.04:amd64": {
     "release": "trusty",
     "version": "14.04",
     "arch": "amd64",
     "versions": {
       "20121218": {
         "items": {
           "test1peebs": {
             "root_store": "ssd",
             "virt": "hvm",
             "region": "test",
             "id": "ami-00000033"
           }
         },
         "pubname": "ubuntu-trusty-14.04-amd64-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:12.10:amd64": {
     "release": "quantal",
     "version": "12.10",
     "arch": "amd64",
     "versions": {
       "20121218": {
         "items": {
           "usee1pi": {
             "root_store": "instance",
             "virt": "pv",
             "region": "us-east-1",
             "id": "ami-00000011"
           },
           "usww1pe": {
             "root_store": "ssd",
             "virt": "pv",
             "region": "eu-west-1",
             "id": "ami-01000016"
           },
           "apne1pe": {
             "root_store": "ssd",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-01000026"
           },
           "apne1he": {
             "root_store": "ssd",
             "virt": "hvm",
             "region": "ap-northeast-1",
             "id": "ami-01000087"
           },
           "test1he": {
             "root_store": "ssd",
             "virt": "hvm",
             "region": "test",
             "id": "ami-01000035"
           }
         },
         "pubname": "ubuntu-quantal-12.10-amd64-server-20121218",
         "label": "release"
       }
     }
   }
 },
 "format": "products:1.0"
}
`
