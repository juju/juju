// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/testing"
)

type showSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api   *fakeShowCloudAPI
	store *jujuclient.MemStore
}

var _ = gc.Suite(&showSuite{})

func (s *showSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.api = &fakeShowCloudAPI{}
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *showSuite) TestShowBadArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand())
	c.Assert(err, gc.ErrorMatches, "no cloud specified")
}

func (s *showSuite) assertShowLocal(c *gc.C, expectedOutput string) {
	command := cloud.NewShowCloudCommandForTest(
		s.store,
		func() (cloud.ShowCloudAPI, error) {
			c.Fail()
			return s.api, nil
		})
	ctx, err := cmdtesting.RunCommand(c, command, "aws-china", "--client")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, expectedOutput)
}

func (s *showSuite) TestShowLocal(c *gc.C) {
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

func (s *showSuite) TestShowLocalWithDefaultCloud(c *gc.C) {
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

func (s *showSuite) TestShowKubernetes(c *gc.C) {
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
	}
	command := cloud.NewShowCloudCommandForTest(
		s.store,
		func() (cloud.ShowCloudAPI, error) {
			return s.api, nil
		})
	ctx, err := cmdtesting.RunCommand(c, command, "--controller", "mycontroller", "beehive")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "Cloud", "Close")
	c.Assert(command.ControllerName, gc.Equals, "mycontroller")
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `
Cloud "beehive" from controller "mycontroller":

defined: public
type: k8s
description: Bumble Bees
auth-types: [userpass]
endpoint: http://cluster
regions:
  default:
    endpoint: http://cluster/default
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

func (s *showSuite) TestShowControllerCloudNoLocal(c *gc.C) {
	s.setupRemoteCloud("beehive")
	command := cloud.NewShowCloudCommandForTest(
		s.store,
		func() (cloud.ShowCloudAPI, error) {
			return s.api, nil
		})
	ctx, err := cmdtesting.RunCommand(c, command, "beehive", "-c", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "Cloud", "Close")
	c.Assert(command.ControllerName, gc.Equals, "mycontroller")
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `
Cloud "beehive" from controller "mycontroller":

defined: public
type: openstack
description: Bumble Bees
auth-types: [userpass, access-key]
endpoint: http://myopenstack
regions:
  regionone:
    endpoint: http://boston/1.0
`[1:])
}

func (s *showSuite) TestShowControllerAndLocalCloud(c *gc.C) {
	s.setupRemoteCloud("aws-china")
	command := cloud.NewShowCloudCommandForTest(
		s.store,
		func() (cloud.ShowCloudAPI, error) {
			return s.api, nil
		})
	ctx, err := cmdtesting.RunCommand(c, command, "aws-china")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "Cloud", "Close")
	c.Assert(command.ControllerName, gc.Equals, "mycontroller")
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `
Cloud "aws-china" from controller "mycontroller":

defined: public
type: openstack
description: Bumble Bees
auth-types: [userpass, access-key]
endpoint: http://myopenstack
regions:
  regionone:
    endpoint: http://boston/1.0

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

func (s *showSuite) TestShowWithConfig(c *gc.C) {
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
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "homestack", "--client")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `

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
use-floating-ip:
  type: bool
  description: Whether a floating IP address is required to give the nodes a public
    IP address. Some installations assign public IP addresses by default without requiring
    a floating IP address.
use-openstack-gbp:
  type: bool
  description: Whether to use Neutrons Group-Based Policy
`

func (s *showSuite) TestShowWithRegionConfigAndFlags(c *gc.C) {
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
        use-floating-ip: true
`[1:]
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "homestack", "--include-config", "--client")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, strings.Join([]string{`

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
    use-floating-ip: true
`[1:], openstackProviderConfig}, ""))
}

func (s *showSuite) TestShowWithRegionConfigAndFlagNoExtraOut(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "joyent", "--include-config", "--client")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `

Client cloud "joyent":

defined: public
type: joyent
description: Joyent Cloud
auth-types: [userpass]
regions:
  us-east-1:
    endpoint: https://us-east-1.api.joyentcloud.com
  us-east-2:
    endpoint: https://us-east-2.api.joyentcloud.com
  us-east-3:
    endpoint: https://us-east-3.api.joyentcloud.com
  us-west-1:
    endpoint: https://us-west-1.api.joyentcloud.com
  us-sw-1:
    endpoint: https://us-sw-1.api.joyentcloud.com
  eu-ams-1:
    endpoint: https://eu-ams-1.api.joyentcloud.com
`[1:])
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

func (s *showSuite) TestShowWithCACertificate(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(yamlWithCert), 0600)
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "homestack", "--client")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, resultWithCert)
}

type fakeShowCloudAPI struct {
	jujutesting.Stub
	cloud jujucloud.Cloud
}

func (api *fakeShowCloudAPI) Close() error {
	api.AddCall("Close", nil)
	return api.NextErr()
}

func (api *fakeShowCloudAPI) Cloud(tag names.CloudTag) (jujucloud.Cloud, error) {
	api.AddCall("Cloud", tag)
	return api.cloud, api.NextErr()
}
