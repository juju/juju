// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/swift"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/instance"
)

// This provides the content for code accessing test:///... URLs. This allows
// us to set the responses for things like the Metadata server, by pointing
// metadata requests at test:///... rather than http://169.254.169.254
var testRoundTripper = &jujutest.ProxyRoundTripper{}

func init() {
	testRoundTripper.RegisterForScheme("test")
}

var origMetadataHost = metadataHost

var metadataContent = `"availability_zone": "nova", "hostname": "test.novalocal", ` +
	`"launch_index": 0, "meta": {"priority": "low", "role": "webserver"}, ` +
	`"public_keys": {"mykey": "ssh-rsa fake-key\n"}, "name": "test"}`

// A group of canned responses for the "metadata server". These match
// reasonably well with the results of making those requests on a Folsom+
// Openstack service
var MetadataTesting = map[string]string{
	"/latest/meta-data/local-ipv4":         "10.1.1.2",
	"/latest/meta-data/public-ipv4":        "203.1.1.2",
	"/openstack/2012-08-10/meta_data.json": metadataContent,
}

// Set Metadata requests to be served by the filecontent supplied.
func UseTestMetadata(metadata map[string]string) {
	if len(metadata) != 0 {
		testRoundTripper.Sub = jujutest.NewCannedRoundTripper(metadata, nil)
		metadataHost = "test:"
	} else {
		testRoundTripper.Sub = nil
		metadataHost = origMetadataHost
	}
}

var (
	ShortAttempt   = &shortAttempt
	StorageAttempt = &storageAttempt
)

func SetFakeToolsStorage(useFake bool) {
	if useFake {
		tools.SetToolPrefix("tools_test/juju-")
	} else {
		tools.SetToolPrefix(tools.DefaultToolPrefix)
	}
}

// WritablePublicStorage returns a Storage instance which is authorised to write to the PublicStorage bucket.
// It is used by tests which need to upload files.
func WritablePublicStorage(e environs.Environ) environs.Storage {
	ecfg := e.(*environ).ecfg()
	authModeCfg := AuthMode(ecfg.authMode())
	writablePublicStorage := &storage{
		containerName: ecfg.publicBucket(),
		swift:         swift.New(e.(*environ).authClient(ecfg, authModeCfg)),
	}

	// Ensure the container exists.
	err := writablePublicStorage.makeContainer(ecfg.publicBucket(), swift.PublicRead)
	if err != nil {
		panic(fmt.Errorf("cannot create writable public container: %v", err))
	}
	return writablePublicStorage
}

func InstanceAddress(addresses map[string][]nova.IPAddress) string {
	return instance.SelectPublicAddress(convertNovaAddresses(addresses))
}

var publicBucketIndexData = `
		{
		 "index": {
		  "com.ubuntu.cloud:released:openstack": {
		   "updated": "Wed, 01 May 2013 13:31:26 +0000",
		   "clouds": [
			{
			 "region": "{{.Region}}",
			 "endpoint": "{{.URL}}"
			}
		   ],
		   "cloudname": "test",
		   "datatype": "image-ids",
		   "format": "products:1.0",
		   "products": [
			"com.ubuntu.cloud:server:12.04:amd64",
			"com.ubuntu.cloud:server:12.10:amd64",
			"com.ubuntu.cloud:server:13.04:amd64"
		   ],
		   "path": "image-metadata/products.json"
		  }
		 },
		 "updated": "Wed, 01 May 2013 13:31:26 +0000",
		 "format": "index:1.0"
		}
`

var publicBucketImagesData = `
{
 "content_id": "com.ubuntu.cloud:released:openstack",
 "products": {
   "com.ubuntu.cloud:server:12.04:amd64": {
     "release": "precise",
     "version": "12.04",
     "arch": "amd64",
     "versions": {
       "20121218": {
         "items": {
           "inst1": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "some-region",
             "id": "1"
           },
           "inst2": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "another-region",
             "id": "2"
           }
         },
         "pubname": "ubuntu-precise-12.04-amd64-server-20121218",
         "label": "release"
       },
       "20121111": {
         "items": {
           "inst3": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "some-region",
             "id": "3"
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
             "root_store": "ebs",
             "virt": "pv",
             "region": "region-1",
             "id": "id-1"
           },
           "inst4": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "region-2",
             "id": "id-2"
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
             "root_store": "ebs",
             "virt": "pv",
             "region": "some-region",
             "id": "id-y"
           },
           "inst6": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "another-region",
             "id": "id-z"
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

const productMetadatafile = "image-metadata/products.json"

func UseTestImageData(e environs.Environ, cred *identity.Credentials) {
	// Put some image metadata files into the public storage.
	t := template.Must(template.New("").Parse(publicBucketIndexData))
	var metadata bytes.Buffer
	if err := t.Execute(&metadata, cred); err != nil {
		panic(fmt.Errorf("cannot generate index metdata: %v", err))
	}
	data := metadata.Bytes()
	WritablePublicStorage(e).Put(simplestreams.DefaultIndexPath+".json", bytes.NewReader(data), int64(len(data)))
	WritablePublicStorage(e).Put(
		productMetadatafile, strings.NewReader(publicBucketImagesData), int64(len(publicBucketImagesData)))
}

func RemoveTestImageData(e environs.Environ) {
	WritablePublicStorage(e).Remove(simplestreams.DefaultIndexPath + ".json")
	WritablePublicStorage(e).Remove(productMetadatafile)
}

func FindInstanceSpec(e environs.Environ, series, arch, cons string) (spec *instances.InstanceSpec, err error) {
	env := e.(*environ)
	spec, err = findInstanceSpec(env, &instances.InstanceConstraint{
		Series:      series,
		Arches:      []string{arch},
		Region:      env.ecfg().region(),
		Constraints: constraints.MustParse(cons),
	})
	return
}

func GetSwiftURL(e environs.Environ) (string, error) {
	return e.(*environ).client.MakeServiceURL("object-store", nil)
}

func SetUseFloatingIP(e environs.Environ, val bool) {
	env := e.(*environ)
	env.ecfg().attrs["use-floating-ip"] = val
}

func EnsureGroup(e environs.Environ, name string, rules []nova.RuleInfo) (nova.SecurityGroup, error) {
	return e.(*environ).ensureGroup(name, rules)
}
