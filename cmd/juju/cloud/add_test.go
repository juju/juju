// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	"gopkg.in/yaml.v2"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	_ "github.com/juju/juju/internal/provider/ec2"
	_ "github.com/juju/juju/internal/provider/lxd"
	_ "github.com/juju/juju/internal/provider/maas"
	_ "github.com/juju/juju/internal/provider/manual"
	_ "github.com/juju/juju/internal/provider/openstack"
	_ "github.com/juju/juju/internal/provider/vsphere"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type addSuite struct {
	testhelpers.IsolationSuite

	store     *jujuclient.MemStore
	addCloudF func(cloud jujucloud.Cloud, force bool) error
}

func TestAddSuite(t *stdtesting.T) { tc.Run(t, &addSuite{}) }
func (s *addSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.store = jujuclient.NewMemStore()
	s.store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "mycontroller"
	s.addCloudF = func(cloud jujucloud.Cloud, force bool) error { return nil }
}

func (s *addSuite) runCommand(c *tc.C, cloudMetadataStore cloud.CloudMetadataStore, args ...string) (*cmd.Context, error) {
	command := cloud.NewAddCloudCommandForTest(cloudMetadataStore, s.store, nil)
	return cmdtesting.RunCommand(c, command, args...)
}

func newFakeCloudMetadataStore() *fakeCloudMetadataStore {
	var logger loggo.Logger
	return &fakeCloudMetadataStore{CallMocker: testhelpers.NewCallMocker(logger)}
}

type fakeCloudMetadataStore struct {
	*testhelpers.CallMocker
}

func (f *fakeCloudMetadataStore) ReadCloudData(path string) ([]byte, error) {
	results := f.MethodCall(f, "ReadCloudData", path)
	if results[0] == nil {
		return nil, testhelpers.TypeAssertError(results[1])
	}
	return []byte(results[0].(string)), testhelpers.TypeAssertError(results[1])
}

func (f *fakeCloudMetadataStore) PublicCloudMetadata(searchPaths ...string) (result map[string]jujucloud.Cloud, fallbackUsed bool, _ error) {
	results := f.MethodCall(f, "PublicCloudMetadata", searchPaths)
	return results[0].(map[string]jujucloud.Cloud), results[1].(bool), testhelpers.TypeAssertError(results[2])
}

func (f *fakeCloudMetadataStore) PersonalCloudMetadata() (map[string]jujucloud.Cloud, error) {
	results := f.MethodCall(f, "PersonalCloudMetadata")
	return results[0].(map[string]jujucloud.Cloud), testhelpers.TypeAssertError(results[1])
}

func (f *fakeCloudMetadataStore) WritePersonalCloudMetadata(cloudsMap map[string]jujucloud.Cloud) error {
	results := f.MethodCall(f, "WritePersonalCloudMetadata", cloudsMap)
	return testhelpers.TypeAssertError(results[0])
}

func (f *fakeCloudMetadataStore) ParseOneCloud(data []byte) (jujucloud.Cloud, error) {
	results := f.MethodCall(f, "ParseOneCloud", data)
	if len(results) != 2 {
		fmt.Printf("ParseOneCloud()\n(%s)\n", string(data))
		return jujucloud.Cloud{}, errors.New("ParseOneCloud failed, not enough results")
	}
	return results[0].(jujucloud.Cloud), testhelpers.TypeAssertError(results[1])
}

func (s *addSuite) TestAddBadArgs(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewAddCloudCommand(nil), "cloud", "cloud.yaml", "extra")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

var (
	homeStackYamlFile = `
        clouds:
          homestack:
            type: openstack
            auth-types: [access-key]
            endpoint: "http://homestack"
            regions:
              london:
                endpoint: "http://london/1.0"`

	homestackCloud = jujucloud.Cloud{
		Name:      "homestack",
		Type:      "openstack",
		AuthTypes: []jujucloud.AuthType{"userpass", "access-key"},
		Endpoint:  "http://homestack",
		Regions: []jujucloud.Region{
			{
				Name:     "london",
				Endpoint: "http://london/1.0",
			},
		},
	}

	localhostYamlFile = `
        clouds:
          localhost:
            type: lxd`

	awsYamlFile = `
        clouds:
          aws:
            type: ec2
            auth-types: [access-key]
            regions:
              us-east-1:
                endpoint: "https://us-east-1.aws.amazon.com/v1.2/"`

	garageMaasYamlFile = `
        clouds:
          garage-maas:
            type: maas
            auth-types: [oauth1]
            endpoint: "http://garagemaas"
            skip-tls-verify: true`

	garageMaasYamlFileListCloudOutput = `
        garage-maas:
          type: maas
          auth-types: [oauth1]
          endpoint: "http://garagemaas"
          skip-tls-verify: true`

	manyCloudsYamlFile = `
        clouds:
          garage-maas:
            type: maas
            auth-types: [oauth1]
            endpoint: "http://garagemaas"
          home-garage-maas:
            type: maas
            auth-types: [oauth1]
            endpoint: "http://garagemaas"`

	garageMAASCloud = jujucloud.Cloud{
		Name:          "garage-maas",
		Type:          "maas",
		AuthTypes:     []jujucloud.AuthType{"oauth1"},
		Endpoint:      "http://garagemaas",
		SkipTLSVerify: true,
	}

	manualCloud = jujucloud.Cloud{
		Name:      "manual",
		Type:      "manual",
		AuthTypes: []jujucloud.AuthType{"manual"},
		Endpoint:  "192.168.1.6",
	}
)

func homestackMetadata() map[string]jujucloud.Cloud {
	return map[string]jujucloud.Cloud{"homestack": homestackCloud}
}

func (*addSuite) TestAddBadFilename(c *tc.C) {
	fake := newFakeCloudMetadataStore()
	badFileErr := errors.New("")
	fake.Call("ReadCloudData", "somefile.yaml").Returns(nil, badFileErr)

	addCmd := cloud.NewAddCloudCommand(fake)
	_, err := cmdtesting.RunCommand(c, addCmd, "cloud", "somefile.yaml", "--client")
	c.Check(errors.Cause(err), tc.Equals, badFileErr)
}

func (s *addSuite) TestAddBadCloudName(c *tc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "testFile").Returns(homeStackYamlFile, nil)

	_, err := s.runCommand(c, fake, "cloud", "testFile", "--client")
	c.Assert(err, tc.ErrorMatches, `cloud "cloud" not found in file .*`)
}

func (s *addSuite) TestAddInvalidCloudName(c *tc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "testFile").Returns(homeStackYamlFile, nil)

	_, err := s.runCommand(c, fake, "bad^cloud", "testFile")
	c.Assert(err, tc.ErrorMatches, `cloud name "bad\^cloud" not valid`)
}

func (s *addSuite) TestAddExisting(c *tc.C) {
	fake := newFakeCloudMetadataStore()

	clouds, err := jujucloud.ParseCloudMetadata([]byte(homeStackYamlFile))
	c.Assert(err, tc.ErrorIsNil)
	fake.Call("ReadCloudData", "mycloud.yaml").Returns(homeStackYamlFile, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(clouds, nil)

	_, err = s.runCommand(c, fake, "homestack", "mycloud.yaml", "--client")
	c.Assert(err, tc.ErrorMatches, "use `update-cloud homestack --client` to override known definition: local cloud \"homestack\" already exists")
}

func (s *addSuite) TestAddExistingPublic(c *tc.C) {
	clouds, err := jujucloud.ParseCloudMetadata([]byte(awsYamlFile))
	c.Assert(err, tc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "mycloud.yaml").Returns(awsYamlFile, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(clouds, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)

	_, err = s.runCommand(c, fake, "aws", "mycloud.yaml", "--client")
	c.Assert(err, tc.ErrorMatches, "use `update-cloud aws --client` to override known definition: local cloud \"aws\" already exists")
}

func (s *addSuite) TestAddExistingBuiltin(c *tc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "mycloud.yaml").Returns(localhostYamlFile, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)

	_, err := s.runCommand(c, fake, "localhost", "mycloud.yaml", "--client")
	c.Assert(err, tc.ErrorMatches, "use `update-cloud localhost --client` to override known definition: local cloud \"localhost\" already exists")
}

func addDefaultRegion(in map[string]jujucloud.Cloud) map[string]jujucloud.Cloud {
	for k, v := range in {
		if len(v.Regions) == 0 {
			v.Regions = []jujucloud.Region{{Name: "default"}}
			in[k] = v
		}
	}
	return in
}

func (s *addSuite) TestAddNew(c *tc.C) {
	clouds, err := jujucloud.ParseCloudMetadata([]byte(garageMaasYamlFile))
	c.Assert(err, tc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "mycloud.yaml").Returns(garageMaasYamlFile, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)

	// but here mockCloud should have a region attached...
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", addDefaultRegion(clouds)).Returns(nil)

	_, err = s.runCommand(c, fake, "garage-maas", "mycloud.yaml", "--client")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(numCallsToWrite(), tc.Equals, 1)
}

func (s *addSuite) TestAddLocalDefault(c *tc.C) {
	s.store.Controllers = nil
	clouds, err := jujucloud.ParseCloudMetadata([]byte(garageMaasYamlFile))
	c.Assert(err, tc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "mycloud.yaml").Returns(garageMaasYamlFile, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", addDefaultRegion(clouds)).Returns(nil)

	ctx, err := s.runCommand(c, fake, "garage-maas", "mycloud.yaml", "--client")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(numCallsToWrite(), tc.Equals, 1)
	c.Assert(cmdtesting.Stderr(ctx), tc.DeepEquals, "Cloud \"garage-maas\" successfully added to your local client.\n"+
		"You will need to add a credential for this cloud (`juju add-credential garage-maas`)\n"+
		"before you can use it to bootstrap a controller (`juju bootstrap garage-maas`) or\n"+
		"to create a model (`juju add-model <your model name> garage-maas`).\n")
}

func (s *addSuite) TestAddNewInvalidAuthType(c *tc.C) {
	fake := newFakeCloudMetadataStore()
	fakeCloudYamlFile := `
        clouds:
          fakecloud:
            type: maas
            auth-types: [oauth1, user-pass]
            endpoint: "http://garagemaas"`

	fake.Call("ReadCloudData", "mycloud.yaml").Returns(fakeCloudYamlFile, nil)

	_, err := s.runCommand(c, fake, "fakecloud", "mycloud.yaml", "--client")
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`auth type "user-pass" not supported`))
}

type fakeAddCloudAPI struct {
	testhelpers.Stub
	addCloudF func(cloud jujucloud.Cloud, force bool) error
}

func (api *fakeAddCloudAPI) Close() error {
	api.AddCall("Close", nil)
	return nil
}

func (api *fakeAddCloudAPI) AddCloud(ctx context.Context, cloud jujucloud.Cloud, force bool) error {
	api.AddCall("AddCloud", cloud, force)
	return api.addCloudF(cloud, force)
}

func (api *fakeAddCloudAPI) AddCredential(ctx context.Context, tag string, credential jujucloud.Credential) error {
	api.AddCall("AddCredential", tag, credential)
	return nil
}

func (s *addSuite) setupControllerCloudScenarioWithClientAndFile(c *tc.C,
	clientF func(ctx context.Context) (cloud.AddCloudAPI, error),
	api *fakeAddCloudAPI,
	cloudsFileYaml string) (
	string, *cloud.AddCloudCommand, *jujuclient.MemStore, *fakeAddCloudAPI, jujucloud.Credential, func() int,
) {
	clouds, err := jujucloud.ParseCloudMetadata([]byte(cloudsFileYaml))
	c.Assert(err, tc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "mycloud.yaml").Returns(cloudsFileYaml, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	callCounter := fake.Call("WritePersonalCloudMetadata", addDefaultRegion(clouds)).Returns(nil)

	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	store.Accounts["mycontroller"] = jujuclient.AccountDetails{User: "fred"}
	cred := jujucloud.NewCredential(jujucloud.OAuth1AuthType, map[string]string{
		"maas-oauth": "auth:token",
	})
	store.Credentials["garage-maas"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{"default": cred},
	}
	command := cloud.NewAddCloudCommandForTest(fake, store, clientF)
	return "mycloud.yaml", command, store, api, cred, callCounter
}

func (s *addSuite) setupControllerCloudScenarioWithClient(c *tc.C,
	clientF func(ctx context.Context) (cloud.AddCloudAPI, error),
	api *fakeAddCloudAPI) (string, *cloud.AddCloudCommand, *jujuclient.MemStore, *fakeAddCloudAPI, jujucloud.Credential, func() int) {
	return s.setupControllerCloudScenarioWithClientAndFile(c, clientF, api, garageMaasYamlFile)
}

func (s *addSuite) setupControllerCloudScenario(c *tc.C) (string, *cloud.AddCloudCommand, *jujuclient.MemStore, *fakeAddCloudAPI, jujucloud.Credential, func() int) {
	return s.setupControllerCloudScenarioWithFile(c, garageMaasYamlFile)
}

func (s *addSuite) setupControllerCloudScenarioWithFile(c *tc.C, cloudsFile string) (string, *cloud.AddCloudCommand, *jujuclient.MemStore, *fakeAddCloudAPI, jujucloud.Credential, func() int) {
	api := &fakeAddCloudAPI{
		Stub:      testhelpers.Stub{},
		addCloudF: s.addCloudF,
	}
	return s.setupControllerCloudScenarioWithClientAndFile(c,
		func(ctx context.Context) (cloud.AddCloudAPI, error) {
			return api, nil
		},
		api,
		cloudsFile)
}

func (s *addSuite) asssertAddToController(c *tc.C, force bool) {
	cloudFileName, command, _, api, cred, _ := s.setupControllerCloudScenario(c)
	args := []string{"garage-maas", cloudFileName, "-c", "mycontroller"}
	if force {
		args = append(args, "--force")
	}
	ctx, err := cmdtesting.RunCommand(c, command, args...)
	c.Assert(err, tc.ErrorIsNil)
	api.CheckCallNames(c, "AddCloud", "AddCredential", "Close")
	api.CheckCall(c, 0, "AddCloud",
		jujucloud.Cloud{
			Name:          "garage-maas",
			Type:          "maas",
			Description:   "Metal As A Service",
			AuthTypes:     jujucloud.AuthTypes{"oauth1"},
			Endpoint:      "http://garagemaas",
			Regions:       []jujucloud.Region{{Name: "default"}},
			SkipTLSVerify: true,
		},
		force)
	api.CheckCall(c, 1, "AddCredential", "cloudcred-garage-maas_fred_default", cred)
	c.Assert(cmdtesting.Stderr(ctx), tc.DeepEquals, "Cloud \"garage-maas\" added to controller \"mycontroller\".\n"+
		"Credential for cloud \"garage-maas\" added to controller \"mycontroller\".\n")

}

func (s *addSuite) TestAddToController(c *tc.C) {
	s.asssertAddToController(c, false)
}

func (s *addSuite) TestAddToControllerIncompatibleCloud(c *tc.C) {
	s.addCloudF = func(cloud jujucloud.Cloud, force bool) error {
		return params.Error{Code: params.CodeIncompatibleClouds}
	}
	cloudFileName, command, _, api, _, _ := s.setupControllerCloudScenario(c)
	ctx, err := cmdtesting.RunCommand(c, command, "garage-maas", cloudFileName, "-c", "mycontroller")
	c.Assert(err, tc.ErrorIsNil)
	api.CheckCallNames(c, "AddCloud", "Close")
	api.CheckCall(c, 0, "AddCloud",
		jujucloud.Cloud{
			Name:          "garage-maas",
			Type:          "maas",
			Description:   "Metal As A Service",
			AuthTypes:     jujucloud.AuthTypes{"oauth1"},
			Endpoint:      "http://garagemaas",
			Regions:       []jujucloud.Region{{Name: "default"}},
			SkipTLSVerify: true,
		},
		false)
	out := cmdtesting.Stderr(ctx)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, tc.Contains, `Adding a cloud of type "maas" might not function correctly on this controller.If you really want to do this, use --force.`)
}

func (s *addSuite) TestForceAddToController(c *tc.C) {
	s.asssertAddToController(c, true)
}

func (s *addSuite) TestAddLocal(c *tc.C) {
	cloudFileName, command, _, api, _, numCalls := s.setupControllerCloudScenario(c)

	_, err := cmdtesting.RunCommand(
		c, command, "garage-maas", cloudFileName, "--client")
	c.Assert(err, tc.ErrorIsNil)
	api.CheckNoCalls(c)

	c.Check(numCalls(), tc.Equals, 1)
}

func (s *addSuite) TestAddLocalNoCloudName(c *tc.C) {
	cloudFileName, command, _, api, _, numCalls := s.setupControllerCloudScenario(c)
	ctx, err := cmdtesting.RunCommand(c, command, "-f", cloudFileName, "--client")
	c.Assert(err, tc.ErrorIsNil)
	api.CheckNoCalls(c)
	c.Check(numCalls(), tc.Equals, 1)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Cloud \"garage-maas\" successfully added to your local client.\n"+
		"You will need to add a credential for this cloud (`juju add-credential garage-maas`)\n"+
		"before you can use it to bootstrap a controller (`juju bootstrap garage-maas`) or\n"+
		"to create a model (`juju add-model <your model name> garage-maas`).\n")
}

func (s *addSuite) TestAddLocalNoCloudNameButManyCloudsInFile(c *tc.C) {
	cloudFileName, command, _, api, _, numCalls := s.setupControllerCloudScenarioWithFile(c, manyCloudsYamlFile)
	ctx, err := cmdtesting.RunCommand(c, command, "-f", cloudFileName, "--client")
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Contains, "there is more than one cloud defined in file")
	api.CheckNoCalls(c)
	c.Check(numCalls(), tc.Equals, 0)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (s *addSuite) TestAddToControllerBadController(c *tc.C) {
	cloudFileName, command, store, _, _, _ := s.setupControllerCloudScenarioWithClient(c, func(ctx context.Context) (cloud.AddCloudAPI, error) {
		return nil, errors.NotFoundf("controller badcontroller")
	}, nil)
	store.Credentials = nil

	ctx, err := cmdtesting.RunCommand(c, command, "garage-maas", cloudFileName, "-c", "badcontroller")
	c.Assert(err, tc.Equals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Could not upload cloud to a controller: controller badcontroller not found\n")
}

func (s *addSuite) TestAddToControllerMissingCredential(c *tc.C) {
	cloudFileName, command, store, _, _, _ := s.setupControllerCloudScenario(c)
	store.Credentials = nil

	ctx, err := cmdtesting.RunCommand(c, command, "garage-maas", cloudFileName, "-c", "mycontroller")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Contains, `
Cloud "garage-maas" added to controller "mycontroller".
To upload a credential to the controller for cloud "garage-maas", use 
* 'add-model' with --credential option or
* 'add-credential -c garage-maas'.
`[1:])
	//c.Assert(c.GetTestLog(), tc.Contains, `loading credentials: credentials for cloud garage-maas not found`)
}

func (s *addSuite) TestAddToControllerAmbiguousCredential(c *tc.C) {
	cloudFileName, command, store, _, cred, _ := s.setupControllerCloudScenario(c)
	store.Credentials["garage-maas"].AuthCredentials["another"] = cred

	ctx, err := cmdtesting.RunCommand(c, command, "garage-maas", cloudFileName, "-c", "mycontroller")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Cloud \"garage-maas\" added to controller \"mycontroller\".\n"+
		"To upload a credential to the controller for cloud \"garage-maas\", use \n"+
		"* 'add-model' with --credential option or\n"+
		"* 'add-credential -c garage-maas'.\n")
	//c.Assert(c.GetTestLog(), tc.Contains, `more than one credential is available`)
}

func (*addSuite) TestInteractive(c *tc.C) {
	command := cloud.NewAddCloudCommand(nil)
	err := cmdtesting.InitCommand(command, []string{"--client"})
	c.Assert(err, tc.ErrorIsNil)

	out := &bytes.Buffer{}

	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  &bytes.Buffer{},
	}
	err = command.Run(ctx)
	c.Check(errors.Cause(err), tc.Equals, io.EOF)

	c.Assert(out.String(), tc.Equals, ""+
		"Cloud Types\n"+
		"  lxd\n"+
		"  maas\n"+
		"  manual\n"+
		"  openstack\n"+
		"  vsphere\n"+
		"\n"+
		"Select cloud type: \n",
	)
}

func (*addSuite) TestInteractiveMaas(c *tc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	const expectedYAMLarg = "" +
		"auth-types:\n" +
		"- oauth1\n" +
		"endpoint: http://mymaas\n"
	fake.Call("ParseOneCloud", []byte(expectedYAMLarg)).Returns(garageMAASCloud, nil)
	m1Cloud := garageMAASCloud
	m1Cloud.Name = "m1"
	m1Metadata := map[string]jujucloud.Cloud{"m1": m1Cloud}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", addDefaultRegion(m1Metadata)).Returns(nil)

	command := cloud.NewAddCloudCommandForTest(fake, jujuclient.NewMemStore(), nil)
	err := cmdtesting.InitCommand(command, []string{"--client"})
	c.Assert(err, tc.ErrorIsNil)

	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Stdout: io.Discard,
		Stderr: out,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "maas\n" +
			/* Enter a name for the cloud: */ "m1\n" +
			/* Enter the controller's hostname or IP address: */ "http://mymaas\n",
		),
	}

	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(numCallsToWrite(), tc.Equals, 1)
	c.Assert(out.String(), tc.Equals, "Cloud \"m1\" successfully added to your local client.\n"+
		"You will need to add a credential for this cloud (`juju add-credential m1`)\n"+
		"before you can use it to bootstrap a controller (`juju bootstrap m1`) or\n"+
		"to create a model (`juju add-model <your model name> m1`).\n")
}

func (*addSuite) TestInteractiveManual(c *tc.C) {
	manCloud := jujucloud.Cloud{
		Name:     "manual",
		Type:     "manual",
		Endpoint: "192.168.1.6",
	}
	manCloud.Name = "man"
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	fake.Call("ParseOneCloud", []byte("endpoint: 192.168.1.6\n")).Returns(manCloud, nil)
	manMetadata := map[string]jujucloud.Cloud{"man": manCloud}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", addDefaultRegion(manMetadata)).Returns(nil)

	command := cloud.NewAddCloudCommandForTest(fake, jujuclient.NewMemStore(), nil)
	err := cmdtesting.InitCommand(command, []string{"--client"})
	c.Assert(err, tc.ErrorIsNil)

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	ctx := &cmd.Context{
		Stdout: out,
		Stderr: errOut,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "manual\n" +
			/* Enter a name for the cloud: */ "man\n" +
			/* Enter the controller's hostname or IP address: */ "192.168.1.6\n",
		),
	}

	err = command.Run(ctx)
	c.Check(err, tc.ErrorIsNil)

	c.Check(numCallsToWrite(), tc.Equals, 1)
	c.Assert(out.String(), tc.Equals, `
Cloud Types
  lxd
  maas
  manual
  openstack
  vsphere

Select cloud type: 
Enter a name for your manual cloud: 
Enter the ssh connection string for controller, username@<hostname or IP> or <hostname or IP>: 
`[1:])
	c.Assert(errOut.String(), tc.Equals, "Cloud \"man\" successfully added to your local client.\n")
}

func (*addSuite) TestInteractiveManualInvalidName(c *tc.C) {
	manCloud := manualCloud
	manCloud.Name = "invalid/123"
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	fake.Call("ParseOneCloud", []byte("endpoint: 192.168.1.6\n")).Returns(manCloud, nil)
	manMetadata := map[string]jujucloud.Cloud{"man": manCloud}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", addDefaultRegion(manMetadata)).Returns(nil)

	command := cloud.NewAddCloudCommandForTest(fake, jujuclient.NewMemStore(), nil)
	err := cmdtesting.InitCommand(command, []string{"--client"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := &cmd.Context{
		Stdout: io.Discard,
		Stderr: io.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "manual\n" +
			/* Enter a name for the cloud: */ manCloud.Name + "\n",
		),
	}

	err = command.Run(ctx)
	c.Check(err, tc.NotNil)
	c.Check(numCallsToWrite(), tc.Equals, 0)
}

func (*addSuite) TestInteractiveVSphere(c *tc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	vsphereCloud := jujucloud.Cloud{
		Name:      "mvs",
		Type:      "vsphere",
		AuthTypes: []jujucloud.AuthType{"userpass", "access-key"},
		Endpoint:  "192.168.1.6",
		Regions: []jujucloud.Region{
			{
				Name:     "foo",
				Endpoint: "192.168.1.6",
			},
			{
				Name:     "bar",
				Endpoint: "192.168.1.6",
			},
		},
	}
	const expectedYAMLarg = "" +
		"auth-types:\n" +
		"- userpass\n" +
		"endpoint: 192.168.1.6\n" +
		"regions:\n" +
		"  bar: {}\n" +
		"  foo: {}\n"
	fake.Call("ParseOneCloud", []byte(expectedYAMLarg)).Returns(vsphereCloud, nil)
	vsphereMetadata := map[string]jujucloud.Cloud{"mvs": vsphereCloud}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", addDefaultRegion(vsphereMetadata)).Returns(nil)

	command := cloud.NewAddCloudCommandForTest(fake, jujuclient.NewMemStore(), nil)
	err := cmdtesting.InitCommand(command, []string{"--client"})
	c.Assert(err, tc.ErrorIsNil)

	var stdout bytes.Buffer
	ctx := &cmd.Context{
		Stdout: &stdout,
		Stderr: io.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "vsphere\n" +
			/* Enter a name for the cloud: */ "mvs\n" +
			/* Enter the vCenter address or URL: */ "192.168.1.6\n" +
			/* Enter datacenter name: */ "foo\n" +
			/* Enter another datacenter? (y/N): */ "y\n" +
			/* Enter datacenter name: */ "bar\n" +
			/* Enter another datacenter? (y/N): */ "n\n",
		),
	}

	err = command.Run(ctx)
	c.Check(err, tc.ErrorIsNil)

	c.Check(numCallsToWrite(), tc.Equals, 1)
	c.Check(stdout.String(), tc.Matches, "(.|\n)*"+`
Select cloud type: 
Enter a name for your vsphere cloud: 
Enter the vCenter address or URL: 
Enter datacenter name: 
Enter another datacenter\? \(y/N\): 
Enter datacenter name: 
Enter another datacenter\? \(y/N\): 
`[1:]+"(.|\n)*")
}

func (*addSuite) TestInteractiveExistingNameOverride(c *tc.C) {
	manualCloud := manualCloud
	manualCloud.Name = "homestack"

	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(homestackMetadata(), nil)
	manMetadata := map[string]jujucloud.Cloud{"homestack": manualCloud}
	fake.Call("ParseOneCloud", []byte("endpoint: 192.168.1.6\n")).Returns(manualCloud, nil)
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", addDefaultRegion(manMetadata)).Returns(nil)

	command := cloud.NewAddCloudCommandForTest(fake, jujuclient.NewMemStore(), nil)
	err := cmdtesting.InitCommand(command, []string{"--client"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := &cmd.Context{
		Stdout: io.Discard,
		Stderr: io.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "manual\n" +
			/* Enter a name for the cloud: */ "homestack\n" +
			/* Do you want to replace that definition? */ "y\n" +
			/* Enter the controller's hostname or IP address: */ "192.168.1.6\n",
		),
	}

	err = command.Run(ctx)
	c.Check(err, tc.ErrorIsNil)

	c.Check(numCallsToWrite(), tc.Equals, 1)
}

func (*addSuite) TestInteractiveExistingNameNoOverride(c *tc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(homestackMetadata(), nil)
	homestack2Cloud := jujucloud.Cloud{
		Name:     "homestack2",
		Type:     "manual",
		Endpoint: "192.168.1.6",
	}
	fake.Call("ParseOneCloud", []byte("endpoint: 192.168.1.6\n")).Returns(homestack2Cloud, nil)
	compoundCloudMetadata := map[string]jujucloud.Cloud{
		"homestack":  homestackCloud,
		"homestack2": homestack2Cloud,
	}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", addDefaultRegion(compoundCloudMetadata)).Returns(nil)

	command := cloud.NewAddCloudCommandForTest(fake, jujuclient.NewMemStore(), nil)
	err := cmdtesting.InitCommand(command, []string{"--client"})
	c.Assert(err, tc.ErrorIsNil)

	var out bytes.Buffer
	ctx := &cmd.Context{
		Stdout: &out,
		Stderr: io.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "manual\n" +
			/* Enter a name for the cloud: */ "homestack" + "\n" +
			/* Do you want to replace that definition? (y/N): */ "n\n" +
			/* Enter a name for the cloud: */ "homestack2" + "\n" +
			/* Enter the controller's hostname or IP address: */ "192.168.1.6" + "\n",
		),
	}

	err = command.Run(ctx)
	c.Log(out.String())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(numCallsToWrite(), tc.Equals, 1)
	c.Check(out.String(), tc.Matches, regexp.QuoteMeta("Cloud Types\n"+
		"  lxd\n"+
		"  maas\n"+
		"  manual\n"+
		"  openstack\n"+
		"  vsphere\n\n"+
		"Select cloud type: \n"+
		"Enter a name for your manual cloud: \n"+
		"A cloud named \"homestack\" already exists. Do you want to replace that definition? (y/N): \n"+
		"Enter a name for your manual cloud: \n"+
		"Enter the ssh connection string for controller, username@<hostname or IP> or <hostname or IP>: \n"))
}

func (s *addSuite) TestInteractiveAddCloud_PromptForNameIsCorrect(c *tc.C) {
	var out bytes.Buffer
	ctx := &cmd.Context{
		Stdout: &out,
		Stderr: io.Discard,
		Stdin: strings.NewReader("" +
			/* Select cloud type: */ "manual\n",
		),
	}

	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(homestackMetadata(), nil)

	command := cloud.NewAddCloudCommandForTest(fake, s.store, nil)
	err := cmdtesting.InitCommand(command, []string{"--client"})
	c.Assert(err, tc.ErrorIsNil)
	// Running the command will return an error because we only give
	// enough input to get to the prompt we care about checking. This
	// test ignores this error.
	err = command.Run(ctx)
	c.Assert(errors.Cause(err), tc.Equals, io.EOF)

	c.Check(out.String(), tc.Matches, "(?s).+Enter a name for your manual cloud: .*")
}

func (s *addSuite) TestSpecifyingjujucloudThroughFlag_CorrectlySetsMemberVar(c *tc.C) {
	runCmd := func() {
		s.runCommand(c, nil, "garage-maas", "-f", "fake.yaml", "--client")
	}
	c.Assert(runCmd, tc.PanicMatches, "runtime error: invalid memory address or nil pointer dereference")
}

func (s *addSuite) TestSpecifyingjujucloudThroughFlagAndArgument_Errors(c *tc.C) {
	_, err := s.runCommand(c, nil, "garage-maas", "-f", "fake.yaml", "foo.yaml")
	c.Check(err, tc.ErrorMatches, "cannot specify cloud file with option and argument")
}

func (s *addSuite) TestSpecifyingTargetControllerFlag(c *tc.C) {
	cloudFileName, command, _, _, _, _ := s.setupControllerCloudScenario(c)

	_, err := cmdtesting.RunCommand(
		c, command, "garage-maas", cloudFileName, "--target-controller=mycontroller-1")
	c.Assert(err, tc.ErrorIs, cmd.ErrCommandMissing)
}

func (s *addSuite) TestValidateGoodCloudFile(c *tc.C) {
	data := `
clouds:
  foundations:
    type: maas
    auth-types: [oauth1]
    endpoint: "http://10.245.31.100/MAAS"`

	var logWriter loggo.TestWriter
	writerName := "add_cloud_tests_writer"
	c.Assert(loggo.RegisterWriter(writerName, &logWriter), tc.ErrorIsNil)
	defer func() {
		loggo.RemoveWriter(writerName)
		logWriter.Clear()
	}()

	mockCloud, err := jujucloud.ParseCloudMetadata([]byte(data))
	c.Assert(err, tc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "mycloud.yaml").Returns(data, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("WritePersonalCloudMetadata", addDefaultRegion(mockCloud)).Returns(nil)

	_, err = s.runCommand(c, fake, "foundations", "mycloud.yaml", "--client")
	c.Check(err, tc.ErrorIsNil)
}

func (s *addSuite) TestValidateBadCloud(c *tc.C) {
	data := `
clouds:
  foundations:
    type: maas
    auth-typs: [oauth1]
    endpoint: "http://10.245.31.100/MAAS"`

	clouds, err := jujucloud.ParseCloudMetadata([]byte(data))
	c.Assert(err, tc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "mycloud.yaml").Returns(data, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("WritePersonalCloudMetadata", addDefaultRegion(clouds)).Returns(nil)

	var logWriter loggo.TestWriter
	writerName := "add_cloud_tests_writer"
	c.Assert(loggo.RegisterWriter(writerName, &logWriter), tc.ErrorIsNil)
	defer func() {
		loggo.RemoveWriter(writerName)
		logWriter.Clear()
	}()

	_, err = s.runCommand(c, fake, "foundations", "mycloud.yaml", "--client")
	c.Check(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_.Message`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_._`, tc.Ignore)
	c.Check(logWriter.Log(), tc.OrderedRight[[]loggo.Entry](mc), []loggo.Entry{{
		Level:   loggo.WARNING,
		Message: "\nproperty \"auth-typs\" is invalid. Perhaps you mean \"auth-types\".",
	}})
}

func (*addSuite) TestInteractiveOpenstackNoCloudCert(c *tc.C) {
	myOpenstack := jujucloud.Cloud{
		Name:      "os1",
		Type:      "openstack",
		AuthTypes: []jujucloud.AuthType{"userpass", "access-key"},
		Endpoint:  "http://myopenstack",
		Regions: []jujucloud.Region{
			{
				Name:     "regionone",
				Endpoint: "http://boston/1.0",
			},
		},
	}

	var expectedYAMLarg = "" +
		"auth-types:\n" +
		"- userpass\n" +
		"- access-key\n" +
		"certfilename: \"\"\n" +
		"endpoint: http://myopenstack\n" +
		"regions:\n" +
		"  regionone:\n" +
		"    endpoint: http://boston/1.0\n"

	var input = "" +
		/* Select cloud type: */ "openstack\n" +
		/* Enter a name for your openstack cloud: */ "os1\n" +
		/* Enter the API endpoint url for the cloud []: */ "http://myopenstack\n" +
		/* Enter ta path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "\n" +
		/* Select one or more auth types separated by commas: */ "userpass,access-key\n" +
		/* Enter region name: */ "regionone\n" +
		/* Enter the API endpoint url for the region [use cloud api url]: */ "http://boston/1.0\n" +
		/* Enter another region? (Y/n): */ "n\n"

	testInteractiveOpenstack(c, myOpenstack, expectedYAMLarg, input, "", "")
}

// Note: The first %s is filled with a string containing a newline
var expectedCloudYAMLarg = `
auth-types:
- userpass
- access-key
%scertfilename: %s
endpoint: http://myopenstack
regions:
  regionone:
    endpoint: ""
`[1:]

func (*addSuite) TestInteractiveOpenstackCloudCertFail(c *tc.C) {
	fakeCertDir := c.MkDir()
	fakeCertFilename := path.Join(fakeCertDir, "cloudcert.crt")

	invalidCertFilename := path.Join(fakeCertDir, "invalid.crt")
	os.WriteFile(invalidCertFilename, []byte("testing certification validation"), 0666)

	input := fmt.Sprintf(""+
		/* Select cloud type: */ "openstack\n"+
		/* Enter a name for your openstack cloud: */ "os1\n"+
		/* Enter the API endpoint url for the cloud []: */ "http://myopenstack\n"+
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "%s\n"+
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "%s\n"+
		/* Select one or more auth types separated by commas: */ "userpass,access-key\n"+
		/* Enter region name: */ "regionone\n"+
		/* Enter the API endpoint url for the region [use cloud api url]: */ "\n"+
		/* Enter another region? (Y/n): */ "n\n", invalidCertFilename, fakeCertFilename)

	testInteractiveOpenstackCloudCert(c, fakeCertFilename, input,
		fmt.Sprintf("Successfully read CA Certificate from %s\n", fakeCertFilename),
		fmt.Sprintf("Can't validate CA Certificate %s: no certificates found", invalidCertFilename))
}

func (*addSuite) TestInteractiveOpenstackCloudCertReadFailRetry(c *tc.C) {
	var invalidCertFilename = "/tmp/no-such-file"
	fakeCertDir := c.MkDir()
	fakeCertFilename := path.Join(fakeCertDir, "cloudcert.crt")

	input := fmt.Sprintf(""+
		/* Select cloud type: */ "openstack\n"+
		/* Enter a name for your openstack cloud: */ "os1\n"+
		/* Enter the API endpoint url for the cloud []: */ "http://myopenstack\n"+
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "%s\n"+
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "%s\n"+
		/* Select one or more auth types separated by commas: */ "userpass,access-key\n"+
		/* Enter region name: */ "regionone\n"+
		/* Enter the API endpoint url for the region [use cloud api url]: */ "\n"+
		/* Enter another region? (Y/n): */ "n\n", invalidCertFilename, fakeCertFilename)

	testInteractiveOpenstackCloudCert(c,
		fakeCertFilename,
		input,
		fmt.Sprintf("Successfully read CA Certificate from %s\n", fakeCertFilename),
		fmt.Sprintf("Can't validate CA Certificate file: open %s:", invalidCertFilename),
	)
}

func (*addSuite) TestInteractiveOpenstackCloudCert(c *tc.C) {
	fakeCertFilename := path.Join(c.MkDir(), "cloudcert.crt")

	input := fmt.Sprintf(""+
		/* Select cloud type: */ "openstack\n"+
		/* Enter a name for your openstack cloud: */ "os1\n"+
		/* Enter the API endpoint url for the cloud []: */ "http://myopenstack\n"+
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [none] */ "%s\n"+
		/* Select one or more auth types separated by commas: */ "userpass,access-key\n"+
		/* Enter region name: */ "regionone\n"+
		/* Enter the API endpoint url for the region [use cloud api url]: */ "\n"+
		/* Enter another region? (Y/n): */ "n\n", fakeCertFilename)

	testInteractiveOpenstackCloudCert(c, fakeCertFilename, input,
		fmt.Sprintf("Successfully read CA Certificate from %s\n", fakeCertFilename), "")
}

type addOpenStackSuite struct {
	testhelpers.IsolationSuite
}

func TestAddOpenStackSuite(t *stdtesting.T) { tc.Run(t, &addOpenStackSuite{}) }
func (s *addOpenStackSuite) TearDownTest(c *tc.C) {
	s.IsolationSuite.TearDownTest(c)
	os.Unsetenv("OS_CACERT")
	os.Unsetenv("OS_AUTH_URL")
}

func (*addOpenStackSuite) TestInteractiveOpenstackCloudCertEnvVar(c *tc.C) {
	fakeCertFilename := path.Join(c.MkDir(), "cloudcert.crt")

	input := "" +
		/* Select cloud type: */ "openstack\n" +
		/* Enter a name for your openstack cloud: */ "os1\n" +
		/* Enter the API endpoint url for the cloud [$OS_AUTH_URL]: */ "\n" +
		/* Enter a path to the CA certificate for your cloud if one is required to access it. (optional) [$OS_CACERT] */ "\n" +
		/* Select one or more auth types separated by commas: */ "userpass,access-key\n" +
		/* Enter region name: */ "regionone\n" +
		/* Enter the API endpoint url for the region [use cloud api url]: */ "\n" +
		/* Enter another region? (Y/n): */ "n\n"

	os.Setenv("OS_CACERT", fakeCertFilename)
	os.Setenv("OS_AUTH_URL", "http://myopenstack")

	testInteractiveOpenstackCloudCert(c, fakeCertFilename, input,
		fmt.Sprintf("Successfully read CA Certificate from %s\n", fakeCertFilename), "")
}

func testInteractiveOpenstackCloudCert(c *tc.C, fakeCertFilename, input, addStdErrMsg, stdOutMsg string) {
	fakeCert := testing.CACert
	os.WriteFile(fakeCertFilename, []byte(fakeCert), 0666)

	myOpenstack := jujucloud.Cloud{
		Name:      "os1",
		Type:      "openstack",
		AuthTypes: []jujucloud.AuthType{"userpass", "access-key"},
		Endpoint:  "http://myopenstack",
		Regions: []jujucloud.Region{
			{
				Name:     "regionone",
				Endpoint: "http://myopenstack",
			},
		},
		CACertificates: []string{fakeCert},
	}

	fakeCertMap := map[string]interface{}{
		"ca-certificates": []string{fakeCert},
	}
	fakeCertYaml, err := yaml.Marshal(fakeCertMap)
	c.Assert(err, tc.IsNil)

	expectedYAMLarg := fmt.Sprintf(expectedCloudYAMLarg, fakeCertYaml, fakeCertFilename)

	testInteractiveOpenstack(c, myOpenstack, expectedYAMLarg, input, addStdErrMsg, stdOutMsg)
}

func testInteractiveOpenstack(c *tc.C, myOpenstack jujucloud.Cloud, expectedYAMLarg, input, addStdErrMsg, stdOutMsg string) {
	fake := newFakeCloudMetadataStore()
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)

	fake.Call("ParseOneCloud", []byte(expectedYAMLarg)).Returns(myOpenstack, nil)
	m1Metadata := map[string]jujucloud.Cloud{"os1": myOpenstack}
	numCallsToWrite := fake.Call("WritePersonalCloudMetadata", addDefaultRegion(m1Metadata)).Returns(nil)

	command := cloud.NewAddCloudCommandForTest(fake, jujuclient.NewMemStore(), nil)
	err := cmdtesting.InitCommand(command, []string{"--client"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader(input)

	err = command.Run(ctx)

	if err != nil {
		fmt.Printf("expectedYAML\n(%s)\n", expectedYAMLarg)
	}

	c.Check(err, tc.ErrorIsNil)
	var output = addStdErrMsg +
		"Cloud \"os1\" successfully added to your local client.\n" +
		"You will need to add a credential for this cloud (`juju add-credential os1`)\n" +
		"before you can use it to bootstrap a controller (`juju bootstrap os1`) or\n" +
		"to create a model (`juju add-model <your model name> os1`).\n"
	c.Assert(cmdtesting.Stderr(ctx), tc.Contains, output)
	c.Assert(cmdtesting.Stdout(ctx), tc.Contains, stdOutMsg)

	c.Check(numCallsToWrite(), tc.Equals, 1)
}
