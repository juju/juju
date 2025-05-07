// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type BootstrapConfigFileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&BootstrapConfigFileSuite{})

const testBootstrapConfigYAML = `
controllers:
  aws-test:
    controller-config:
      api-port: 17070
      state-port: 37017
    model-config:
      name: admin
      type: ec2
    controller-model-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
    credential: default
    cloud: aws
    type: ec2
    region: us-east-1
    endpoint: https://us-east-1.amazonaws.com
  mallards:
    controller-config:
      api-port: 17070
      state-port: 37017
    model-config:
      name: admin
      type: maas
    controller-model-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
    cloud: maas
    type: maas
    region: 127.0.0.1
    ca-certificates:
    - |
      -----BEGIN CERTIFICATE-----
      MIIC0DCCAbigAwIBAgIUeIj3r4ocrSubOmb1yPxmoiRfhO8wDQYJKoZIhvcNAQEL
      BQAwDzENMAsGA1UEAwwET1NDSTAeFw0xODA3MTUxNjE2NTZaFw0xODEwMjQxNjE2
      NTZaMA8xDTALBgNVBAMMBE9TQ0kwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK
      AoIBAQCpCLMLGZpLojudOwrupsbk2ESCQO4/kEOF6L5YHxcUrRxcrxu0DmnwWYcK
      pjKL9K3U7xSiSL+MtNff7MfBYbV0SOfjHR0/gqwio0JeYONABeeynUZkuXg1CXuG
      uMHcmPjCAWnLyAnlF4Wwavv6pPdM4l1X4lt1b2ez8G6u4+UPg/zNt473aqOzwMzy
      B3aToHSHOoDvXQDwtDkR0PimyEtHVz/17AcwSHzMqNGLgLFEx0SPuYJus8WJg1Sn
      c9kqrvIUBnZzjtbCquCxLRxG2xHdvBxOesbRyJPO0ypqEcTMtrX9rmJce67HG+4h
      EgLCEpcgfSVyH9PS3wdUAfkr9KE9AgMBAAGjJDAiMA8GA1UdEQQIMAaCBE9TQ0kw
      DwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAFIYyqNayVFxZ1jcz
      fdvEP2yVB9dq8vhSXU4lbkqlPw5q954bLURQzklqMfpXhhIbmrvq6LcLGaSkgmPp
      CzlxMkjr8oTRVQUqNIfcJQKtwNOAGh7xZ77GPhBlfHJ8VhTFtDXPM/fj8GLr5Oav
      gy9+QywhESKkwAn4+AubBRRtEDBX9zwc2hT5uqz1x1tcs16tKAZBIekwmMBJKkNs
      61I+cRHoXtXFh8/upMC6eMAvv6eVHgqpcEWrVLvoBh7ivcsFuUD1IyuIlN4i6roh
      xcSAzRCXqVe/BBsHqYyd8044vrIG7P7pYGaQm99nFGylTBfSh5g1LrYV7IJP6KkG
      6JHZXg==
      -----END CERTIFICATE-----
`

var fakecert = `
-----BEGIN CERTIFICATE-----
MIIC0DCCAbigAwIBAgIUeIj3r4ocrSubOmb1yPxmoiRfhO8wDQYJKoZIhvcNAQEL
BQAwDzENMAsGA1UEAwwET1NDSTAeFw0xODA3MTUxNjE2NTZaFw0xODEwMjQxNjE2
NTZaMA8xDTALBgNVBAMMBE9TQ0kwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK
AoIBAQCpCLMLGZpLojudOwrupsbk2ESCQO4/kEOF6L5YHxcUrRxcrxu0DmnwWYcK
pjKL9K3U7xSiSL+MtNff7MfBYbV0SOfjHR0/gqwio0JeYONABeeynUZkuXg1CXuG
uMHcmPjCAWnLyAnlF4Wwavv6pPdM4l1X4lt1b2ez8G6u4+UPg/zNt473aqOzwMzy
B3aToHSHOoDvXQDwtDkR0PimyEtHVz/17AcwSHzMqNGLgLFEx0SPuYJus8WJg1Sn
c9kqrvIUBnZzjtbCquCxLRxG2xHdvBxOesbRyJPO0ypqEcTMtrX9rmJce67HG+4h
EgLCEpcgfSVyH9PS3wdUAfkr9KE9AgMBAAGjJDAiMA8GA1UdEQQIMAaCBE9TQ0kw
DwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAFIYyqNayVFxZ1jcz
fdvEP2yVB9dq8vhSXU4lbkqlPw5q954bLURQzklqMfpXhhIbmrvq6LcLGaSkgmPp
CzlxMkjr8oTRVQUqNIfcJQKtwNOAGh7xZ77GPhBlfHJ8VhTFtDXPM/fj8GLr5Oav
gy9+QywhESKkwAn4+AubBRRtEDBX9zwc2hT5uqz1x1tcs16tKAZBIekwmMBJKkNs
61I+cRHoXtXFh8/upMC6eMAvv6eVHgqpcEWrVLvoBh7ivcsFuUD1IyuIlN4i6roh
xcSAzRCXqVe/BBsHqYyd8044vrIG7P7pYGaQm99nFGylTBfSh5g1LrYV7IJP6KkG
6JHZXg==
-----END CERTIFICATE-----
`[1:]

var testBootstrapConfig = map[string]jujuclient.BootstrapConfig{
	"aws-test": {
		ControllerConfig: controller.Config{
			"api-port":   17070,
			"state-port": 37017,
		},
		Config: map[string]interface{}{
			"type": "ec2",
			"name": "admin",
		},
		ControllerModelUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		Credential:          "default",
		Cloud:               "aws",
		CloudType:           "ec2",
		CloudRegion:         "us-east-1",
		CloudEndpoint:       "https://us-east-1.amazonaws.com",
	},
	"mallards": {
		ControllerConfig: controller.Config{
			"api-port":   17070,
			"state-port": 37017,
		},
		Config: map[string]interface{}{
			"type": "maas",
			"name": "admin",
		},
		ControllerModelUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		Cloud:               "maas",
		CloudType:           "maas",
		CloudRegion:         "127.0.0.1",
		CloudCACertificates: []string{fakecert},
	},
}

func (s *BootstrapConfigFileSuite) TestWriteFile(c *tc.C) {
	writeTestBootstrapConfigFile(c)
	data, err := os.ReadFile(osenv.JujuXDGDataHomePath("bootstrap-config.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, testBootstrapConfigYAML[1:])
}

func (s *BootstrapConfigFileSuite) TestReadNoFile(c *tc.C) {
	controllers, err := jujuclient.ReadBootstrapConfigFile("nohere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllers, tc.IsNil)
}

func (s *BootstrapConfigFileSuite) TestReadEmptyFile(c *tc.C) {
	path := osenv.JujuXDGDataHomePath("bootstrap-config.yaml")
	err := os.WriteFile(path, []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)

	configs, err := jujuclient.ReadBootstrapConfigFile(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(configs, tc.HasLen, 0)
}

func parseBootstrapConfig(c *tc.C) map[string]jujuclient.BootstrapConfig {
	configs, err := jujuclient.ParseBootstrapConfig([]byte(testBootstrapConfigYAML))
	c.Assert(err, jc.ErrorIsNil)
	return configs
}

func writeTestBootstrapConfigFile(c *tc.C) map[string]jujuclient.BootstrapConfig {
	configs := parseBootstrapConfig(c)
	err := jujuclient.WriteBootstrapConfigFile(configs)
	c.Assert(err, jc.ErrorIsNil)
	return configs
}

func (s *BootstrapConfigFileSuite) TestParseControllerMetadata(c *tc.C) {
	controllers := parseBootstrapConfig(c)
	var names []string
	for name := range controllers {
		names = append(names, name)
	}
	c.Assert(names, jc.SameContents, []string{"mallards", "aws-test"})
}

func (s *BootstrapConfigFileSuite) TestParseControllerMetadataError(c *tc.C) {
	controllers, err := jujuclient.ParseBootstrapConfig([]byte("fail me now"))
	c.Assert(err, tc.ErrorMatches, "cannot unmarshal bootstrap config: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `fail me...` into jujuclient.bootstrapConfigCollection")
	c.Assert(controllers, tc.IsNil)
}
