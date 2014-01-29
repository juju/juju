// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"launchpad.net/goose/errors"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/swift"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
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

// MetadataStorage returns a Storage instance which is used to store simplestreams metadata for tests.
func MetadataStorage(e environs.Environ) storage.Storage {
	ecfg := e.(*environ).ecfg()
	authModeCfg := AuthMode(ecfg.authMode())
	container := "juju-dist-test"
	metadataStorage := &openstackstorage{
		containerName: container,
		swift:         swift.New(e.(*environ).authClient(ecfg, authModeCfg)),
	}

	// Ensure the container exists.
	err := metadataStorage.makeContainer(container, swift.PublicRead)
	if err != nil {
		panic(fmt.Errorf("cannot create %s container: %v", container, err))
	}
	return metadataStorage
}

func InstanceAddress(addresses map[string][]nova.IPAddress) string {
	return instance.SelectPublicAddress(convertNovaAddresses(addresses))
}

var indexData = `
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

var imagesData = `
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

func UseTestImageData(stor storage.Storage, cred *identity.Credentials) {
	// Put some image metadata files into the public storage.
	t := template.Must(template.New("").Parse(indexData))
	var metadata bytes.Buffer
	if err := t.Execute(&metadata, cred); err != nil {
		panic(fmt.Errorf("cannot generate index metdata: %v", err))
	}
	data := metadata.Bytes()
	stor.Put(simplestreams.DefaultIndexPath+".json", bytes.NewReader(data), int64(len(data)))
	stor.Put(
		productMetadatafile, strings.NewReader(imagesData), int64(len(imagesData)))
}

func RemoveTestImageData(stor storage.Storage) {
	stor.Remove(simplestreams.DefaultIndexPath + ".json")
	stor.Remove(productMetadatafile)
}

// DiscardSecurityGroup cleans up a security group, it is not an error to
// delete something that doesn't exist.
func DiscardSecurityGroup(e environs.Environ, name string) error {
	env := e.(*environ)
	novaClient := env.nova()
	group, err := novaClient.SecurityGroupByName(name)
	if err != nil {
		if errors.IsNotFound(err) {
			// Group already deleted, done
			return nil
		}
	}
	err = novaClient.DeleteSecurityGroup(group.Id)
	if err != nil {
		return err
	}
	return nil
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

func ControlBucketName(e environs.Environ) string {
	env := e.(*environ)
	return env.ecfg().controlBucket()
}

func GetSwiftURL(e environs.Environ) (string, error) {
	return e.(*environ).client.MakeServiceURL("object-store", nil)
}

func SetUseFloatingIP(e environs.Environ, val bool) {
	env := e.(*environ)
	env.ecfg().attrs["use-floating-ip"] = val
}

func SetUpGlobalGroup(e environs.Environ, name string, statePort, apiPort int) (nova.SecurityGroup, error) {
	return e.(*environ).setUpGlobalGroup(name, statePort, apiPort)
}

func EnsureGroup(e environs.Environ, name string, rules []nova.RuleInfo) (nova.SecurityGroup, error) {
	return e.(*environ).ensureGroup(name, rules)
}

func CollectInstances(e environs.Environ, ids []instance.Id, out map[instance.Id]instance.Instance) []instance.Id {
	return e.(*environ).collectInstances(ids, out)
}

// ImageMetadataStorage returns a Storage object pointing where the goose
// infrastructure sets up its keystone entry for image metadata
func ImageMetadataStorage(e environs.Environ) storage.Storage {
	env := e.(*environ)
	return &openstackstorage{
		containerName: "imagemetadata",
		swift:         swift.New(env.client),
	}
}

// CreateCustomStorage creates a swift container and returns the Storage object
// so you can put data into it.
func CreateCustomStorage(e environs.Environ, containerName string) storage.Storage {
	env := e.(*environ)
	swiftClient := swift.New(env.client)
	if err := swiftClient.CreateContainer(containerName, swift.PublicRead); err != nil {
		panic(err)
	}
	return &openstackstorage{
		containerName: containerName,
		swift:         swiftClient,
	}
}

func GetNovaClient(e environs.Environ) *nova.Client {
	return e.(*environ).nova()
}

// ResolveNetwork exposes environ helper function resolveNetwork for testing
func ResolveNetwork(e environs.Environ, networkName string) (string, error) {
	return e.(*environ).resolveNetwork(networkName)
}
