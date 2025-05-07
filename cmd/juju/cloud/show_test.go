// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"context"
	"os"
	"strings"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"

	cloudapi "github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type showSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api   *fakeShowCloudAPI
	store *jujuclient.MemStore
}

var _ = tc.Suite(&showSuite{})

func (s *showSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.api = &fakeShowCloudAPI{}
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *showSuite) TestShowBadArgs(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand())
	c.Assert(err, tc.ErrorMatches, "no cloud specified")
}

func (s *showSuite) assertShowLocal(c *tc.C, expectedOutput string) {
	command := cloud.NewShowCloudCommandForTest(
		s.store,
		func(ctx context.Context) (cloud.ShowCloudAPI, error) {
			c.Fail()
			return s.api, nil
		})
	ctx, err := cmdtesting.RunCommand(c, command, "aws-china", "--client")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, expectedOutput)
}

func (s *showSuite) TestShowLocal(c *tc.C) {
	s.assertShowLocal(c, `
Client cloud "aws-china":

defined: public
type: ec2
description: Amazon China
auth-types: [access-key]
regions:
  cn-north-1:
    endpoint: https://ec2.cn-north-1.amazonaws.com.cn
  cn-northwest-1:
    endpoint: https://ec2.cn-northwest-1.amazonaws.com.cn
`[1:])
}

func (s *showSuite) TestShowLocalWithDefaultCloud(c *tc.C) {
	s.store.Credentials["aws-china"] = jujucloud.CloudCredential{DefaultRegion: "cn-north-1"}
	s.assertShowLocal(c, `
Client cloud "aws-china":

defined: public
type: ec2
description: Amazon China
auth-types: [access-key]
default-region: cn-north-1
regions:
  cn-north-1:
    endpoint: https://ec2.cn-north-1.amazonaws.com.cn
  cn-northwest-1:
    endpoint: https://ec2.cn-northwest-1.amazonaws.com.cn
`[1:])
}

func (s *showSuite) TestShowKubernetes(c *tc.C) {
	s.api.cloud = jujucloud.Cloud{
		Name:        "beehive",
		Type:        "kubernetes",
		Description: "Bumble Bees",
		AuthTypes:   []jujucloud.AuthType{"userpass"},
		Endpoint:    "http://cluster",
		Regions: []jujucloud.Region{
			{
				Name:     "default",
				Endpoint: "http://cluster/default",
			},
		},
		SkipTLSVerify: true,
	}
	command := cloud.NewShowCloudCommandForTest(
		s.store,
		func(ctx context.Context) (cloud.ShowCloudAPI, error) {
			return s.api, nil
		})
	ctx, err := cmdtesting.RunCommand(c, command, "--controller", "mycontroller", "beehive")
	c.Assert(err, tc.ErrorIsNil)
	s.api.CheckCallNames(c, "CloudInfo", "Close")
	c.Assert(command.ControllerName, tc.Equals, "mycontroller")
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, `
Cloud "beehive" from controller "mycontroller":

defined: public
type: k8s
description: Bumble Bees
auth-types: [userpass]
endpoint: http://cluster
regions:
  default:
    endpoint: http://cluster/default
users:
  fred:
    display-name: Fred
    access: admin
skip-tls-verify: true
`[1:])
}

func (s *showSuite) setupRemoteCloud(cloudName string) {
	s.api.cloud = jujucloud.Cloud{
		Name:        cloudName,
		Type:        "openstack",
		Description: "Bumble Bees",
		AuthTypes:   []jujucloud.AuthType{"userpass", "access-key"},
		Endpoint:    "http://myopenstack",
		Regions: []jujucloud.Region{
			{
				Name:     "regionone",
				Endpoint: "http://boston/1.0",
			},
		},
	}
}

func (s *showSuite) TestShowControllerCloudNoLocal(c *tc.C) {
	s.setupRemoteCloud("beehive")
	command := cloud.NewShowCloudCommandForTest(
		s.store,
		func(ctx context.Context) (cloud.ShowCloudAPI, error) {
			return s.api, nil
		})
	ctx, err := cmdtesting.RunCommand(c, command, "beehive", "-c", "mycontroller")
	c.Assert(err, tc.ErrorIsNil)
	s.api.CheckCallNames(c, "CloudInfo", "Close")
	c.Assert(command.ControllerName, tc.Equals, "mycontroller")
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, `
Cloud "beehive" from controller "mycontroller":

defined: public
type: openstack
description: Bumble Bees
auth-types: [userpass, access-key]
endpoint: http://myopenstack
regions:
  regionone:
    endpoint: http://boston/1.0
users:
  fred:
    display-name: Fred
    access: admin
`[1:])
}

func (s *showSuite) TestShowControllerAndLocalCloud(c *tc.C) {
	s.setupRemoteCloud("aws-china")
	command := cloud.NewShowCloudCommandForTest(
		s.store,
		func(ctx context.Context) (cloud.ShowCloudAPI, error) {
			return s.api, nil
		})
	ctx, err := cmdtesting.RunCommand(c, command, "aws-china")
	c.Assert(err, tc.ErrorIsNil)
	s.api.CheckCallNames(c, "CloudInfo", "Close")
	c.Assert(command.ControllerName, tc.Equals, "mycontroller")
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, `
Cloud "aws-china" from controller "mycontroller":

defined: public
type: openstack
description: Bumble Bees
auth-types: [userpass, access-key]
endpoint: http://myopenstack
regions:
  regionone:
    endpoint: http://boston/1.0
users:
  fred:
    display-name: Fred
    access: admin

Client cloud "aws-china":

defined: public
type: ec2
description: Amazon China
auth-types: [access-key]
regions:
  cn-north-1:
    endpoint: https://ec2.cn-north-1.amazonaws.com.cn
  cn-northwest-1:
    endpoint: https://ec2.cn-northwest-1.amazonaws.com.cn

`[1:])
}

func (s *showSuite) TestShowWithConfig(c *tc.C) {
	data := `
clouds:
  homestack:
    type: openstack
    description: Openstack Cloud
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
    config:
      bootstrap-timeout: 1800
      use-default-secgroup: true
`[1:]
	err := os.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)
	c.Assert(err, tc.ErrorIsNil)
	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "homestack", "--client")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, `
Client cloud "homestack":

defined: local
type: openstack
description: Openstack Cloud
auth-types: [userpass, access-key]
endpoint: http://homestack
regions:
  london:
    endpoint: http://london/1.0
config:
  bootstrap-timeout: 1800
  use-default-secgroup: true
`[1:])
}

var openstackProviderConfig = `
The available config options specific to openstack clouds are:
external-network:
  type: string
  description: The network label or UUID to create floating IP addresses on when multiple
    external networks exist.
network:
  type: string
  description: The network label or UUID to bring machines up on when multiple networks
    exist.
policy-target-group:
  type: string
  description: The UUID of Policy Target Group to use for Policy Targets created.
use-default-secgroup:
  type: bool
  description: Whether new machine instances should have the "default" Openstack security
    group assigned in addition to juju defined security groups.
use-openstack-gbp:
  type: bool
  description: Whether to use Neutrons Group-Based Policy
`

func (s *showSuite) TestShowWithRegionConfigAndFlags(c *tc.C) {
	data := `
clouds:
  homestack:
    type: openstack
    description: Openstack Cloud
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
    config:
      bootstrap-retry-delay: 1500
      network: nameme
    region-config:
      london:
        bootstrap-timeout: 1800
`[1:]
	err := os.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)
	c.Assert(err, tc.ErrorIsNil)
	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "homestack", "--include-config", "--client")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, strings.Join([]string{`
Client cloud "homestack":

defined: local
type: openstack
description: Openstack Cloud
auth-types: [userpass, access-key]
endpoint: http://homestack
regions:
  london:
    endpoint: http://london/1.0
config:
  bootstrap-retry-delay: 1500
  network: nameme
region-config:
  london:
    bootstrap-timeout: 1800
`[1:], openstackProviderConfig}, ""))
}

var yamlWithCert = `
clouds:
  homestack:
    type: openstack
    auth-types: [userpass]
    endpoint: https://homestack:5000/v2.0
    regions:
      RegionOne:
        endpoint: https://homestack:5000/v2.0
    ca-certificates:
    - |-
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

var resultWithCert = `
Client cloud "homestack":

defined: local
type: openstack
description: Openstack Cloud
auth-types: [userpass]
endpoint: https://homestack:5000/v2.0
regions:
  RegionOne: {}
ca-credentials:
- |-
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

func (s *showSuite) TestShowWithCACertificate(c *tc.C) {
	err := os.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(yamlWithCert), 0600)
	c.Assert(err, tc.ErrorIsNil)
	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "homestack", "--client")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, resultWithCert)
}

type fakeShowCloudAPI struct {
	jujutesting.Stub
	cloud jujucloud.Cloud
}

func (api *fakeShowCloudAPI) Close() error {
	api.AddCall("Close", nil)
	return api.NextErr()
}

func (api *fakeShowCloudAPI) Cloud(ctx context.Context, tag names.CloudTag) (jujucloud.Cloud, error) {
	api.AddCall("Cloud", tag)
	return api.cloud, api.NextErr()
}

func (api *fakeShowCloudAPI) CloudInfo(ctx context.Context, tags []names.CloudTag) ([]cloudapi.CloudInfo, error) {
	api.AddCall("CloudInfo", tags)
	return []cloudapi.CloudInfo{{
		Cloud: api.cloud,
		Users: map[string]cloudapi.CloudUserInfo{
			"fred": {
				DisplayName: "Fred",
				Access:      "admin",
			},
		},
	}}, api.NextErr()
}
