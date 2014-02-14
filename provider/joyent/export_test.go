// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"bytes"
	"strings"
	"text/template"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"

	"launchpad.net/gojoyent/jpc"
)

var Provider environs.EnvironProvider = GetProviderInstance()

// MetadataStorage returns a Storage instance which is used to store simplestreams metadata for tests.
func MetadataStorage(e environs.Environ) storage.Storage {
	container := "juju-test"
	metadataStorage := NewStorage(e.(*JoyentEnviron), container)

	// Ensure the container exists.
	err := metadataStorage.(*JoyentStorage).CreateContainer()
	if err != nil {
		panic(fmt.Errorf("cannot create %s container: %v", container, err))
	}
	return metadataStorage
}

// ImageMetadataStorage returns a Storage object pointing where the goose
// infrastructure sets up its keystone entry for image metadata
func ImageMetadataStorage(e environs.Environ) storage.Storage {
	env := e.(*JoyentEnviron)
	return NewStorage(env, "juju-test")
}

var indexData = `
		{
		 "index": {
		  "com.ubuntu.cloud:released:joyent": {
		   "updated": "Wed, 01 May 2013 13:31:26 +0000",
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
		   "path": "images/streams/v1/com.ubuntu.cloud:released:joyent.json"
		  }
		 },
		 "updated": "Wed, 01 May 2013 13:31:26 +0000",
		 "format": "index:1.0"
		}
`

var imagesData = `
{
 "content_id": "com.ubuntu.cloud:released:joyent",
 "products": {
   "com.ubuntu.cloud:server:12.04:amd64": {
     "release": "precise",
     "version": "12.04",
     "arch": "amd64",
     "versions": {
       "20121218": {
         "items": {
           "inst1": {
             "virt": "virtualmachine",
             "region": "some-region",
             "id": "11223344-0a0a-ff99-11bb-0a1b2c3d4e5f"
           },
           "inst2": {
             "virt": "virtualmachine",
             "region": "another-region",
             "id": "11223344-0a0a-ff99-11bb-0a1b2c3d4e5f"
           }
         },
         "pubname": "ubuntu-precise-12.04-amd64-server-20121218",
         "label": "release"
       },
       "20121111": {
         "items": {
           "inst3": {
             "virt": "virtualmachine",
             "region": "some-region",
             "id": "11223344-0a0a-ff99-11bb-0a1b2c3d4e5f"
           }
         },
         "pubname": "ubuntu-precise-12.04-amd64-server-20121111",
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
           "inst3": {
             "virt": "virtualmachine",
             "region": "region-1",
             "id": "11223344-0a0a-ee88-22ab-00aa11bb22cc"
           },
           "inst4": {
             "virt": "virtualmachine",
             "region": "region-2",
             "id": "11223344-0a0a-ee88-22ab-00aa11bb22cc"
           }
         },
         "pubname": "ubuntu-quantal-12.14-amd64-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:13.04:amd64": {
     "release": "raring",
     "version": "13.04",
     "arch": "amd64",
     "versions": {
       "20121218": {
         "items": {
           "inst5": {
             "virt": "virtualmachine",
             "region": "some-region",
             "id": "11223344-0a0a-dd77-33cd-abcd1234e5f6"
           },
           "inst6": {
             "virt": "virtualmachine",
             "region": "another-region",
             "id": "11223344-0a0a-dd77-33cd-abcd1234e5f6"
           }
         },
         "pubname": "ubuntu-raring-13.04-amd64-server-20121218",
         "label": "release"
       }
     }
   }
 },
 "format": "products:1.0"
}
`

const productMetadataFile = "images/streams/v1/com.ubuntu.cloud:released:joyent.json"

func UseTestImageData(stor storage.Storage, creds *jpc.Credentials) {
	// Put some image metadata files into the public storage.
	t := template.Must(template.New("").Parse(indexData))
	var metadata bytes.Buffer
	if err := t.Execute(&metadata, creds); err != nil {
		panic(fmt.Errorf("cannot generate index metdata: %v", err))
	}
	data := metadata.Bytes()
	stor.Put("images/"+simplestreams.DefaultIndexPath+".json", bytes.NewReader(data), int64(len(data)))
	stor.Put(
		productMetadataFile, strings.NewReader(imagesData), int64(len(imagesData)))
}

func RemoveTestImageData(stor storage.Storage) {
	stor.Remove("images/"+simplestreams.DefaultIndexPath + ".json")
	stor.Remove(productMetadataFile)
}

func FindInstanceSpec(e environs.Environ, series, arch, cons string) (spec *instances.InstanceSpec, err error) {
	env := e.(*JoyentEnviron)
	spec, err = env.FindInstanceSpec(&instances.InstanceConstraint{
		Series:      series,
		Arches:      []string{arch},
		Region:      env.Ecfg().Region(),
		Constraints: constraints.MustParse(cons),
	})
	return
}

func ControlBucketName(e environs.Environ) string {
	env := e.(*JoyentEnviron)
	return env.Ecfg().ControlDir()
}
