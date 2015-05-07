// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/gomaasapi"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/storage"
)

var (
	ShortAttempt            = &shortAttempt
	APIVersion              = apiVersion
	MaasStorageProviderType = maasStorageProviderType
)

func MAASAgentName(env environs.Environ) string {
	return env.(*maasEnviron).ecfg().maasAgentName()
}

func GetMAASClient(env environs.Environ) *gomaasapi.MAASObject {
	return env.(*maasEnviron).getMAASClient()
}

func NewCloudinitConfig(env environs.Environ, hostname, iface, series string) (cloudinit.CloudConfig, error) {
	return env.(*maasEnviron).newCloudinitConfig(hostname, iface, series)
}

var indexData = `
{
 "index": {
  "com.ubuntu.cloud:released:maas": {
   "updated": "Fri, 14 Feb 2014 13:39:35 +0000",
   "cloudname": "maas",
   "datatype": "image-ids",
   "format": "products:1.0",
   "products": [
     "com.ubuntu.cloud:server:12.04:amd64"
   ],
   "path": "streams/v1/com.ubuntu.cloud:released:maas.json"
  }
 },
 "updated": "Fri, 14 Feb 2014 13:39:35 +0000",
 "format": "index:1.0"
}
`

var imagesData = `
{
  "content_id": "com.ubuntu.cloud:released:maas",
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
    }
  }
}
`

func UseTestImageMetadata(c *gc.C, stor storage.Storage) {
	files := map[string]string{
		"images/streams/v1/index.json":                          indexData,
		"images/streams/v1/com.ubuntu.cloud:released:maas.json": imagesData,
	}
	for f, d := range files {
		rdr := strings.NewReader(d)
		err := stor.Put(f, rdr, int64(len(d)))
		c.Assert(err, jc.ErrorIsNil)
	}
}
