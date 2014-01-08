// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"io"

	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/instance"
)

func ControlBucketName(e environs.Environ) string {
	return e.(*environ).ecfg().controlBucket()
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

func EnvironS3(e environs.Environ) *s3.S3 {
	return e.(*environ).s3()
}

func InstanceEC2(inst instance.Instance) *ec2.Instance {
	return inst.(*ec2Instance).Instance
}

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

var testRoundTripper = &jujutest.ProxyRoundTripper{}

func init() {
	// Prepare mock http transport for overriding metadata and images output in tests.
	testRoundTripper.RegisterForScheme("test")
}

// TODO: Apart from overriding different hardcoded hosts, these two test helpers are identical. Let's share.

var origImagesUrl = imagemetadata.DefaultBaseURL

// UseTestImageData causes the given content to be served
// when the ec2 client asks for image data.
func UseTestImageData(files map[string]string) {
	if files != nil {
		testRoundTripper.Sub = jujutest.NewCannedRoundTripper(files, nil)
		imagemetadata.DefaultBaseURL = "test:"
		signedImageDataOnly = false
	} else {
		signedImageDataOnly = true
		testRoundTripper.Sub = nil
		imagemetadata.DefaultBaseURL = origImagesUrl
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

var origMetadataHost = metadataHost

func UseTestMetadata(files map[string]string) {
	if files != nil {
		testRoundTripper.Sub = jujutest.NewCannedRoundTripper(files, nil)
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

var TestImagesData = map[string]string{
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
			"com.ubuntu.cloud:server:12.04:amd64",
			"com.ubuntu.cloud:server:12.04:i386",
			"com.ubuntu.cloud:server:12.04:amd64",
			"com.ubuntu.cloud:server:12.10:amd64",
			"com.ubuntu.cloud:server:13.04:i386"
		   ],
		   "path": "streams/v1/com.ubuntu.cloud:released:aws.js"
		  }
		 },
		 "updated": "Wed, 01 May 2013 13:31:26 +0000",
		 "format": "index:1.0"
		}
`,
	"/streams/v1/com.ubuntu.cloud:released:aws.js": `
{
 "content_id": "com.ubuntu.cloud:released:aws",
 "products": {
   "com.ubuntu.cloud:server:12.04:amd64": {
     "release": "precise",
     "version": "12.04",
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
             "root_store": "ebs",
             "virt": "pv",
             "region": "eu-west-1",
             "id": "ami-00000016"
           },
           "apne1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-00000026"
           },
           "apne1he": {
             "root_store": "ebs",
             "virt": "hvm",
             "region": "ap-northeast-1",
             "id": "ami-00000087"
           },
           "test1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "test",
             "id": "ami-00000033"
           },
           "test1he": {
             "root_store": "ebs",
             "virt": "hvm",
             "region": "test",
             "id": "ami-00000035"
           }
         },
         "pubname": "ubuntu-precise-12.04-amd64-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:12.04:i386": {
     "release": "precise",
     "version": "12.04",
     "arch": "i386",
     "versions": {
       "20121218": {
         "items": {
           "test1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "test",
             "id": "ami-00000034"
           },
           "apne1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-00000023"
           }
         },
         "pubname": "ubuntu-precise-12.04-i386-server-20121218",
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
             "root_store": "ebs",
             "virt": "pv",
             "region": "eu-west-1",
             "id": "ami-01000016"
           },
           "apne1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-01000026"
           },
           "apne1he": {
             "root_store": "ebs",
             "virt": "hvm",
             "region": "ap-northeast-1",
             "id": "ami-01000087"
           },
           "test1he": {
             "root_store": "ebs",
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
             "root_store": "ebs",
             "virt": "pv",
             "region": "test",
             "id": "ami-01000034"
           },
           "apne1pe": {
             "root_store": "ebs",
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
             "root_store": "ebs",
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
	"t1.micro":    20,
	"c1.medium":   145,
	"c1.xlarge":   580,
	"cc1.4xlarge": 1300,
	"cc2.8xlarge": 2400,
}

var TestRegions = map[string]aws.Region{
	"test": {
		Name:        "test",
		EC2Endpoint: "https://ec2.endpoint.com",
	},
}
