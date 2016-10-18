// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"bytes"
	"fmt"
	"text/template"
	"time"

	"github.com/joyent/gosign/auth"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
)

// Use ShortAttempt to poll for short-term events.
//
// TODO(katco): 2016-08-09: lp:1611427
var ShortAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

var Provider environs.EnvironProvider = GetProviderInstance()

var indexData = `
		{
		 "index": {
		  "com.ubuntu.cloud:released:joyent": {
		   "updated": "Fri, 14 Feb 2014 13:39:35 +0000",
		   "clouds": [
			{
			 "region": "{{.Region}}",
			 "endpoint": "{{.SdcEndpoint.URL}}"
			}
		   ],
		   "cloudname": "joyent",
		   "datatype": "image-ids",
		   "format": "products:1.0",
		   "products": [
			"com.ubuntu.cloud:server:16.04:amd64",
			"com.ubuntu.cloud:server:14.04:amd64",
			"com.ubuntu.cloud:server:12.10:amd64",
			"com.ubuntu.cloud:server:13.04:amd64"
		   ],
		   "path": "streams/v1/com.ubuntu.cloud:released:joyent.json"
		  }
		 },
		 "updated": "Fri, 14 Feb 2014 13:39:35 +0000",
		 "format": "index:1.0"
		}
`

var imagesData = `
{
  "content_id": "com.ubuntu.cloud:released:joyent",
  "format": "products:1.0",
  "updated": "Fri, 14 Feb 2014 13:39:35 +0000",
  "datatype": "image-ids",
  "products": {
    "com.ubuntu.cloud:server:16.04:amd64": {
      "release": "trusty",
      "version": "16.04",
      "arch": "amd64",
      "versions": {
        "20160216": {
          "items": {
            "11223344-0a0a-ff99-11bb-0a1b2c3d4e5f": {
              "region": "some-region",
              "id": "11223344-0a0a-ff99-11bb-0a1b2c3d4e5f",
              "virt": "kvm"
            }
          },
          "pubname": "ubuntu-trusty-16.04-amd64-server-20160216",
          "label": "release"
        }
      }
    },
	"com.ubuntu.cloud:server:14.04:amd64": {
      "release": "trusty",
      "version": "14.04",
      "arch": "amd64",
      "versions": {
        "20140214": {
          "items": {
            "11223344-0a0a-ff99-11bb-0a1b2c3d4e5f": {
              "region": "some-region",
              "id": "11223344-0a0a-ff99-11bb-0a1b2c3d4e5f",
              "virt": "kvm"
            }
          },
          "pubname": "ubuntu-trusty-14.04-amd64-server-20140214",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:12.10:amd64": {
      "release": "quantal",
      "version": "12.10",
      "arch": "amd64",
      "versions": {
        "20140214": {
          "items": {
            "11223344-0a0a-ee88-22ab-00aa11bb22cc": {
              "region": "some-region",
              "id": "11223344-0a0a-ee88-22ab-00aa11bb22cc",
              "virt": "kvm"
            }
          },
          "pubname": "ubuntu-quantal-12.10-amd64-server-20140214",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:13.04:amd64": {
      "release": "raring",
      "version": "13.04",
      "arch": "amd64",
      "versions": {
        "20140214": {
          "items": {
            "11223344-0a0a-dd77-33cd-abcd1234e5f6": {
              "region": "some-region",
              "id": "11223344-0a0a-dd77-33cd-abcd1234e5f6",
              "virt": "kvm"
            }
          },
          "pubname": "ubuntu-raring-13.04-amd64-server-20140214",
          "label": "release"
        }
      }
    }
  }
}
`

func parseIndexData(creds *auth.Credentials) bytes.Buffer {
	var metadata bytes.Buffer

	t := template.Must(template.New("").Parse(indexData))
	if err := t.Execute(&metadata, creds); err != nil {
		panic(fmt.Errorf("cannot generate index metdata: %v", err))
	}

	return metadata
}

// Set Metadata requests to be served by the filecontent supplied.
func UseExternalTestImageMetadata(c *gc.C, creds *auth.Credentials) {
	metadata := parseIndexData(creds)
	files := map[string]string{
		"/streams/v1/index.json":                            metadata.String(),
		"/streams/v1/com.ubuntu.cloud:released:joyent.json": imagesData,
	}
	sstesting.SetRoundTripperFiles(sstesting.AddSignedFiles(c, files), nil)
}

func UnregisterExternalTestImageMetadata() {
	sstesting.SetRoundTripperFiles(nil, nil)
}

// RegisterMachinesEndpoint creates a fake endpoint so that
// machines api calls succeed.
func RegisterMachinesEndpoint() {
	files := map[string]string{
		"/test/machines": "",
	}
	sstesting.SetRoundTripperFiles(files, nil)
}

// UnregisterMachinesEndpoint resets the machines endpoint.
func UnregisterMachinesEndpoint() {
	sstesting.SetRoundTripperFiles(nil, nil)
}

func FindInstanceSpec(
	e environs.Environ, series, arch, cons string,
	imageMetadata []*imagemetadata.ImageMetadata,
) (spec *instances.InstanceSpec, err error) {
	env := e.(*joyentEnviron)
	spec, err = env.FindInstanceSpec(&instances.InstanceConstraint{
		Series:      series,
		Arches:      []string{arch},
		Region:      env.cloud.Region,
		Constraints: constraints.MustParse(cons),
	}, imageMetadata)
	return
}

// MakeCredentials creates credentials for a test.
func MakeCredentials(c *gc.C, endpoint string, cloudCredential cloud.Credential) *auth.Credentials {
	creds, err := credentials(environs.CloudSpec{
		Endpoint:   endpoint,
		Credential: &cloudCredential,
	})
	c.Assert(err, jc.ErrorIsNil)
	return creds
}

var GetPorts = getPorts

var CreateFirewallRuleAll = createFirewallRuleAll

var CreateFirewallRuleVm = createFirewallRuleVm
