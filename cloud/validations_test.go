// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cloud"
)

func (s *cloudSuite) TestValidateValidCloud(c *tc.C) {
	validCloud := `
          clouds:
            vmwarestack-trusty:
              type: maas
              description: A mass cloud
              auth-types: [oauth1]
              host-cloud-region: host/region
              endpoint: http://10.245.200.27/MAAS
              config:
                default-series: trusty
                bootstrap-timeout: 900
                http-proxy: http://10.245.200.27:8000/
              regions:
                dev1:
                  endpoint: https://openstack.example.com:35574/v3.0/
              ca-certificates:
              - |-
                -----BEGIN CERTIFICATE-----
                -----END CERTIFICATE-----`

	yaml := []byte(validCloud)
	err := cloud.ValidateCloudSet(yaml)
	c.Assert(err, tc.IsNil)
}

func (s *cloudSuite) TestValidateInvalidCloud(c *tc.C) {
	validCloud := `
          clouds:
            vmwarestack-trusty:
              tpe: maas
              descript: A mass cloud
              auth-types: [oauth1]
              endpoint: http://10.245.200.27/MAAS
              config:
                default-series: trusty
                bootstrap-timeout: 900
                http-proxy: http://10.245.200.27:8000/
              regions:
                dev1:
                  endpont: https://openstack.example.com:35574/v3.0/`

	yaml := []byte(validCloud)
	err := cloud.ValidateCloudSet(yaml)
	c.Assert(err.Error(), jc.Contains, `"endpont" is invalid. Perhaps you mean "endpoint"`)
	c.Assert(err.Error(), jc.Contains, `"descript" is invalid. Perhaps you mean "description"`)
	c.Assert(err.Error(), jc.Contains, `"tpe" is invalid. Perhaps you mean "type"`)
}

func (s *cloudSuite) TestValidateMultipleValidClouds(c *tc.C) {
	validCloud := `
          clouds:
            vmwarestack-trusty:
              type: maas
              auth-types: [oauth1]
              endpoint: http://10.245.200.27/MAAS
              config:
                default-series: trusty
                bootstrap-timeout: 900
                http-proxy: http://10.245.200.27:8000/
              regions:
                dev1:
                  endpoint: https://openstack.example.com:35574/v3.0/
            vmwarestack-xenial:
              type: maas
              auth-types: [oauth1]
              endpoint: http://10.245.200.28/MAAS
              config:
                default-series: xenial
                bootstrap-timeout: 900
                http-proxy: http://10.245.200.28:8000/
              regions:
                dev1:
                  endpoint: https://openstack.example.com:35575/v3.0/`

	yaml := []byte(validCloud)
	err := cloud.ValidateCloudSet(yaml)
	c.Assert(err, tc.IsNil)
}

func (s *cloudSuite) TestValidateMultipleInvalidClouds(c *tc.C) {
	validCloud := `
          clouds:
            vmwarestack-trusty:
              type: maas
              auth-types: [oauth1]
              endpoint: http://10.245.200.27/MAAS
              config:
                default-series: trusty
                bootstrap-timeout: 900
                http-proxy: http://10.245.200.27:8000/
              regions:
                dev1:
                  endpoint: https://openstack.example.com:35574/v3.0/
            vmwarestack-xenial:
              type: maas
              auth-tpes: [oauth1]
              endpoit: http://10.245.200.28/MAAS
              config:
                default-series: xenial
                bootstrap-timeout: 900
                http-proxy: http://10.245.200.28:8000/
              regions:
                dev1:
                  endpoint: https://openstack.example.com:35575/v3.0/`

	yaml := []byte(validCloud)
	err := cloud.ValidateCloudSet(yaml)
	c.Assert(err.Error(), jc.Contains, `"endpoit" is invalid. Perhaps you mean "endpoint"`)
	c.Assert(err.Error(), jc.Contains, `"auth-tpes" is invalid. Perhaps you mean "auth-types"`)
}

func (s *cloudSuite) TestValidateInvalidPropertyWithNoSuggestion(c *tc.C) {
	validCloud := `
          clouds:
            vmwarestack-trusty:
              type: maas
              auth-types: [oauth1]
              endpoint: http://10.245.200.27/MAAS
              config:
                default-series: trusty
                bootstrap-timeout: 900
                http-proxy: http://10.245.200.27:8000/
              regions:
                dev1:
                  endpoint: https://openstack.example.com:35574/v3.0/
            vmwarestack-xenial:
              type: maas
              auth-types: [oauth1]
              invalidProperty: "something strange"
              endpoit: http://10.245.200.28/MAAS
              config:
                default-series: xenial
                bootstrap-timeout: 900
                http-proxy: http://10.245.200.28:8000/
              regions:
                dev1:
                  endpoint: https://openstack.example.com:35575/v3.0/`

	yaml := []byte(validCloud)
	err := cloud.ValidateCloudSet(yaml)
	c.Assert(err.Error(), jc.Contains, `"endpoit" is invalid. Perhaps you mean "endpoint"`)
	c.Assert(err.Error(), jc.Contains, `"invalidProperty" is invalid.`)
}

func (s *cloudSuite) TestValidateOneValidCloud(c *tc.C) {
	validCloud := `
          name: vmwarestack-trusty
          type: maas
          description: A mass cloud
          auth-types: [oauth1]
          endpoint: http://10.245.200.27/MAAS
          config:
            default-series: trusty
            bootstrap-timeout: 900
            http-proxy: http://10.245.200.27:8000/
            regions:
              dev1:
                endpoint: https://openstack.example.com:35574/v3.0/`

	yaml := []byte(validCloud)
	err := cloud.ValidateOneCloud(yaml)
	c.Assert(err, tc.IsNil)
}

func (s *cloudSuite) TestValidateOneInvalidCloud(c *tc.C) {
	validCloud := `
          nae: vmwarestack-trusty
          type: maas
          escription: A mass cloud
          auth-types: [oauth1]
          endpoint: http://10.245.200.27/MAAS
          config:
            default-series: trusty
            bootstrap-timeout: 900
            http-proxy: http://10.245.200.27:8000/
            regions:
              dev1:
                endpoint: https://openstack.example.com:35574/v3.0/`

	yaml := []byte(validCloud)
	err := cloud.ValidateOneCloud(yaml)
	c.Assert(err.Error(), jc.Contains, `"nae" is invalid. Perhaps you mean "name"`)
	c.Assert(err.Error(), jc.Contains, `"escription" is invalid. Perhaps you mean "description"`)
}
