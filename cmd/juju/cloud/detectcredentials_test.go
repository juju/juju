// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type detectCredentialsSuite struct {
	testing.BaseSuite

	store             *jujuclient.MemStore
	aCredential       jujucloud.CloudCredential
	credentialAPIFunc func() (cloud.CredentialAPI, error)
	api               *fakeUpdateCredentialAPI
}

var _ = gc.Suite(&detectCredentialsSuite{})

type mockProvider struct {
	environs.CloudEnvironProvider
	detectedCreds *jujucloud.CloudCredential
	credSchemas   *map[jujucloud.AuthType]jujucloud.CredentialSchema
}

func (p *mockProvider) DetectCredentials() (*jujucloud.CloudCredential, error) {
	if len(p.detectedCreds.AuthCredentials) == 0 {
		return nil, errors.NotFoundf("credentials")
	}
	return p.detectedCreds, nil
}

func (p *mockProvider) CredentialSchemas() map[jujucloud.AuthType]jujucloud.CredentialSchema {
	if p.credSchemas == nil {
		return map[jujucloud.AuthType]jujucloud.CredentialSchema{
			jujucloud.AccessKeyAuthType: {
				{
					"access-key", jujucloud.CredentialAttr{},
				}, {
					"secret-key", jujucloud.CredentialAttr{Hidden: true},
				},
			},
			jujucloud.UserPassAuthType: {
				{
					"username", jujucloud.CredentialAttr{},
				}, {
					"password", jujucloud.CredentialAttr{Hidden: true},
				}, {
					"application-password", jujucloud.CredentialAttr{Hidden: true},
				},
			},
			jujucloud.OAuth2AuthType: {
				{
					"client-id", jujucloud.CredentialAttr{},
				}, {
					"client-email", jujucloud.CredentialAttr{},
				}, {
					"private-key", jujucloud.CredentialAttr{Hidden: true},
				}, {
					"project-id", jujucloud.CredentialAttr{},
				},
			},
		}
	}
	return *p.credSchemas
}

func (p *mockProvider) FinalizeCredential(
	ctx environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
) (*jujucloud.Credential, error) {
	if args.Credential.AuthType() == "interactive" {
		fmt.Fprintln(ctx.GetStderr(), "generating userpass credential")
		out := jujucloud.NewCredential(jujucloud.UserPassAuthType, map[string]string{
			"username":             args.Credential.Attributes()["username"],
			"password":             args.CloudEndpoint,
			"application-password": args.CloudIdentityEndpoint,
		})
		return &out, nil
	}
	return &args.Credential, nil
}

func (s *detectCredentialsSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	unreg := environs.RegisterProvider("mock-provider", &mockProvider{detectedCreds: &s.aCredential})
	s.AddCleanup(func(_ *gc.C) {
		unreg()
	})
}

func (s *detectCredentialsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.store = jujuclient.NewMemStore()
	s.aCredential = jujucloud.CloudCredential{}
	s.api = &fakeUpdateCredentialAPI{
		v:      5,
		clouds: func() (map[names.CloudTag]jujucloud.Cloud, error) { return nil, nil },
	}
	s.credentialAPIFunc = func() (cloud.CredentialAPI, error) { return s.api, nil }
}

func (s *detectCredentialsSuite) run(c *gc.C, stdin io.Reader, clouds map[string]jujucloud.Cloud, args ...string) (*cmd.Context, error) {
	allCloudsFunc := func(*cmd.Context) (map[string]jujucloud.Cloud, error) {
		return clouds, nil
	}
	cloudByNameFunc := func(cloudName string) (*jujucloud.Cloud, error) {
		if one, ok := clouds[cloudName]; ok {
			return &one, nil
		}
		return nil, errors.NotFoundf("cloud %s", cloudName)
	}
	return s.runWithCloudsFunc(c, stdin, allCloudsFunc, cloudByNameFunc, args...)
}

func (s *detectCredentialsSuite) runWithCloudsFunc(c *gc.C, stdin io.Reader,
	cloudsFunc func(*cmd.Context) (map[string]jujucloud.Cloud, error),
	cloudByNameFunc func(cloudName string) (*jujucloud.Cloud, error),
	args ...string) (*cmd.Context, error) {
	registeredProvidersFunc := func() []string {
		return []string{"mock-provider"}
	}
	command := cloud.NewDetectCredentialsCommandForTest(s.store, registeredProvidersFunc, cloudsFunc, cloudByNameFunc, s.credentialAPIFunc)
	ctx := cmdtesting.Context(c)
	ctx.Stdin = stdin
	err := cmdtesting.InitCommand(command, args)
	c.Assert(err, jc.ErrorIsNil)
	return ctx, command.Run(ctx)
}

func (s *detectCredentialsSuite) credentialWithLabel(authType jujucloud.AuthType, label string) jujucloud.Credential {
	cred := jujucloud.NewCredential(authType, nil)
	cred.Label = label
	return cred
}

type detectCredentialTestExpectations struct {
	cloudName, expectedRegion, expectedStderr, expectedWarn string
}

func (s *detectCredentialsSuite) assertDetectCredential(c *gc.C, t detectCredentialTestExpectations) {
	s.aCredential = jujucloud.CloudCredential{
		DefaultRegion: "default region",
		AuthCredentials: map[string]jujucloud.Credential{
			"test": s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential")},
	}
	clouds := map[string]jujucloud.Cloud{
		"test-cloud": {
			Type: "mock-provider",
		},
		"another-cloud": {
			Type: "another-provider",
		},
	}

	stdin := strings.NewReader(fmt.Sprintf("1\n%s\nQ\n", t.cloudName))
	ctx, err := s.run(c, stdin, clouds, "--client")
	c.Assert(err, jc.ErrorIsNil)
	if t.expectedStderr == "" {
		if t.expectedRegion != "" {
			s.aCredential.DefaultRegion = t.expectedRegion
		}
		c.Assert(s.store.Credentials["test-cloud"], jc.DeepEquals, s.aCredential)
	} else {
		c.Assert(cmdtesting.Stderr(ctx), gc.DeepEquals, t.expectedStderr)
	}
	if t.expectedWarn != "" {
		c.Assert(c.GetTestLog(), jc.Contains, t.expectedWarn)
	}
}

func (s *detectCredentialsSuite) TestDetectNewCredential(c *gc.C) {
	s.assertDetectCredential(c, detectCredentialTestExpectations{cloudName: "test-cloud"})
}

func (s *detectCredentialsSuite) TestDetectCredentialOverwrites(c *gc.C) {
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"test-cloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"test": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
			},
		},
	}
	s.assertDetectCredential(c, detectCredentialTestExpectations{cloudName: "test-cloud"})
}

func (s *detectCredentialsSuite) TestDetectCredentialKeepsExistingRegion(c *gc.C) {
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"test-cloud": {
			DefaultRegion: "west",
			AuthCredentials: map[string]jujucloud.Credential{
				"test": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
			},
		},
	}
	s.assertDetectCredential(c, detectCredentialTestExpectations{cloudName: "test-cloud", expectedRegion: "west"})
}

func (s *detectCredentialsSuite) TestDetectCredentialDefaultCloud(c *gc.C) {
	s.assertDetectCredential(c, detectCredentialTestExpectations{})
}

func (s *detectCredentialsSuite) TestDetectCredentialUnknownCloud(c *gc.C) {
	s.assertDetectCredential(c, detectCredentialTestExpectations{
		cloudName: "foo",
		expectedStderr: `

1. credential (new)
Select a credential to save by number, or type Q to quit: 
Select the cloud it belongs to, or type Q to quit [test-cloud]: 

1. credential (new)
Select a credential to save by number, or type Q to quit: 
`[1:],
		expectedWarn: "cloud foo not valid",
	})
}

func (s *detectCredentialsSuite) TestDetectCredentialInvalidCloud(c *gc.C) {
	s.assertDetectCredential(c, detectCredentialTestExpectations{
		cloudName: "another-cloud",
		expectedStderr: `

1. credential (new)
Select a credential to save by number, or type Q to quit: 
Select the cloud it belongs to, or type Q to quit [test-cloud]: 

1. credential (new)
Select a credential to save by number, or type Q to quit: 
`[1:],
		expectedWarn: `chosen credential not compatible with "another-provider" cloud`,
	})
}

func (s *detectCredentialsSuite) TestNewDetectCredentialNoneFound(c *gc.C) {
	stdin := strings.NewReader("")
	ctx, err := s.run(c, stdin, nil, "--client")
	c.Assert(err, jc.ErrorIsNil)
	output := strings.Replace(cmdtesting.Stderr(ctx), "\n", "", -1)
	c.Assert(output, gc.Matches, ".*No cloud credentials found.*")
	c.Assert(s.store.Credentials, gc.HasLen, 0)
}

func (s *detectCredentialsSuite) TestNewDetectCredentialFilter(c *gc.C) {
	s.aCredential = jujucloud.CloudCredential{
		DefaultRegion: "default region",
		AuthCredentials: map[string]jujucloud.Credential{
			"test": s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential")},
	}
	clouds := map[string]jujucloud.Cloud{
		"test-cloud": {
			Type: "mock-provider",
		},
		"another-cloud": {
			Type: "another-provider",
		},
	}

	stdin := strings.NewReader("")
	ctx, err := s.run(c, stdin, clouds, "some-provider", "--client")
	c.Assert(err, jc.ErrorIsNil)
	output := strings.Replace(cmdtesting.Stderr(ctx), "\n", "", -1)
	c.Assert(output, gc.Matches, ".*No cloud credentials found.*")
	c.Assert(s.store.Credentials, gc.HasLen, 0)
}

func (s *detectCredentialsSuite) TestDetectCredentialInvalidChoice(c *gc.C) {
	s.aCredential = jujucloud.CloudCredential{
		DefaultRegion: "detected region",
		AuthCredentials: map[string]jujucloud.Credential{
			"test":    s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential 1"),
			"another": s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential 2")},
	}

	stdin := strings.NewReader("3\nQ\n")
	ctx, err := s.run(c, stdin, nil, "--client")
	c.Assert(err, jc.ErrorIsNil)
	output := strings.Replace(cmdtesting.Stderr(ctx), "\n", "", -1)
	c.Assert(output, gc.Matches, ".*Invalid choice, enter a number between 1 and 2.*")
	c.Assert(s.store.Credentials, gc.HasLen, 0)
}

func (s *detectCredentialsSuite) TestDetectCredentialCloudMismatch(c *gc.C) {
	s.aCredential = jujucloud.CloudCredential{
		DefaultRegion: "detected region",
		AuthCredentials: map[string]jujucloud.Credential{
			"test":    s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential 1"),
			"another": s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential 2")},
	}
	clouds := map[string]jujucloud.Cloud{
		"aws": {
			Name:             "aws",
			Type:             "aws",
			AuthTypes:        []jujucloud.AuthType{jujucloud.AccessKeyAuthType},
			Endpoint:         "cloud-endpoint",
			IdentityEndpoint: "cloud-identity-endpoint",
			Regions: []jujucloud.Region{
				{Name: "default region", Endpoint: "specialendpoint", IdentityEndpoint: "specialidentityendpoint", StorageEndpoint: "storageendpoint"},
			},
		},
	}

	stdin := strings.NewReader("1\naws\nQ\n")
	ctx, err := s.run(c, stdin, clouds, "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `

1. credential 2 (new)
2. credential 1 (new)
Select a credential to save by number, or type Q to quit: 
Select the cloud it belongs to, or type Q to quit []: 

1. credential 2 (new)
2. credential 1 (new)
Select a credential to save by number, or type Q to quit: 
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, ``)
	c.Assert(s.store.Credentials, gc.HasLen, 0)
}

func (s *detectCredentialsSuite) TestDetectCredentialQuitOnCloud(c *gc.C) {
	s.aCredential = jujucloud.CloudCredential{
		DefaultRegion: "detected region",
		AuthCredentials: map[string]jujucloud.Credential{
			"test":    s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential b"),
			"another": s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential a")},
	}

	stdin := strings.NewReader("1\nQ\n")
	ctx, err := s.run(c, stdin, nil, "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `

1. credential a (new)
2. credential b (new)
Select a credential to save by number, or type Q to quit: 
Select the cloud it belongs to, or type Q to quit []: 
`[1:])
	c.Assert(s.store.Credentials, gc.HasLen, 0)
}

func (s *detectCredentialsSuite) setupStore(c *gc.C) {
	s.store.Controllers["controller"] = jujuclient.ControllerDetails{ControllerUUID: "cdcssc"}
	s.store.CurrentControllerName = "controller"
	s.store.Accounts = map[string]jujuclient.AccountDetails{
		"controller": {
			User: "admin@local",
		},
	}
}

func (s *detectCredentialsSuite) TestRemoteLoad(c *gc.C) {
	// Ensure that there is a current controller to be picked for
	// loading remotely.
	s.setupStore(c)
	cloudName := "test-cloud"
	called := false
	s.api.addCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error) {
		c.Assert(cloudCredentials, gc.HasLen, 1)
		called = true
		expectedTag := names.NewCloudCredentialTag(fmt.Sprintf("%v/admin@local/blah", cloudName)).String()
		for k := range cloudCredentials {
			c.Assert(k, gc.DeepEquals, expectedTag)
		}
		return []params.UpdateCredentialResult{{CredentialTag: expectedTag}}, nil
	}

	remoteTestCloud := jujucloud.Cloud{
		Name:             cloudName,
		Type:             "mock-provider",
		AuthTypes:        []jujucloud.AuthType{jujucloud.AccessKeyAuthType},
		Endpoint:         "cloud-endpoint",
		IdentityEndpoint: "cloud-identity-endpoint",
		Regions: []jujucloud.Region{
			{Name: "default region", Endpoint: "specialendpoint", IdentityEndpoint: "specialidentityendpoint", StorageEndpoint: "storageendpoint"},
		},
	}
	s.api.clouds = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag(cloudName): remoteTestCloud,
		}, nil
	}
	cred := jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{
		"secret-key": "topsekret",
		"access-key": "lesssekret",
	})
	cred.Label = "credential"
	s.aCredential = jujucloud.CloudCredential{
		DefaultRegion:   "default region",
		AuthCredentials: map[string]jujucloud.Credential{"blah": cred},
	}
	cloudByNameFunc := func(cloudName string) (*jujucloud.Cloud, error) {
		return &remoteTestCloud, nil
	}

	stdin := strings.NewReader(fmt.Sprintf("3\n1\n%s\nQ\n", cloudName))
	ctx, err := s.runWithCloudsFunc(c, stdin, nil, cloudByNameFunc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.DeepEquals, `
This operation can be applied to both a copy on this client and to the one on a controller.

Looking for cloud and credential information on local client...

Looking for cloud information on controller "controller"...

1. credential (new)
Select a credential to save by number, or type Q to quit: 
Select the cloud it belongs to, or type Q to quit [test-cloud]: 
Saved credential to cloud test-cloud locally

1. credential (existing, will overwrite)
Select a credential to save by number, or type Q to quit: 

Controller credential "blah" for user "admin@local" for cloud "test-cloud" on controller "controller" loaded.
For more information, see ‘juju show-credential test-cloud blah’.
`[1:])
	c.Assert(called, jc.IsTrue)
	c.Assert(cmdtesting.Stdout(ctx), gc.DeepEquals, `
Do you want to add a credential to:
    1. client only (--client)
    2. controller "controller" only (--controller controller)
    3. both (--client --controller controller)
Enter your choice, or type Q|q to quit: `[1:])
}

func (s *detectCredentialsSuite) assertAutoloadCredentials(c *gc.C, expectedStderr string, args ...string) {
	// Ensure that there is a current controller to be picked for
	// loading remotely.
	s.setupStore(c)
	cloudName := "test-cloud"
	called := false
	s.api.addCloudsCredentials = func(cloudCredentials map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error) {
		c.Assert(cloudCredentials, gc.HasLen, 1)
		called = true
		expectedTag := names.NewCloudCredentialTag(fmt.Sprintf("%v/admin@local/blah", cloudName)).String()
		for k := range cloudCredentials {
			c.Assert(k, gc.DeepEquals, expectedTag)
		}
		return []params.UpdateCredentialResult{{CredentialTag: expectedTag}}, nil
	}

	clouds := map[string]jujucloud.Cloud{
		cloudName: {
			Name:             cloudName,
			Type:             "mock-provider",
			AuthTypes:        []jujucloud.AuthType{jujucloud.AccessKeyAuthType},
			Endpoint:         "cloud-endpoint",
			IdentityEndpoint: "cloud-identity-endpoint",
			Regions: []jujucloud.Region{
				{Name: "default region", Endpoint: "specialendpoint", IdentityEndpoint: "specialidentityendpoint", StorageEndpoint: "storageendpoint"},
			},
		},
	}
	cred := jujucloud.NewCredential(jujucloud.AccessKeyAuthType, map[string]string{
		"secret-key": "topsekret",
		"access-key": "lesssekret",
	})
	cred.Label = "credential"
	s.aCredential = jujucloud.CloudCredential{
		DefaultRegion:   "default region",
		AuthCredentials: map[string]jujucloud.Credential{"blah": cred},
	}

	stdin := strings.NewReader(fmt.Sprintf("1\n%s\nQ\n", cloudName))
	ctx, err := s.run(c, stdin, clouds, args...)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cmdtesting.Stderr(ctx), gc.DeepEquals, expectedStderr)
	c.Assert(called, jc.IsFalse)
}

func (s *detectCredentialsSuite) TestRemoteLoadNoRemoteCloud(c *gc.C) {
	s.assertAutoloadCredentials(c, `

1. credential (new)
Select a credential to save by number, or type Q to quit: 
Select the cloud it belongs to, or type Q to quit [test-cloud]: 

1. credential (new)
Select a credential to save by number, or type Q to quit: 

Cloud "test-cloud" does not exist on the controller: not uploading credentials for it...
Use 'juju clouds' to view all available clouds and 'juju add-cloud' to add missing ones.
`[1:], "-c", "controller")
}

func (s *detectCredentialsSuite) TestDetectCredentialClientOnly(c *gc.C) {
	s.assertAutoloadCredentials(c, `

1. credential (new)
Select a credential to save by number, or type Q to quit: 
Select the cloud it belongs to, or type Q to quit [test-cloud]: 
Saved credential to cloud test-cloud locally

1. credential (existing, will overwrite)
Select a credential to save by number, or type Q to quit: 
`[1:],
		"--client")
}

func (s *detectCredentialsSuite) TestAddLoadedCredential(c *gc.C) {
	all := map[string]map[string]map[string]jujucloud.Credential{}
	cloud.AddLoadedCredentialForTest(all, "a", "b", "one", jujucloud.NewEmptyCredential())
	cloud.AddLoadedCredentialForTest(all, "a", "b", "two", jujucloud.NewEmptyCredential())
	cloud.AddLoadedCredentialForTest(all, "a", "c", "three", jujucloud.NewEmptyCredential())
	cloud.AddLoadedCredentialForTest(all, "d", "a", "four", jujucloud.NewEmptyCredential())
	c.Assert(all, gc.HasLen, 2)
	c.Assert(all["d"], gc.DeepEquals, map[string]map[string]jujucloud.Credential{"a": {"four": jujucloud.NewEmptyCredential()}})
	c.Assert(all["a"]["c"], gc.DeepEquals, map[string]jujucloud.Credential{"three": jujucloud.NewEmptyCredential()})
	c.Assert(all["a"]["b"], gc.HasLen, 2)
}
