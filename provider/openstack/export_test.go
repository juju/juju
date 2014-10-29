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

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// This provides the content for code accessing test:///... URLs. This allows
// us to set the responses for things like the Metadata server, by pointing
// metadata requests at test:///... rather than http://169.254.169.254
var testRoundTripper = &jujutest.ProxyRoundTripper{}

func init() {
	testRoundTripper.RegisterForScheme("test")
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

func InstanceAddress(publicIP string, addresses map[string][]nova.IPAddress) string {
	return network.SelectPublicAddress(convertNovaAddresses(publicIP, addresses))
}

func InstanceServerDetail(inst instance.Instance) *nova.ServerDetail {
	return inst.(*openstackInstance).serverDetail
}

func InstanceFloatingIP(inst instance.Instance) *nova.FloatingIP {
	return inst.(*openstackInstance).floatingIP
}

var (
	NovaListAvailabilityZones   = &novaListAvailabilityZones
	AvailabilityZoneAllocations = &availabilityZoneAllocations
)

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
			"com.ubuntu.cloud:server:14.04:amd64",
			"com.ubuntu.cloud:server:14.04:i386",
			"com.ubuntu.cloud:server:14.04:ppc64el",
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
   "com.ubuntu.cloud:server:14.04:amd64": {
     "release": "trusty",
     "version": "14.04",
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
         "pubname": "ubuntu-trusty-14.04-amd64-server-20121218",
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
         "pubname": "ubuntu-trusty-14.04-amd64-server-20121111",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:14.04:i386": {
     "release": "trusty",
     "version": "14.04",
     "arch": "i386",
     "versions": {
       "20121111": {
         "items": {
           "inst33": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "some-region",
             "id": "33"
           }
         },
         "pubname": "ubuntu-trusty-14.04-i386-server-20121111",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:14.04:ppc64el": {
     "release": "trusty",
     "version": "14.04",
     "arch": "ppc64el",
     "versions": {
       "20121111": {
         "items": {
           "inst33": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "some-region",
             "id": "33"
           }
         },
         "pubname": "ubuntu-trusty-14.04-ppc64el-server-20121111",
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
	stor.Put(simplestreams.UnsignedIndex(imagemetadata.StreamsVersionV1), bytes.NewReader(data), int64(len(data)))
	stor.Put(
		productMetadatafile, strings.NewReader(imagesData), int64(len(imagesData)))
}

func RemoveTestImageData(stor storage.Storage) {
	stor.Remove(simplestreams.UnsignedIndex("v1"))
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

func SetUpGlobalGroup(e environs.Environ, name string, apiPort int) (nova.SecurityGroup, error) {
	return e.(*environ).setUpGlobalGroup(name, apiPort)
}

func EnsureGroup(e environs.Environ, name string, rules []nova.RuleInfo) (nova.SecurityGroup, error) {
	return e.(*environ).ensureGroup(name, rules)
}

func CollectInstances(e environs.Environ, ids []instance.Id, out map[string]instance.Instance) []instance.Id {
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

var PortsToRuleInfo = portsToRuleInfo
var RuleMatchesPortRange = ruleMatchesPortRange

var MakeServiceURL = &makeServiceURL
