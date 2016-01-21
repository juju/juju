// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
)

type credentialsSuite struct{}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) TestMarshallAccessKey(c *gc.C) {
	creds := cloud.Credentials{
		Credentials: map[string]cloud.CloudCredential{
			"aws": {
				DefaultCredential: "default-cred",
				DefaultRegion:     "us-west-2",
				AuthCredentials: map[string]cloud.Credential{
					"peter": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
						"access-key": "key",
						"secret-key": "secret",
					}),
					// TODO(wallyworld) - add anther credential once goyaml.v2 supports inline MapSlice.
					//"paul": &cloud.AccessKeyCredentials{
					//	Key: "paulkey",
					//	Secret: "paulsecret",
					//},
				},
			},
		},
	}
	out, err := yaml.Marshal(creds)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, `
credentials:
  aws:
    default-credential: default-cred
    default-region: us-west-2
    peter:
      auth-type: access-key
      access-key: key
      secret-key: secret
`[1:])
}

func (s *credentialsSuite) TestMarshallOpenstackAccessKey(c *gc.C) {
	creds := cloud.Credentials{
		Credentials: map[string]cloud.CloudCredential{
			"openstack": {
				DefaultCredential: "default-cred",
				DefaultRegion:     "region-a",
				AuthCredentials: map[string]cloud.Credential{
					"peter": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
						"access-key":  "key",
						"secret-key":  "secret",
						"tenant-name": "tenant",
					}),
				},
			},
		},
	}
	out, err := yaml.Marshal(creds)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, `
credentials:
  openstack:
    default-credential: default-cred
    default-region: region-a
    peter:
      auth-type: access-key
      access-key: key
      secret-key: secret
      tenant-name: tenant
`[1:])
}

func (s *credentialsSuite) TestMarshallOpenstackUserPass(c *gc.C) {
	creds := cloud.Credentials{
		Credentials: map[string]cloud.CloudCredential{
			"openstack": {
				DefaultCredential: "default-cred",
				DefaultRegion:     "region-a",
				AuthCredentials: map[string]cloud.Credential{
					"peter": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
						"username":    "user",
						"password":    "secret",
						"tenant-name": "tenant",
					}),
				},
			},
		},
	}
	out, err := yaml.Marshal(creds)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, `
credentials:
  openstack:
    default-credential: default-cred
    default-region: region-a
    peter:
      auth-type: userpass
      password: secret
      tenant-name: tenant
      username: user
`[1:])
}

func (s *credentialsSuite) TestMarshallAzureCredntials(c *gc.C) {
	creds := cloud.Credentials{
		Credentials: map[string]cloud.CloudCredential{
			"azure": {
				DefaultCredential: "default-cred",
				DefaultRegion:     "Central US",
				AuthCredentials: map[string]cloud.Credential{
					"peter": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
						"application-id":       "app-id",
						"application-password": "app-secret",
						"subscription-id":      "subscription-id",
						"tenant-id":            "tenant-id",
					}),
				},
			},
		},
	}
	out, err := yaml.Marshal(creds)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, `
credentials:
  azure:
    default-credential: default-cred
    default-region: Central US
    peter:
      auth-type: userpass
      application-id: app-id
      application-password: app-secret
      subscription-id: subscription-id
      tenant-id: tenant-id
`[1:])
}

func (s *credentialsSuite) TestMarshallOAuth1(c *gc.C) {
	creds := cloud.Credentials{
		Credentials: map[string]cloud.CloudCredential{
			"maas": {
				DefaultCredential: "default-cred",
				DefaultRegion:     "region-default",
				AuthCredentials: map[string]cloud.Credential{
					"peter": cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{
						"consumer-key":    "consumer-key",
						"consumer-secret": "consumer-secret",
						"access-token":    "access-token",
						"token-secret":    "token-secret",
					}),
				},
			},
		},
	}
	out, err := yaml.Marshal(creds)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, `
credentials:
  maas:
    default-credential: default-cred
    default-region: region-default
    peter:
      auth-type: oauth1
      access-token: access-token
      consumer-key: consumer-key
      consumer-secret: consumer-secret
      token-secret: token-secret
`[1:])
}

func (s *credentialsSuite) TestMarshallOAuth2(c *gc.C) {
	creds := cloud.Credentials{
		Credentials: map[string]cloud.CloudCredential{
			"google": {
				DefaultCredential: "default-cred",
				DefaultRegion:     "West US",
				AuthCredentials: map[string]cloud.Credential{
					"peter": cloud.NewCredential(cloud.OAuth2AuthType, map[string]string{
						"client-id":    "client-id",
						"client-email": "client-email",
						"private-key":  "secret",
					}),
				},
			},
		},
	}
	out, err := yaml.Marshal(creds)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, `
credentials:
  google:
    default-credential: default-cred
    default-region: West US
    peter:
      auth-type: oauth2
      client-email: client-email
      client-id: client-id
      private-key: secret
`[1:])
}

func (s *credentialsSuite) TestParseCredentials(c *gc.C) {
	s.testParseCredentials(c, []byte(`
credentials:
  aws:
    default-credential: peter
    default-region: us-east-2
    peter:
      auth-type: access-key
      access-key: key
      secret-key: secret
  aws-china:
    default-credential: zhu8jie
    zhu8jie:
      auth-type: access-key
      access-key: key
      secret-key: secret
    sun5kong:
      auth-type: access-key
      access-key: quay
      secret-key: sekrit
  aws-gov:
    default-region: us-gov-west-1
    supersekrit:
      auth-type: access-key
      access-key: super
      secret-key: sekrit
`[1:]), &cloud.Credentials{
		Credentials: map[string]cloud.CloudCredential{
			"aws": cloud.CloudCredential{
				DefaultCredential: "peter",
				DefaultRegion:     "us-east-2",
				AuthCredentials: map[string]cloud.Credential{
					"peter": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
						"access-key": "key",
						"secret-key": "secret",
					}),
				},
			},
			"aws-china": cloud.CloudCredential{
				DefaultCredential: "zhu8jie",
				AuthCredentials: map[string]cloud.Credential{
					"zhu8jie": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
						"access-key": "key",
						"secret-key": "secret",
					}),
					"sun5kong": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
						"access-key": "quay",
						"secret-key": "sekrit",
					}),
				},
			},
			"aws-gov": cloud.CloudCredential{
				DefaultRegion: "us-gov-west-1",
				AuthCredentials: map[string]cloud.Credential{
					"supersekrit": cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
						"access-key": "super",
						"secret-key": "sekrit",
					}),
				},
			},
		},
	})
}

func (s *credentialsSuite) TestParseCredentialsUnknownAuthType(c *gc.C) {
	// Unknown auth-type is not validated by ParseCredentials.
	// Validation is deferred to ValidateCredential.
	s.testParseCredentials(c, []byte(`
credentials:
  cloud-name:
    credential-name:
      auth-type: woop
`[1:]), &cloud.Credentials{
		Credentials: map[string]cloud.CloudCredential{
			"cloud-name": cloud.CloudCredential{
				AuthCredentials: map[string]cloud.Credential{
					"credential-name": cloud.NewCredential("woop", nil),
				},
			},
		},
	})
}

func (s *credentialsSuite) testParseCredentials(c *gc.C, input []byte, expect *cloud.Credentials) {
	output, err := cloud.ParseCredentials(input)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, jc.DeepEquals, expect)
}

func (s *credentialsSuite) TestParseCredentialsMissingAuthType(c *gc.C) {
	s.testParseCredentialsError(c, []byte(`
credentials:
  cloud-name:
    credential-name:
      doesnt: really-matter
`[1:]), "credentials.cloud-name.credential-name: missing auth-type")
}

func (s *credentialsSuite) TestParseCredentialsNonStringValue(c *gc.C) {
	s.testParseCredentialsError(c, []byte(`
credentials:
  cloud-name:
    credential-name:
      non-string-value: 123
`[1:]), `credentials\.cloud-name\.credential-name\.non-string-value: expected string, got int\(123\)`)
}

func (s *credentialsSuite) testParseCredentialsError(c *gc.C, input []byte, expect string) {
	_, err := cloud.ParseCredentials(input)
	c.Assert(err, gc.ErrorMatches, expect)
}
