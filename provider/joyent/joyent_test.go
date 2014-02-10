// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"bytes"
	"strings"
	"text/template"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	jp "launchpad.net/juju-core/provider/joyent"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"

	"launchpad.net/gojoyent/jpc"
)

const (
	testUser        = "test"
	testKeyFileName = "provider_id_rsa"
	testPrivateKey  = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAza+KvczCrcpQGRq9e347VHx9oEvuhseJt0ydR+UMAveyQprU
4JHvzwUUhGnG147GJQYyfQ4nzaSG62az/YThoZJzw8gtxGkVHv0wlAlRkYhxbKbq
8WQIh73xDQkHLw2lXLvf7Tt0Mhow0qGEmkOjTb5fPsj2evphrV3jJ15QlhL4cv33
t8jVadIrL0iIpwdqWiPqUKpsrSfKJghkoXS6quPy78P820TnuoBG+/Ppr8Kkvn6m
A7j4xnOQ12QE6jPK4zkikj5ZczSC4fTG0d3BwwX4VYu+4y/T/BX0L9VNUmQU22Y+
/MRXAUZxsa8VhNB+xXF5XSubyb2n6loMWWaYGwIDAQABAoIBAQDCJt9JxYxGS+BL
sigF9+O9Hj3fH42p/5QJR/J2uMgbzP+hS1GCIX9B5MO3MbmWI5j5vd3OmZwMyy7n
6Wwg9FufDgTkW4KIEcD0HX7LXfh27VpTe0PuU8SRjUOKUGlNiw36eQUog6Rs3rgT
Oo9Wpl3xtq9lLoErGEk3QpZ2xNpArTfsN9N3pdmD4sv7wmJq0PZQyej482g9R0g/
5k2ni6JpcEifzBQ6Bzx3EV2l9UipEIqbqDpMOtYFCpnLQhEaDfUribqXINGIsjiq
VyFa3Mbg/eayqG3UX3rVTCif2NnW2ojl4mMgWCyxgWfb4Jg1trc3v7X4SXfbgPWD
WcfrOhOhAoGBAP7ZC8KHAnjujwvXf3PxVNm6CTs5evbByQVyxNciGxRuOomJIR4D
euqepQ4PuNAabnrbMyQWXpibIKpmLnBVoj1q0IMXYvi2MZF5e2tH/Gx01UvxamHh
bKhHmp9ImHhVl6kObXOdNvLVTt/BI5FZBblvm7qOoiVwImPbqqVHP7Q5AoGBAM6d
mNsrW0iV/nP1m7d8mcFw74PI0FNlNdfUoePUgokO0t5OU0Ri/lPBDCRGlvVF3pj1
HnmwroNtdWr9oPVB6km8193fb2zIWe53tj+6yRFQpz5elrSPfeZaZXlJZAGCCCdN
gBggWQFPeQiT54aPywPpcTZHIs72XBqQ6QsIPrbzAoGAdW2hg5MeSobyFuzHZ69N
/70/P7DuvgDxFbeah97JR5K7GmC7h87mtnE/cMlByXJEcgvK9tfv4rWoSZwnzc9H
oLE1PxJpolyhXnzxp69V2svC9OlasZtjq+7Cip6y0s/twBJL0Lgid6ZeX6/pKbIx
dw68XSwX/tQ6pHS1ns7DxdECgYBJbBWapNCefbbbjEcWsC+PX0uuABmP2SKGHSie
ZrEwdVUX7KuIXMlWB/8BkRgp9vdAUbLPuap6R9Z2+8RMA213YKUxUiotdREIPgBE
q2KyRX/5GPHjHi62Qh9XN25TXtr45ICFklEutwgitTSMS+Lv8+/oQuUquL9ILYCz
C+4FYwKBgQDE9yZTUpJjG2424z6bl/MHzwl5RB4pMronp0BbeVqPwhCBfj0W5I42
1ZL4+8eniHfUs4GXzf5tb9YwVt3EltIF2JybaBvFsv2o356yJUQmqQ+jyYRoEpT5
2SwilFo/XCotCXxi5n8sm9V94a0oix4ehZrohTA/FZLsggwFCPmXfw==
-----END RSA PRIVATE KEY-----`
	testKeyFingerprint = "66:ca:1c:09:75:99:35:69:be:91:08:25:03:c0:17:c0"
)

func TestJoyentProvider(t *stdtesting.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	testbase.LoggingSuite
	envtesting.ToolsFixture
	restoreTimeouts func()
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.restoreTimeouts = envtesting.PatchAttemptStrategies()
	s.LoggingSuite.SetUpSuite(c)
	CreateTestKey()
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	RemoveTestKey()
	s.restoreTimeouts()
	s.LoggingSuite.TearDownSuite(c)
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func GetFakeConfig(sdcUrl, mantaUrl string) coretesting.Attrs {
	return coretesting.FakeConfig().Merge(coretesting.Attrs{
		"name":         	"joyent test environment",
		"type":         	"joyent",
		"sdc-user":     	testUser,
		"sdc-key-id":   	testKeyFingerprint,
		"sdc-url":      	sdcUrl,
		"manta-user":   	testUser,
		"manta-key-id": 	testKeyFingerprint,
		"manta-url":    	mantaUrl,
		"key-file":     	fmt.Sprintf("%s/.ssh/%s", os.Getenv("HOME"), testKeyFileName),
		"algorithm":    	"rsa-sha256",
		"control-dir":  	"juju-test",
		"agent-version":    version.Current.Number.String(),
	})
}

// makeEnviron creates a functional Joyent environ for a test.
func MakeEnviron(sdcUrl, mantaUrl string) *jp.JoyentEnviron {
	attrs := GetFakeConfig(sdcUrl, mantaUrl)
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		panic(err)
	}
	env, err := jp.NewEnviron(cfg)
	if err != nil {
		panic(err)
	}
	return env
}

func CreateTestKey() error {
	keyFile := fmt.Sprintf("%s/.ssh/%s", os.Getenv("HOME"), testKeyFileName)
	return ioutil.WriteFile(keyFile, []byte(testPrivateKey), 400)
}

func RemoveTestKey() error {
	keyFile := fmt.Sprintf("%s/.ssh/%s", os.Getenv("HOME"), testKeyFileName)
	return os.Remove(keyFile)
}

// MetadataStorage returns a Storage instance which is used to store simplestreams metadata for tests.
func MetadataStorage(e environs.Environ) storage.Storage {
	container := "juju-dist-test"
	metadataStorage := jp.NewStorage(e.(*jp.JoyentEnviron), container)

	// Ensure the container exists.
	err := metadataStorage.(*jp.JoyentStorage).CreateContainer()
	if err != nil {
		panic(fmt.Errorf("cannot create %s container: %v", container, err))
	}
	return metadataStorage
}

// ImageMetadataStorage returns a Storage object pointing where the goose
// infrastructure sets up its keystone entry for image metadata
func ImageMetadataStorage(e environs.Environ) storage.Storage {
	env := e.(*jp.JoyentEnviron)
	return jp.NewStorage(env, "imagedata")
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

func UseTestImageData(stor storage.Storage, creds *jpc.Credentials) {
	// Put some image metadata files into the public storage.
	t := template.Must(template.New("").Parse(indexData))
	var metadata bytes.Buffer
	if err := t.Execute(&metadata, creds); err != nil {
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

func FindInstanceSpec(e environs.Environ, series, arch, cons string) (spec *instances.InstanceSpec, err error) {
	env := e.(*jp.JoyentEnviron)
	spec, err = env.FindInstanceSpec(&instances.InstanceConstraint{
		Series:      series,
		Arches:      []string{arch},
		Region:      env.Ecfg().Region(),
		Constraints: constraints.MustParse(cons),
	})
	return
}

func ControlBucketName(e environs.Environ) string {
	env := e.(*jp.JoyentEnviron)
	return env.Ecfg().ControlDir()
}
