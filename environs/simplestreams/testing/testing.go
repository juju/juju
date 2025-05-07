// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

// Provides a TestDataSuite which creates and provides http access to sample simplestreams metadata.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/environs/simplestreams"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/testing"
)

var PrivateKeyPassphrase = "12345"

var SignedMetadataPrivateKey = `
-----BEGIN PGP PRIVATE KEY BLOCK-----
Version: GnuPG v1.4.12 (GNU/Linux)

lQH+BFJCk2EBBAC4wo3+aJ0PSeE54sv+GYNYckqysjazcZfJSdPK1GCN+Teat7ey
9dwlLhUIS34H29V+0/RcXmmRV+dObSkXzCx5ltKPSnuDsxvqiDEP0CgWdyFxhDf0
TbQuKK5OXcZ9rOTSFmnMxGaAzaV7T1IyuqA9HqntTIfC2tL4Y+QN41gS+QARAQAB
/gMDAjYGIOoxe8CYYGwpat1V7NGuphvvZRpqeP0RrJ6h4vHV3hXu5NK3tn6LZF0n
Qp31LfTH4BHF091UTiebexuuF1/ixLVihtv45mEVejFG1U3G298+WkWUP6AYA/5c
QRzXGiuTXlsBUuFVTGn1mvxRmG3yVoLkDj0l5rN9Tq3Ir4BACIWyxjBv1n8fqw+x
ti4b7YoE35FpIXQqLOdfdcKTOqUJt+5c+bed4Yx82BsLiY2/huqG2dLnbwln80Dz
iYudtG8xLJ1AeHBBFB0nVdyO+mPzXgLNEbP3zle2W+rUfz+s6te7y+rlV0gad2VG
tBAvUy08T9rDk0DNQl7jMq/3cGfDI1Zi/nzf2BuuBu2ddgIRmsXgKYly+Fq6eIpa
nM+P1hd1Fa3rQwUSJc/zrl48tukf8sdPLDk/+nMhLHy86jp+NeXyXPLvsMAlF5kR
eFjxEjHOnJlo4uIUxvlUuePyEOEl0XkQfZs+VWAPo+l2tB5UZXN0IFVzZXIgPHRl
c3RAc29tZXdoZXJlLmNvbT6IuAQTAQIAIgUCUkKTYQIbAwYLCQgHAwIGFQgCCQoL
BBYCAwECHgECF4AACgkQuK3uqWB66vCVugP/eJFir6Qdcvl+y9/HuP4q2iECi8ny
z9tC3YC9DcJePyoBnt1LJO3HvaquZh1AIr6hgMFaujjx6cCb7YEgE0pJ4m74dvtS
Y03MUPQ+Ok4cYV66zaDZLk6zpYJXZhxP7ZhlBvwQRES/rudBEQMfBcU9PrduFU39
iI+2ojHI4lsnMQE=
=UUIf
-----END PGP PRIVATE KEY BLOCK-----
`

var SignedMetadataPublicKey = `
-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: GnuPG v1.4.12 (GNU/Linux)

mI0EUkKTYQEEALjCjf5onQ9J4Tniy/4Zg1hySrKyNrNxl8lJ08rUYI35N5q3t7L1
3CUuFQhLfgfb1X7T9FxeaZFX505tKRfMLHmW0o9Ke4OzG+qIMQ/QKBZ3IXGEN/RN
tC4ork5dxn2s5NIWaczEZoDNpXtPUjK6oD0eqe1Mh8La0vhj5A3jWBL5ABEBAAG0
HlRlc3QgVXNlciA8dGVzdEBzb21ld2hlcmUuY29tPoi4BBMBAgAiBQJSQpNhAhsD
BgsJCAcDAgYVCAIJCgsEFgIDAQIeAQIXgAAKCRC4re6pYHrq8JW6A/94kWKvpB1y
+X7L38e4/iraIQKLyfLP20LdgL0Nwl4/KgGe3Usk7ce9qq5mHUAivqGAwVq6OPHp
wJvtgSATSknibvh2+1JjTcxQ9D46ThxhXrrNoNkuTrOlgldmHE/tmGUG/BBERL+u
50ERAx8FxT0+t24VTf2Ij7aiMcjiWycxAQ==
=zBYH
-----END PGP PUBLIC KEY BLOCK-----`

var imageData = map[string]string{
	"/daily/streams/v1/index.json": `
        {
         "index": {
          "com.ubuntu.cloud:released:raring": {
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
            "com.ubuntu.cloud:server:13.04:amd64"
           ],
           "path": "streams/v1/raring_metadata.json"
          }
         },
         "updated": "Wed, 01 May 2013 13:31:26 +0000",
         "format": "index:1.0"
        }
    `,
	"/streams/v1/index.json": `
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
		    "com.ubuntu.cloud:server:12.04:amd64",
 		    "com.ubuntu.cloud:server:12.10:amd64",
 		    "com.ubuntu.cloud:server:13.04:amd64",
 		    "com.ubuntu.cloud:server:14.04:amd64",
 		    "com.ubuntu.cloud:server:14.10:amd64",
 		    "com.ubuntu.cloud:server:12.04:arm"
		   ],
		   "path": "streams/v1/image_metadata.json"
		  },
		  "com.ubuntu.cloud:released:raring": {
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
		    "com.ubuntu.cloud:server:13.04:amd64"
		   ],
		   "path": "streams/v1/raring_metadata.json"
		  },
		  "com.ubuntu.cloud:released:download": {
		   "datatype": "content-download",
		   "path": "streams/v1/com.ubuntu.cloud:released:download.json",
		   "updated": "Wed, 01 May 2013 13:30:37 +0000",
		   "products": [
		    "com.ubuntu.cloud:server:12.10:amd64",
		    "com.ubuntu.cloud:server:13.04:amd64"
		   ],
		   "format": "products:1.0"
		  },
		  "com.ubuntu.juju:released:agents": {
		   "updated": "Mon, 05 Aug 2013 11:07:04 +0000",
		   "datatype": "content-download",
		   "format": "products:1.0",
		   "products": [
		     "com.ubuntu.juju:ubuntu:amd64",
		     "com.ubuntu.juju:ubuntu:arm"
		   ],
		   "path": "streams/v1/tools_metadata.json"
		  },
		  "com.ubuntu.juju:testing:agents": {
		   "updated": "Mon, 05 Aug 2013 11:07:04 +0000",
		   "datatype": "content-download",
		   "format": "products:1.0",
		   "products": [
		     "com.ubuntu.juju:ubuntu:amd64"
		   ],
		   "path": "streams/v1/testing_tools_metadata.json"
		  }
		 },
		 "updated": "Wed, 01 May 2013 13:31:26 +0000",
		 "format": "index:1.0"
		}
`,
	"/streams/v1/mirrors.json": `
        {
         "mirrors": {
          "com.ubuntu.juju:released:agents": [
             {
              "datatype": "content-download",
              "path": "streams/v1/tools_metadata:public-mirrors.json",
              "clouds": [
               {
                "region": "us-east-2",
                "endpoint": "https://ec2.us-east-2.amazonaws.com"
               },
               {
                "region": "us-west-2",
                "endpoint": "https://ec2.us-west-2.amazonaws.com"
               }
              ],
              "updated": "Wed, 14 Aug 2013 13:46:17 +0000",
              "format": "mirrors:1.0"
             },
             {
              "datatype": "content-download",
              "path": "streams/v1/tools_metadata:more-mirrors.json",
              "updated": "Wed, 14 Aug 2013 13:46:17 +0000",
              "format": "mirrors:1.0"
             }
          ]
         },
         "updated": "Wed, 01 May 2013 13:31:26 +0000",
         "format": "index:1.0"
        }
`,
	"/streams/v1/tools_metadata.json": `
{
 "content_id": "com.ubuntu.juju:agents",
 "datatype": "content-download",
 "updated": "Tue, 04 Jun 2013 13:50:31 +0000",
 "format": "products:1.0",
 "products": {
  "com.ubuntu.juju:ubuntu:amd64": {
   "arch": "amd64",
   "release": "ubuntu",
   "versions": {
    "20130806": {
     "items": {
      "1130ubuntuamd64": {
       "version": "1.13.0",
       "size": 2973595,
       "path": "tools/released/20130806/juju-1.13.0-ubuntu-amd64.tgz",
       "ftype": "tar.gz",
       "sha256": "447aeb6a934a5eaec4f703eda4ef2dde"
      }
     }
    }
   }
  },
  "com.ubuntu.juju:ubuntu:arm": {
   "arch": "arm",
   "release": "ubuntu",
   "versions": {
    "20130806": {
     "items": {
      "201ubuntuarm": {
       "version": "2.0.1",
       "size": 1951096,
       "path": "tools/released/20130806/juju-2.0.1-ubuntu-arm.tgz",
       "ftype": "tar.gz",
       "sha256": "f65a92b3b41311bdf398663ee1c5cd0c"
      },
      "1114ubuntuarm": {
       "version": "1.11.4",
       "size": 1951096,
       "path": "tools/released/20130806/juju-1.11.4-ubuntu-arm.tgz",
       "ftype": "tar.gz",
       "sha256": "f65a92b3b41311bdf398663ee1c5cd0c"
      }
     }
    },
    "20130803": {
     "items": {
      "1114ubuntuarm": {
       "version": "1.11.4",
       "size": 2851541,
       "path": "tools/released/20130803/juju-1.11.4-ubuntu-arm.tgz",
       "ftype": "tar.gz",
       "sha256": "df07ac5e1fb4232d4e9aa2effa57918a"
      },
      "1115ubuntuarm": {
       "version": "1.11.5",
       "size": 2031281,
       "path": "tools/released/20130803/juju-1.11.5-ubuntu-arm.tgz",
       "ftype": "tar.gz",
       "sha256": "df07ac5e1fb4232d4e9aa2effa57918a"
      }
     }
    }
   }
  }
 }
}
`,
	"/streams/v1/testing_tools_metadata.json": `
{
 "content_id": "com.ubuntu.juju:agents",
 "datatype": "content-download",
 "updated": "Tue, 04 Jun 2013 13:50:31 +0000",
 "format": "products:1.0",
 "products": {
  "com.ubuntu.juju:ubuntu:amd64": {
   "arch": "amd64",
   "release": "ubuntu",
   "versions": {
    "20130806": {
     "items": {
      "1130ubuntuamd64": {
       "version": "1.16.0",
       "size": 2973512,
       "path": "tools/testing/20130806/juju-1.16.0-ubuntu-amd64.tgz",
       "ftype": "tar.gz",
       "sha256": "447aeb6a934a5eaec4f703eda4ef2dac"
      }
     }
    }
   }
  }
 }
}
`,
	"/streams/v1/mirrored-tools-metadata.json": `
{
 "content_id": "com.ubuntu.juju:agents",
 "datatype": "content-download",
 "updated": "Tue, 04 Jun 2013 13:50:31 +0000",
 "format": "products:1.0",
 "products": {
  "com.ubuntu.juju:ubuntu:amd64": {
   "arch": "amd64",
   "release": "ubuntu",
   "versions": {
    "20130806": {
     "items": {
      "1130ubuntuamd64": {
       "version": "1.13.0",
       "size": 2973595,
       "path": "mirrored-path/juju-1.13.0-ubuntu-amd64.tgz",
       "ftype": "tar.gz",
       "sha256": "447aeb6a934a5eaec4f703eda4ef2dde"
      }
     }
    }
   }
  }
 }
}
`,
	"/streams/v1/tools_metadata:public-mirrors.json": `
{
  "mirrors": {
    "com.ubuntu.juju:released:agents": [
      {
        "mirror": "http://some-mirror/",
        "path": "com.ubuntu.juju:download.json",
        "format": "products:1.0",
        "clouds": [
          {
            "endpoint": "https://ec2.us-east-2.amazonaws.com",
            "region": "us-east-2"
          }
        ]
      },
      {
        "mirror": "test:/",
        "path": "streams/v1/mirrored-tools-metadata.json",
        "format": "products:1.0",
        "clouds": [
          {
            "endpoint": "https://ec2.us-west-2.amazonaws.com",
            "region": "us-west-2"
          }
        ]
      },
      {
        "mirror": "http://another-mirror/",
        "path": "com.ubuntu.juju:download.json",
        "format": "products:1.0",
        "clouds": [
          {
            "endpoint": "https://ec2.us-west-1.amazonaws.com",
            "region": "us-west-1"
          }
        ]
      }
    ]
  },
  "format": "mirrors:1.0",
  "updated": "Mon, 05 Aug 2013 11:07:04 +0000"
}
`,
	"/streams/v1/tools_metadata:more-mirrors.json": `
{
  "mirrors": {
    "com.ubuntu.juju:released:agents": [
      {
        "mirror": "http://big-mirror/",
        "path": "big:download.json",
        "clouds": [
          {
            "endpoint": "https://some-endpoint.com",
            "region": "some-region"
          }
        ]
      }
    ]
  },
  "format": "mirrors:1.0",
  "updated": "Mon, 05 Aug 2013 11:07:04 +0000"
}
`,
	"/streams/v1/image_metadata.json": `
{
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "content_id": "com.ubuntu.cloud:released:aws",
 "region": "nz-east-1",
 "endpoint": "https://anywhere",
 "root_store": "ebs",
 "virt": "pv", 
 "products": {
  "com.ubuntu.cloud:server:14.04:amd64": {
   "release": "trusty",
   "version": "14.04",
   "arch": "amd64",
   "versions": {
    "20140118": {
     "items": {
      "nzww1pe": {
       "root_store": "ssd",
       "virt": "hvm",
       "id": "ami-36745463"
      }
     },
     "pubname": "ubuntu-trusty-14.04-amd64-server-20140118",
     "label": "release"
    }
   }
  },
  "com.ubuntu.cloud:server:13.04:amd64": {
   "release": "raring",
   "version": "13.04",
   "arch": "amd64",
   "versions": {
    "20160318": {
     "items": {
      "nzww1pe": {
       "id": "ami-36745463"
      }
     },
     "pubname": "ubuntu-utopic-13.04-amd64-server-20160318",
     "label": "release"
    }
   }
  },
  "com.ubuntu.cloud:server:14.10:amd64": {
   "release": "utopic",
   "version": "14.10",
   "arch": "amd64",
   "root_store": "ebs",
   "virt": "pv",
   "versions": {
    "20160218": {
     "items": {
      "nzww1pe": {
       "id": "ami-36745463"
      }
     },
     "pubname": "ubuntu-utopic-14.10-amd64-server-20160218",
     "label": "release"
    }
   }
  },
  "com.ubuntu.cloud:server:12.10:amd64": {
   "release": "quantal",
   "version": "12.10",
   "arch": "amd64",
   "versions": {
    "20160118": {
     "items": {
      "nzww1pe": {
       "id": "ami-36745463"
      }
     },
     "root_store": "ebs",
     "virt": "pv",
     "pubname": "ubuntu-quantal-12.10-amd64-server-20160118",
     "label": "release"
    }
   }
  },
  "com.ubuntu.cloud:server:12.04:amd64": {
   "release": "precise",
   "version": "12.04",
   "arch": "amd64",
   "region": "au-east-1",
   "endpoint": "https://somewhere",
   "versions": {
    "20121218": {
     "region": "au-east-2",
     "endpoint": "https://somewhere-else",
     "items": {
      "usww1pe": {
       "root_store": "ebs",
       "virt": "pv",
       "id": "ami-26745463"
      },
      "usww2he": {
       "root_store": "ebs",
       "virt": "hvm",
       "id": "ami-442ea674",
       "region": "us-east-1",
       "endpoint": "https://ec2.us-east-1.amazonaws.com"
      },
      "usww3he": {
       "root_store": "ebs",
       "virt": "hvm",
       "crsn": "uswest3",
       "id": "ami-442ea675"
      }
     },
     "pubname": "ubuntu-precise-12.04-amd64-server-20121218",
     "label": "release"
    },
    "20111111": {
     "items": {
      "usww3pe": {
       "root_store": "ebs",
       "virt": "pv",
       "id": "ami-26745464"
      },
      "usww2pe": {
       "root_store": "instance",
       "virt": "pv",
       "id": "ami-442ea684",
       "region": "us-east-1",
       "endpoint": "https://ec2.us-east-1.amazonaws.com"
      }
     },
     "pubname": "ubuntu-precise-12.04-amd64-server-20111111",
     "label": "release"
    }
   }
  },
  "com.ubuntu.cloud:server:12.04:arm": {
   "release": "precise",
   "version": "12.04",
   "arch": "arm",
   "region": "us-east-1",
   "endpoint": "https://ec2.us-east-1.amazonaws.com",
   "versions": {
    "20121219": {
     "items": {
      "usww2he": {
       "root_store": "ebs",
       "virt": "pv",
       "id": "ami-442ea699"
      }
     },
     "pubname": "ubuntu-precise-12.04-arm-server-20121219",
     "label": "release"
    }
   }
  }
 },
 "_aliases": {
  "crsn": {
   "uswest3": {
    "region": "us-west-3",
    "endpoint": "https://ec2.us-west-3.amazonaws.com"
   }
  }
 },
 "format": "products:1.0"
}
`,
}

var TestRoundTripper = &testing.ProxyRoundTripper{}

type TestDataSuite struct{}

func (s *TestDataSuite) SetUpSuite(c *tc.C) {
	TestRoundTripper.Sub = testing.NewCannedRoundTripper(imageData, map[string]int{"test://unauth": http.StatusUnauthorized})
}

func (s *TestDataSuite) TearDownSuite(c *tc.C) {
	TestRoundTripper.Sub = nil
}

const (
	UnsignedJsonSuffix = ".json"
	SignedJsonSuffix   = ".sjson"
)

func SetRoundTripperFiles(files map[string]string, errorFiles map[string]int) {
	TestRoundTripper.Sub = testing.NewCannedRoundTripper(files, errorFiles)
}

func AddSignedFiles(c *tc.C, files map[string]string) map[string]string {
	all := make(map[string]string)
	for name, content := range files {
		all[name] = content
		// Sign file content
		r := strings.NewReader(content)
		bytes, err := io.ReadAll(r)
		c.Assert(err, jc.ErrorIsNil)
		signedName, signedContent, err := SignMetadata(name, bytes)
		c.Assert(err, jc.ErrorIsNil)
		all[signedName] = string(signedContent)
	}
	return all
}

func SignMetadata(fileName string, fileData []byte) (string, []byte, error) {
	signString := func(unsigned string) string {
		return strings.Replace(unsigned, UnsignedJsonSuffix, SignedJsonSuffix, -1)
	}

	// Make sure that contents point to signed files too.
	signedFileData := signString(string(fileData))
	signedBytes, err := simplestreams.Encode(strings.NewReader(signedFileData), SignedMetadataPrivateKey, PrivateKeyPassphrase)
	if err != nil {
		return "", nil, err
	}

	return signString(fileName), signedBytes, nil
}

// SourceDetails stored some details that need to be checked about data source.
type SourceDetails struct {
	URL           string
	Key           string
	RequireSigned bool
}

func AssertExpectedSources(c *tc.C, obtained []simplestreams.DataSource, dsDetails []SourceDetails) {
	// Some data sources do not require to contain signed data.
	// However, they may still contain it.
	// Since we will always try to read signed data first,
	// we want to be able to try to read this signed data
	// with a public key. Check keys are provided where needed.
	// Bugs #1542127, #1542131
	for i, source := range obtained {
		url, err := source.URL("")
		c.Assert(err, jc.ErrorIsNil)
		expected := dsDetails[i]
		c.Assert(url, tc.DeepEquals, expected.URL)
		c.Assert(source.PublicSigningKey(), tc.DeepEquals, expected.Key)
		c.Assert(source.RequireSigned(), tc.Equals, expected.RequireSigned)
	}
	c.Assert(obtained, tc.HasLen, len(dsDetails))
}

type LocalLiveSimplestreamsSuite struct {
	testing.BaseSuite
	Source          simplestreams.DataSource
	RequireSigned   bool
	StreamsVersion  string
	DataType        string
	ValidConstraint simplestreams.LookupConstraint
}

const (
	Index_v1   = "index:1.0"
	Product_v1 = "products:1.0"
)

type testConstraint struct {
	simplestreams.LookupParams
}

func NewTestConstraint(params simplestreams.LookupParams) *testConstraint {
	return &testConstraint{LookupParams: params}
}

func (tc *testConstraint) IndexIds() []string {
	return nil
}

func (tc *testConstraint) ProductIds() ([]string, error) {
	ids := make([]string, len(tc.Arches))
	for i, arch := range tc.Arches {
		ids[i] = fmt.Sprintf("com.ubuntu.cloud:server:%s:%s", tc.Releases[0], arch)
	}
	return ids, nil
}

type testDataSourceFactory struct{}

func TestDataSourceFactory() simplestreams.DataSourceFactory {
	return testDataSourceFactory{}
}

func (testDataSourceFactory) NewDataSource(cfg simplestreams.Config) simplestreams.DataSource {
	return simplestreams.NewDataSourceWithClient(cfg, jujuhttp.NewClient(
		jujuhttp.WithTransportMiddlewares(
			jujuhttp.DialContextMiddleware(jujuhttp.NewLocalDialBreaker(false)),
			jujuhttp.FileProtocolMiddleware,
			FileProtocolMiddleware,
		),
	))
}

type testSkipVerifyDataSourceFactory struct{}

func TestSkipVerifyDataSourceFactory() simplestreams.DataSourceFactory {
	return testSkipVerifyDataSourceFactory{}
}

func (testSkipVerifyDataSourceFactory) NewDataSource(cfg simplestreams.Config) simplestreams.DataSource {
	return simplestreams.NewDataSourceWithClient(cfg, jujuhttp.NewClient(
		jujuhttp.WithSkipHostnameVerification(true),
		jujuhttp.WithTransportMiddlewares(
			jujuhttp.DialContextMiddleware(jujuhttp.NewLocalDialBreaker(false)),
			jujuhttp.FileProtocolMiddleware,
			FileProtocolMiddleware,
		),
	))
}

// FileProtocolMiddleware registers support for file:// URLs on the given
// transport.
func FileProtocolMiddleware(transport *http.Transport) *http.Transport {
	TestRoundTripper.RegisterForTransportScheme(transport, "test")
	TestRoundTripper.RegisterForTransportScheme(transport, "signedtest")
	return transport
}

func init() {
	// Ensure out test struct can have its tags extracted.
	simplestreams.RegisterStructTags(TestItem{})
}

type TestItem struct {
	Id          string `json:"id"`
	Version     string `json:"version"`
	Storage     string `json:"root_store"`
	VirtType    string `json:"virt"`
	Arch        string `json:"arch"`
	RegionAlias string `json:"crsn"`
	RegionName  string `json:"region"`
	Endpoint    string `json:"endpoint"`
}

func (s *LocalLiveSimplestreamsSuite) IndexPath() string {
	if s.RequireSigned {
		return fmt.Sprintf("streams/%s/index.sjson", s.StreamsVersion)
	}
	return fmt.Sprintf("streams/%s/index.json", s.StreamsVersion)
}

func (s *LocalLiveSimplestreamsSuite) TestGetIndex(c *tc.C) {
	indexRef, err := s.GetIndexRef(Index_v1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(indexRef.Format, tc.Equals, Index_v1)
	c.Assert(indexRef.Source, tc.Equals, s.Source)
	c.Assert(len(indexRef.Indexes) > 0, jc.IsTrue)
}

func (s *LocalLiveSimplestreamsSuite) GetIndexRef(format string) (*simplestreams.IndexReference, error) {
	params := simplestreams.ValueParams{
		DataType:      s.DataType,
		ValueTemplate: TestItem{},
	}
	ss := simplestreams.NewSimpleStreams(TestDataSourceFactory())
	return ss.GetIndexWithFormat(
		context.Background(),
		s.Source, s.IndexPath(),
		format,
		simplestreams.MirrorsPath(s.StreamsVersion),
		s.RequireSigned,
		s.ValidConstraint.Params().CloudSpec,
		params,
	)
}

func (s *LocalLiveSimplestreamsSuite) TestGetIndexWrongFormat(c *tc.C) {
	_, err := s.GetIndexRef("bad")
	c.Assert(err, tc.NotNil)
}

func (s *LocalLiveSimplestreamsSuite) TestGetProductsPathExists(c *tc.C) {
	indexRef, err := s.GetIndexRef(Index_v1)
	c.Assert(err, jc.ErrorIsNil)
	path, err := indexRef.GetProductsPath(s.ValidConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, tc.Not(tc.Equals), "")
}

func (s *LocalLiveSimplestreamsSuite) TestGetProductsPathInvalidCloudSpec(c *tc.C) {
	indexRef, err := s.GetIndexRef(Index_v1)
	c.Assert(err, jc.ErrorIsNil)
	ic := NewTestConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{Region: "bad", Endpoint: "spec"},
		Releases:  []string{"12.04"},
	})
	_, err = indexRef.GetProductsPath(ic)
	c.Assert(err, tc.NotNil)
}

func (s *LocalLiveSimplestreamsSuite) TestGetProductsPathInvalidProductSpec(c *tc.C) {
	indexRef, err := s.GetIndexRef(Index_v1)
	c.Assert(err, jc.ErrorIsNil)
	ic := NewTestConstraint(simplestreams.LookupParams{
		CloudSpec: s.ValidConstraint.Params().CloudSpec,
		Releases:  []string{"12.04"},
		Arches:    []string{"bad"},
		Stream:    "spec",
	})
	_, err = indexRef.GetProductsPath(ic)
	c.Assert(err, tc.NotNil)
}

func (s *LocalLiveSimplestreamsSuite) AssertGetMetadata(c *tc.C) *simplestreams.CloudMetadata {
	indexRef, err := s.GetIndexRef(Index_v1)
	c.Assert(err, jc.ErrorIsNil)
	metadata, err := indexRef.GetCloudMetadataWithFormat(context.Background(), s.ValidConstraint, Product_v1, s.RequireSigned)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata.Format, tc.Equals, Product_v1)
	c.Assert(len(metadata.Products) > 0, jc.IsTrue)
	return metadata
}

func (s *LocalLiveSimplestreamsSuite) TestGetCloudMetadataWithFormat(c *tc.C) {
	s.AssertGetMetadata(c)
}

func (s *LocalLiveSimplestreamsSuite) AssertGetItemCollections(c *tc.C, version string) *simplestreams.ItemCollection {
	metadata := s.AssertGetMetadata(c)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	ic := metadataCatalog.Items[version]
	return ic
}

func InvalidDataSource(requireSigned bool) simplestreams.DataSource {
	factory := TestDataSourceFactory()
	return factory.NewDataSource(simplestreams.Config{
		Description:          "invalid",
		BaseURL:              "file://invalid",
		HostnameVerification: true,
		Priority:             simplestreams.DEFAULT_CLOUD_DATA,
		RequireSigned:        requireSigned})
}

func VerifyDefaultCloudDataSource(description, baseURL string) simplestreams.DataSource {
	factory := TestDataSourceFactory()
	return factory.NewDataSource(simplestreams.Config{
		Description:          description,
		BaseURL:              baseURL,
		HostnameVerification: true,
		Priority:             simplestreams.DEFAULT_CLOUD_DATA})
}
