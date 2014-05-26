// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"bytes"
	"fmt"
	"text/template"
	"time"

	"github.com/joyent/gosign/auth"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

// Use ShortAttempt to poll for short-term events.
var ShortAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

var Provider environs.EnvironProvider = GetProviderInstance()
var EnvironmentVariables = environmentVariables

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
			"com.ubuntu.cloud:server:12.04:amd64",
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
    "com.ubuntu.cloud:server:12.04:amd64": {
      "release": "precise",
      "version": "12.04",
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
          "pubname": "ubuntu-precise-12.04-amd64-server-20140214",
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

// This provides the content for code accessing test://host/... URLs. This allows
// us to set the responses for things like the Metadata server, by pointing
// metadata requests at test://host/...
var testRoundTripper = &jujutest.ProxyRoundTripper{}

func init() {
	testRoundTripper.RegisterForScheme("test")
}

var origImagesUrl = imagemetadata.DefaultBaseURL

// Set Metadata requests to be served by the filecontent supplied.
func UseExternalTestImageMetadata(creds *auth.Credentials) {
	metadata := parseIndexData(creds)
	files := map[string]string{
		"/streams/v1/index.json":                            metadata.String(),
		"/streams/v1/com.ubuntu.cloud:released:joyent.json": imagesData,
	}
	testRoundTripper.Sub = jujutest.NewCannedRoundTripper(files, nil)
	imagemetadata.DefaultBaseURL = "test://host"
}

func UnregisterExternalTestImageMetadata() {
	testRoundTripper.Sub = nil
	imagemetadata.DefaultBaseURL = origImagesUrl
}

func FindInstanceSpec(e environs.Environ, series, arch, cons string) (spec *instances.InstanceSpec, err error) {
	env := e.(*joyentEnviron)
	spec, err = env.FindInstanceSpec(&instances.InstanceConstraint{
		Series:      series,
		Arches:      []string{arch},
		Region:      env.Ecfg().Region(),
		Constraints: constraints.MustParse(cons),
	})
	return
}

func ControlBucketName(e environs.Environ) string {
	env := e.(*joyentEnviron)
	return env.Storage().(*JoyentStorage).GetContainerName()
}

func CreateContainer(s *JoyentStorage) error {
	return s.createContainer()
}

// MakeConfig creates a functional environConfig for a test.
func MakeConfig(c *gc.C, attrs testing.Attrs) *environConfig {
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, testing.Context(c), configstore.NewMem())
	c.Assert(err, gc.IsNil)
	return env.(*joyentEnviron).Ecfg()
}

// MakeCredentials creates credentials for a test.
func MakeCredentials(c *gc.C, attrs testing.Attrs) *auth.Credentials {
	creds, err := credentials(MakeConfig(c, attrs))
	c.Assert(err, gc.IsNil)
	return creds
}

// MakeStorage creates an env storage for a test.
func MakeStorage(c *gc.C, attrs testing.Attrs) storage.Storage {
	stor, err := newStorage(MakeConfig(c, attrs), "")
	c.Assert(err, gc.IsNil)
	return stor
}
