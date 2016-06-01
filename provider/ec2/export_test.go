// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"io"

	"gopkg.in/amz.v3/aws"
	"gopkg.in/amz.v3/ec2"
	"gopkg.in/amz.v3/s3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	jujustorage "github.com/juju/juju/storage"
)

func EBSProvider() jujustorage.Provider {
	return &ebsProvider{}
}

func StorageEC2(vs jujustorage.VolumeSource) *ec2.EC2 {
	return vs.(*ebsVolumeSource).ec2
}

func JujuGroupName(e environs.Environ) string {
	return e.(*environ).jujuGroupName()
}

func MachineGroupName(e environs.Environ, machineId string) string {
	return e.(*environ).machineGroupName(machineId)
}

func EnvironEC2(e environs.Environ) *ec2.EC2 {
	return e.(*environ).ec2()
}

func InstanceEC2(inst instance.Instance) *ec2.Instance {
	return inst.(*ec2Instance).Instance
}

func TerminatedInstances(e environs.Environ) ([]instance.Instance, error) {
	return e.(*environ).AllInstancesByState("shutting-down", "terminated")
}

func InstanceSecurityGroups(e environs.Environ, ids []instance.Id, states ...string) ([]ec2.SecurityGroup, error) {
	return e.(*environ).instanceSecurityGroups(ids, states...)
}

var (
	EC2AvailabilityZones        = &ec2AvailabilityZones
	AvailabilityZoneAllocations = &availabilityZoneAllocations
	RunInstances                = &runInstances
	BlockDeviceNamer            = blockDeviceNamer
	GetBlockDeviceMappings      = getBlockDeviceMappings
	IsVPCNotUsableError         = isVPCNotUsableError
	IsVPCNotRecommendedError    = isVPCNotRecommendedError
)

const VPCIDNone = vpcIDNone

// BucketStorage returns a storage instance addressing
// an arbitrary s3 bucket.
func BucketStorage(b *s3.Bucket) storage.Storage {
	return &ec2storage{
		bucket: b,
	}
}

// DeleteBucket deletes the s3 bucket used by the storage instance.
func DeleteBucket(s storage.Storage) error {
	return deleteBucket(s.(*ec2storage))
}

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

func UseTestRegionData(content map[string]aws.Region) {
	if content != nil {
		allRegions = content
	} else {
		allRegions = aws.Regions
	}
}

// UseTestInstanceTypeData causes the given instance type
// cost data to be served for the "test" region.
func UseTestInstanceTypeData(content instanceTypeCost) {
	if content != nil {
		allRegionCosts["test"] = content
	} else {
		delete(allRegionCosts, "test")
	}
}

var (
	ShortAttempt                   = &shortAttempt
	StorageAttempt                 = &storageAttempt
	DestroyVolumeAttempt           = &destroyVolumeAttempt
	DeleteSecurityGroupInsistently = &deleteSecurityGroupInsistently
	TerminateInstancesById         = &terminateInstancesById
)

func EC2ErrCode(err error) string {
	return ec2ErrCode(err)
}

// FabricateInstance creates a new fictitious instance
// given an existing instance and a new id.
func FabricateInstance(inst instance.Instance, newId string) instance.Instance {
	oldi := inst.(*ec2Instance)
	newi := &ec2Instance{
		e:        oldi.e,
		Instance: &ec2.Instance{},
	}
	*newi.Instance = *oldi.Instance
	newi.InstanceId = newId
	return newi
}

// Access non exported methods on ec2.storage
type Storage interface {
	Put(file string, r io.Reader, length int64) error
	ResetMadeBucket()
}

func (s *ec2storage) ResetMadeBucket() {
	s.Lock()
	defer s.Unlock()
	s.madeBucket = false
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
	// 16.04:amd64
	makeImage("ami-00000133", "ssd", "pv", "amd64", "16.04", "test"),
	makeImage("ami-00000139", "ebs", "pv", "amd64", "16.04", "test"),
	makeImage("ami-00000135", "ssd", "hvm", "amd64", "16.04", "test"),

	// 14.04:amd64
	makeImage("ami-00000033", "ssd", "pv", "amd64", "14.04", "test"),

	// 14.04:i386
	makeImage("ami-00000034", "ssd", "pv", "i386", "14.04", "test"),

	// 12.10:amd64
	makeImage("ami-01000035", "ssd", "hvm", "amd64", "12.10", "test"),

	// 12.10:i386
	makeImage("ami-01000034", "ssd", "pv", "i386", "12.10", "test"),

	// 13.04:i386
	makeImage("ami-02000034", "ssd", "pv", "i386", "13.04", "test"),
}

var TestImagesData = map[string]string{
	// LTS-dependent requires new/updated entries upon new LTS release.
	"/streams/v1/index.json": `
        {
         "index": {
          "com.ubuntu.cloud:released": {
           "updated": "Wed, 01 May 2013 13:31:26 +0000",
           "clouds": [
            {
             "region": "test",
             "endpoint": "https://ec2.endpoint.com"
            }
           ],
           "cloudname": "aws",
           "datatype": "image-ids",
           "format": "products:1.0",
           "products": [
            "com.ubuntu.cloud:server:16.04:amd64",
            "com.ubuntu.cloud:server:14.04:amd64",
            "com.ubuntu.cloud:server:14.04:i386",
            "com.ubuntu.cloud:server:12.10:i386",
            "com.ubuntu.cloud:server:13.04:i386"
           ],
           "path": "streams/v1/com.ubuntu.cloud:released:aws.json"
          }
         },
         "updated": "Wed, 01 May 2013 13:31:26 +0000",
         "format": "index:1.0"
        }
`,
	"/streams/v1/com.ubuntu.cloud:released:aws.json": `
{
 "content_id": "com.ubuntu.cloud:released:aws",
 "products": {
   "com.ubuntu.cloud:server:16.04:amd64": {
     "release": "trusty",
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
         "pubname": "ubuntu-trusty-16.04-amd64-server-20121218",
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
             "virt": "pv",
             "region": "test",
             "id": "ami-00000033"
			}
         },
         "pubname": "ubuntu-trusty-14.04-amd64-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:14.04:i386": {
     "release": "trusty",
     "version": "14.04",
     "arch": "i386",
     "versions": {
       "20121218": {
         "items": {
           "test1pe": {
             "root_store": "ssd",
             "virt": "pv",
             "region": "test",
             "id": "ami-00000034"
           }
         },
         "pubname": "ubuntu-trusty-14.04-i386-server-20121218",
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
   },
   "com.ubuntu.cloud:server:12.10:i386": {
     "release": "quantal",
     "version": "12.10",
     "arch": "i386",
     "versions": {
       "20121218": {
         "items": {
           "test1pe": {
             "root_store": "ssd",
             "virt": "pv",
             "region": "test",
             "id": "ami-01000034"
           },
           "apne1pe": {
             "root_store": "ssd",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-01000023"
           }
         },
         "pubname": "ubuntu-quantal-12.10-i386-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:13.04:i386": {
     "release": "raring",
     "version": "13.04",
     "arch": "i386",
     "versions": {
       "20121218": {
         "items": {
           "test1pe": {
             "root_store": "ssd",
             "virt": "pv",
             "region": "test",
             "id": "ami-02000034"
           }
         },
         "pubname": "ubuntu-raring-13.04-i386-server-20121218",
         "label": "release"
       }
     }
   }
 },
 "format": "products:1.0"
}
`,
}

var TestInstanceTypeCosts = instanceTypeCost{
	"m1.small":    60,
	"m1.medium":   120,
	"m1.large":    240,
	"m1.xlarge":   480,
	"m3.medium":   95,
	"m3.large":    190,
	"m3.xlarge":   385,
	"m3.2xlarge":  765,
	"t1.micro":    20,
	"c1.medium":   145,
	"c1.xlarge":   580,
	"cc2.8xlarge": 2400,
}

var TestRegions = map[string]aws.Region{
	"test": {
		Name:        "test",
		EC2Endpoint: "https://ec2.endpoint.com",
	},
}
