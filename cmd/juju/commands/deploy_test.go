// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/persistent-cookiejar"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v1"
	"gopkg.in/juju/charmrepo.v1/csclient"
	"gopkg.in/juju/charmstore.v5-unstable"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type DeploySuite struct {
	testing.RepoSuite
	CmdBlockHelper
}

func (s *DeploySuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&DeploySuite{})

func runDeploy(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{}), args...)
	return err
}

var initErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: nil,
		err:  `no charm specified`,
	}, {
		args: []string{"charm-name", "service-name", "hotdog"},
		err:  `unrecognized args: \["hotdog"\]`,
	}, {
		args: []string{"craz~ness"},
		err:  `invalid charm name "craz~ness"`,
	}, {
		args: []string{"craziness", "burble-1"},
		err:  `invalid service name "burble-1"`,
	}, {
		args: []string{"craziness", "burble1", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{"craziness", "burble1", "--to", "#:foo"},
		err:  `invalid --to parameter "#:foo"`,
	}, {
		args: []string{"craziness", "burble1", "--constraints", "gibber=plop"},
		err:  `invalid value "gibber=plop" for flag --constraints: unknown constraint "gibber"`,
	},
}

func (s *DeploySuite) TestInitErrors(c *gc.C) {
	for i, t := range initErrorTests {
		c.Logf("test %d", i)
		err := coretesting.InitCommand(envcmd.Wrap(&DeployCommand{}), t.args)
		c.Assert(err, gc.ErrorMatches, t.err)
	}
}

func (s *DeploySuite) TestNoCharm(c *gc.C) {
	err := runDeploy(c, "local:unknown-123")
	c.Assert(err, gc.ErrorMatches, `entity not found in ".*": local:trusty/unknown-123`)
}

func (s *DeploySuite) TestBlockDeploy(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockDeploy")
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	s.AssertBlocked(c, err, ".*TestBlockDeploy.*")
}

func (s *DeploySuite) TestCharmDir(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestUpgradeReportsDeprecated(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{}), "local:dummy", "-u")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")
	output := strings.Split(coretesting.Stderr(ctx), "\n")
	c.Check(output[0], gc.Matches, `Added charm ".*" to the environment.`)
	c.Check(output[1], gc.Equals, "--upgrade (or -u) is deprecated and ignored; charms are always deployed with a unique revision.")
}

func (s *DeploySuite) TestUpgradeCharmDir(c *gc.C) {
	// Add the charm, so the url will exist and a new revision will be
	// picked in ServiceDeploy.
	dummyCharm := s.AddTestingCharm(c, "dummy")

	dirPath := testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:quantal/dummy")
	c.Assert(err, jc.ErrorIsNil)
	upgradedRev := dummyCharm.Revision() + 1
	curl := dummyCharm.URL().WithRevision(upgradedRev)
	s.AssertService(c, "dummy", curl, 1, 0)
	// Check the charm dir was left untouched.
	ch, err := charm.ReadCharmDir(dirPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
}

func (s *DeploySuite) TestCharmBundle(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)
}

func (s *DeploySuite) TestSubordinateCharm(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err := runDeploy(c, "local:logging")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/logging-1")
	s.AssertService(c, "logging", curl, 0, 0)
}

func (s *DeploySuite) TestConfig(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, "local:dummy", "dummy-service", "--config", path)
	c.Assert(err, jc.ErrorIsNil)
	service, err := s.State.Service("dummy-service")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := service.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"skill-level": int64(9000),
		"username":    "admin001",
	})
}

func (s *DeploySuite) TestRelativeConfigPath(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	// Putting a config file in home is okay as $HOME is set to a tempdir
	setupConfigFile(c, utils.Home())
	err := runDeploy(c, "local:dummy", "dummy-service", "--config", "~/testconfig.yaml")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DeploySuite) TestConfigError(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	path := setupConfigFile(c, c.MkDir())
	err := runDeploy(c, "local:dummy", "other-service", "--config", path)
	c.Assert(err, gc.ErrorMatches, `no settings found for "other-service"`)
	_, err = s.State.Service("other-service")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DeploySuite) TestConstraints(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "--constraints", "mem=2G cpu-cores=2 networks=net1,^net2")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	service, _ := s.AssertService(c, "dummy", curl, 1, 0)
	cons, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, constraints.MustParse("mem=2G cpu-cores=2 networks=net1,^net2"))
}

func (s *DeploySuite) TestNetworks(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "--networks", ", net1, net2 , ", "--constraints", "mem=2G cpu-cores=2 networks=net1,net0,^net3,^net4")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	service, _ := s.AssertService(c, "dummy", curl, 1, 0)
	networks, err := service.Networks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networks, jc.DeepEquals, []string{"net1", "net2"})
	cons, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, constraints.MustParse("mem=2G cpu-cores=2 networks=net1,net0,^net3,^net4"))
}

// TODO(wallyworld) - add another test that deploy with storage fails for older environments
// (need deploy client to be refactored to use API stub)
func (s *DeploySuite) TestStorage(c *gc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	testcharms.Repo.CharmArchivePath(s.SeriesPath, "storage-block")
	err = runDeploy(c, "local:storage-block", "--storage", "data=loop-pool,1G")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/storage-block-1")
	service, _ := s.AssertService(c, "storage-block", curl, 1, 0)

	cons, err := service.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Pool:  "loop-pool",
			Count: 1,
			Size:  1024,
		},
		"allecto": {
			Pool:  "loop",
			Count: 0,
			Size:  1024,
		},
	})
}

// TODO(wallyworld) - add another test that deploy with placement fails for older environments
// (need deploy client to be refactored to use API stub)
func (s *DeploySuite) TestPlacement(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	// Add a machine that will be ignored due to placement directive.
	machine, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = runDeploy(c, "local:dummy", "-n", "1", "--to", "valid")
	c.Assert(err, jc.ErrorIsNil)

	svc, err := s.State.Service("dummy")
	c.Assert(err, jc.ErrorIsNil)
	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Not(gc.Equals), machine.Id())
}

func (s *DeploySuite) TestSubordinateConstraints(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err := runDeploy(c, "local:logging", "--constraints", "mem=1G")
	c.Assert(err, gc.ErrorMatches, "cannot use --constraints with subordinate service")
}

func (s *DeploySuite) TestNumUnits(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-n", "13")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "dummy", curl, 13, 0)
}

func (s *DeploySuite) TestNumUnitsSubordinate(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err := runDeploy(c, "--num-units", "3", "local:logging")
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate service")
	_, err = s.State.Service("dummy")
	c.Assert(err, gc.ErrorMatches, `service "dummy" not found`)
}

func (s *DeploySuite) assertForceMachine(c *gc.C, machineId string) {
	svc, err := s.State.Service("portlandia")
	c.Assert(err, jc.ErrorIsNil)
	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machineId)
}

func (s *DeploySuite) TestForceMachine(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	machine, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", machine.Id(), "local:dummy", "portlandia")
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, machine.Id())
}

func (s *DeploySuite) TestForceMachineExistingContainer(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	template := state.MachineTemplate{
		Series: coretesting.FakeDefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", container.Id(), "local:dummy", "portlandia")
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, container.Id())
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNewContainer(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	machine, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = runDeploy(c, "--to", "lxc:"+machine.Id(), "local:dummy", "portlandia")
	c.Assert(err, jc.ErrorIsNil)
	s.assertForceMachine(c, machine.Id()+"/lxc/0")
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 2)
}

func (s *DeploySuite) TestForceMachineNotFound(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "--to", "42", "local:dummy", "portlandia")
	c.Assert(err, gc.ErrorMatches, `cannot deploy "portlandia" to machine 42: machine 42 not found`)
	_, err = s.State.Service("portlandia")
	c.Assert(err, gc.ErrorMatches, `service "portlandia" not found`)
}

func (s *DeploySuite) TestForceMachineSubordinate(c *gc.C) {
	machine, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err = runDeploy(c, "--to", machine.Id(), "local:logging")
	c.Assert(err, gc.ErrorMatches, "cannot use --num-units or --to with subordinate service")
	_, err = s.State.Service("dummy")
	c.Assert(err, gc.ErrorMatches, `service "dummy" not found`)
}

func (s *DeploySuite) TestNonLocalCannotHostUnits(c *gc.C) {
	err := runDeploy(c, "--to", "0", "local:dummy", "portlandia")
	c.Assert(err, gc.Not(gc.ErrorMatches), "machine 0 is the state server for a local environment and cannot host units")
}

type DeployLocalSuite struct {
	testing.RepoSuite
}

var _ = gc.Suite(&DeployLocalSuite{})

func (s *DeployLocalSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)

	// override provider type
	s.PatchValue(&service.GetClientConfig, func(client service.ServiceAddUnitAPI) (*config.Config, error) {
		attrs, err := client.EnvironmentGet()
		if err != nil {
			return nil, err
		}
		attrs["type"] = "local"

		return config.New(config.NoDefaults, attrs)
	})
}

func (s *DeployLocalSuite) TestLocalCannotHostUnits(c *gc.C) {
	err := runDeploy(c, "--to", "0", "local:dummy", "portlandia")
	c.Assert(err, gc.ErrorMatches, "machine 0 is the state server for a local environment and cannot host units")
}

// setupConfigFile creates a configuration file for testing set
// with the --config argument specifying a configuration file.
func setupConfigFile(c *gc.C, dir string) string {
	ctx := coretesting.ContextForDir(c, dir)
	path := ctx.AbsPath("testconfig.yaml")
	content := []byte("dummy-service:\n  skill-level: 9000\n  username: admin001\n\n")
	err := ioutil.WriteFile(path, content, 0666)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

type DeployCharmStoreSuite struct {
	charmStoreSuite
}

var _ = gc.Suite(&DeployCharmStoreSuite{})

var deployAuthorizationTests = []struct {
	about        string
	uploadURL    string
	deployURL    string
	readPermUser string
	expectError  string
	expectOutput string
}{{
	about:        "public charm, success",
	uploadURL:    "cs:~bob/trusty/wordpress1-10",
	deployURL:    "cs:~bob/trusty/wordpress1",
	expectOutput: `Added charm "cs:~bob/trusty/wordpress1-10" to the environment.`,
}, {
	about:        "public charm, fully resolved, success",
	uploadURL:    "cs:~bob/trusty/wordpress2-10",
	deployURL:    "cs:~bob/trusty/wordpress2-10",
	expectOutput: `Added charm "cs:~bob/trusty/wordpress2-10" to the environment.`,
}, {
	about:        "non-public charm, success",
	uploadURL:    "cs:~bob/trusty/wordpress3-10",
	deployURL:    "cs:~bob/trusty/wordpress3",
	readPermUser: clientUserName,
	expectOutput: `Added charm "cs:~bob/trusty/wordpress3-10" to the environment.`,
}, {
	about:        "non-public charm, fully resolved, success",
	uploadURL:    "cs:~bob/trusty/wordpress4-10",
	deployURL:    "cs:~bob/trusty/wordpress4-10",
	readPermUser: clientUserName,
	expectOutput: `Added charm "cs:~bob/trusty/wordpress4-10" to the environment.`,
}, {
	about:        "non-public charm, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress5-10",
	deployURL:    "cs:~bob/trusty/wordpress5",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/trusty/wordpress5": cannot get "/~bob/trusty/wordpress5/meta/any\?include=id": unauthorized: access denied for user "client-username"`,
}, {
	about:        "non-public charm, fully resolved, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress6-47",
	deployURL:    "cs:~bob/trusty/wordpress6-47",
	readPermUser: "bob",
	expectError:  `cannot retrieve charm "cs:~bob/trusty/wordpress6-47": cannot get archive: unauthorized: access denied for user "client-username"`,
}, {
	about:     "public bundle, success",
	uploadURL: "cs:~bob/bundle/wordpress-simple1-42",
	deployURL: "cs:~bob/bundle/wordpress-simple1",
	expectOutput: `
added charm cs:trusty/mysql-0
service mysql deployed (charm: cs:trusty/mysql-0)
added charm cs:trusty/wordpress-1
service wordpress deployed (charm: cs:trusty/wordpress-1)
related wordpress:db and mysql:server
added mysql/0 unit to new machine
added wordpress/0 unit to new machine
deployment of bundle "cs:~bob/bundle/wordpress-simple1-42" completed`,
}, {
	about:        "non-public bundle, success",
	uploadURL:    "cs:~bob/bundle/wordpress-simple2-0",
	deployURL:    "cs:~bob/bundle/wordpress-simple2-0",
	readPermUser: clientUserName,
	expectOutput: `
added charm cs:trusty/mysql-0
reusing service mysql (charm: cs:trusty/mysql-0)
added charm cs:trusty/wordpress-1
reusing service wordpress (charm: cs:trusty/wordpress-1)
wordpress:db and mysql:server are already related
avoid adding new units to service mysql: 1 unit already present
avoid adding new units to service wordpress: 1 unit already present
deployment of bundle "cs:~bob/bundle/wordpress-simple2-0" completed`,
}, {
	about:        "non-public bundle, access denied",
	uploadURL:    "cs:~bob/bundle/wordpress-simple3-47",
	deployURL:    "cs:~bob/bundle/wordpress-simple3",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/bundle/wordpress-simple3": cannot get "/~bob/bundle/wordpress-simple3/meta/any\?include=id": unauthorized: access denied for user "client-username"`,
}}

func (s *DeployCharmStoreSuite) TestDeployAuthorization(c *gc.C) {
	// Upload the two charms required to upload the bundle.
	testcharms.UploadCharm(c, s.client, "trusty/mysql-0", "mysql")
	testcharms.UploadCharm(c, s.client, "trusty/wordpress-1", "wordpress")

	// Run the tests.
	for i, test := range deployAuthorizationTests {
		c.Logf("test %d: %s", i, test.about)

		// Upload the charm or bundle under test.
		url := charm.MustParseURL(test.uploadURL)
		if url.Series == "bundle" {
			url, _ = testcharms.UploadBundle(c, s.client, test.uploadURL, "wordpress-simple")
		} else {
			url, _ = testcharms.UploadCharm(c, s.client, test.uploadURL, "wordpress")
		}

		// Change the ACL of the uploaded entity if required in this case.
		if test.readPermUser != "" {
			s.changeReadPerm(c, url, test.readPermUser)
		}

		// Deploy the entity and check output or possible errors.
		ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{}), test.deployURL, fmt.Sprintf("wordpress%d", i))
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		output := strings.Trim(coretesting.Stderr(ctx), "\n")
		c.Assert(output, gc.Equals, strings.TrimSpace(test.expectOutput))
	}
}

const (
	// clientUserCookie is the name of the cookie which is
	// used to signal to the charmStoreSuite macaroon discharger
	// that the client is a juju client rather than the juju environment.
	clientUserCookie = "client"

	// clientUserName is the name chosen for the juju client
	// when it has authorized.
	clientUserName = "client-username"
)

// charmStoreSuite is a suite fixture that puts the machinery in
// place to allow testing code that calls addCharmViaAPI.
type charmStoreSuite struct {
	testing.JujuConnSuite
	handler    charmstore.HTTPCloseHandler
	srv        *httptest.Server
	client     *csclient.Client
	discharger *bakerytest.Discharger
}

func (s *charmStoreSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Set up the third party discharger.
	s.discharger = bakerytest.NewDischarger(nil, func(req *http.Request, cond string, arg string) ([]checkers.Caveat, error) {
		cookie, err := req.Cookie(clientUserCookie)
		if err != nil {
			return nil, errors.Annotate(err, "discharge denied to non-clients")
		}
		return []checkers.Caveat{
			checkers.DeclaredCaveat("username", cookie.Value),
		}, nil
	})

	// Set up the charm store testing server.
	db := s.Session.DB("juju-testing")
	params := charmstore.ServerParams{
		AuthUsername:     "test-user",
		AuthPassword:     "test-password",
		IdentityLocation: s.discharger.Location(),
		PublicKeyLocator: s.discharger,
	}
	handler, err := charmstore.NewServer(db, nil, "", params, charmstore.V4)
	c.Assert(err, jc.ErrorIsNil)
	s.handler = handler
	s.srv = httptest.NewServer(handler)
	s.client = csclient.New(csclient.Params{
		URL:      s.srv.URL,
		User:     params.AuthUsername,
		Password: params.AuthPassword,
	})

	// Initialize the charm cache dir.
	s.PatchValue(&charmrepo.CacheDir, c.MkDir())

	// Point the CLI to the charm store testing server.
	original := newCharmStoreClient
	s.PatchValue(&newCharmStoreClient, func() (*csClient, error) {
		csclient, err := original()
		if err != nil {
			return nil, err
		}
		csclient.params.URL = s.srv.URL
		// Add a cookie so that the discharger can detect whether the
		// HTTP client is the juju environment or the juju client.
		lurl, err := url.Parse(s.discharger.Location())
		if err != nil {
			panic(err)
		}
		csclient.params.HTTPClient.Jar.SetCookies(lurl, []*http.Cookie{{
			Name:  clientUserCookie,
			Value: clientUserName,
		}})
		return csclient, nil
	})

	// Point the Juju API server to the charm store testing server.
	s.PatchValue(&csclient.ServerURL, s.srv.URL)
}

func (s *charmStoreSuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.handler.Close()
	s.srv.Close()
	s.JujuConnSuite.TearDownTest(c)
}

// changeReadPerm changes the read permission of the given charm URL.
// The charm must be present in the testing charm store.
func (s *charmStoreSuite) changeReadPerm(c *gc.C, url *charm.URL, perms ...string) {
	err := s.client.Put("/"+url.Path()+"/meta/perm/read", perms)
	c.Assert(err, jc.ErrorIsNil)
}

// assertCharmsUplodaded checks that the given charm ids have been uploaded.
func (s *charmStoreSuite) assertCharmsUplodaded(c *gc.C, ids ...string) {
	charms, err := s.State.AllCharms()
	c.Assert(err, jc.ErrorIsNil)
	uploaded := make([]string, len(charms))
	for i, charm := range charms {
		uploaded[i] = charm.URL().String()
	}
	c.Assert(uploaded, jc.SameContents, ids)
}

// serviceInfo holds information about a deployed service.
type serviceInfo struct {
	charm       string
	config      charm.Settings
	constraints constraints.Value
}

// assertServicesDeployed checks that the given services have been deployed.
func (s *charmStoreSuite) assertServicesDeployed(c *gc.C, info map[string]serviceInfo) {
	services, err := s.State.AllServices()
	c.Assert(err, jc.ErrorIsNil)
	deployed := make(map[string]serviceInfo, len(services))
	for _, service := range services {
		charm, _ := service.CharmURL()
		config, err := service.ConfigSettings()
		c.Assert(err, jc.ErrorIsNil)
		if len(config) == 0 {
			config = nil
		}
		constraints, err := service.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		deployed[service.Name()] = serviceInfo{
			charm:       charm.String(),
			config:      config,
			constraints: constraints,
		}
	}
	c.Assert(deployed, jc.DeepEquals, info)
}

// assertRelationsEstablished checks that the given relations have been set.
func (s *charmStoreSuite) assertRelationsEstablished(c *gc.C, relations ...string) {
	rs, err := s.State.AllRelations()
	c.Assert(err, jc.ErrorIsNil)
	established := make([]string, len(rs))
	for i, r := range rs {
		established[i] = r.String()
	}
	c.Assert(established, jc.SameContents, relations)
}

// assertUnitsCreated checks that the given units have been created. The
// expectedUnits argument maps unit names to machine names.
func (s *charmStoreSuite) assertUnitsCreated(c *gc.C, expectedUnits map[string]string) {
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	created := make(map[string]string)
	for _, m := range machines {
		id := m.Id()
		units, err := s.State.UnitsFor(id)
		c.Assert(err, jc.ErrorIsNil)
		for _, u := range units {
			created[u.Name()] = id
		}
	}
	c.Assert(created, jc.DeepEquals, expectedUnits)
}

type testMetricCredentialsSetter struct {
	assert func(string, []byte)
}

func (t *testMetricCredentialsSetter) SetMetricCredentials(serviceName string, data []byte) error {
	t.assert(serviceName, data)
	return nil
}

func (t *testMetricCredentialsSetter) Close() error {
	return nil
}

func (s *DeploySuite) TestAddMetricCredentialsDefault(c *gc.C) {
	var called bool
	setter := &testMetricCredentialsSetter{
		assert: func(serviceName string, data []byte) {
			called = true
			c.Assert(serviceName, gc.DeepEquals, "metered")
			var b []byte
			err := json.Unmarshal(data, &b)
			c.Assert(err, gc.IsNil)
			c.Assert(string(b), gc.Equals, "hello registration")
		},
	}

	cleanup := jujutesting.PatchValue(&getMetricCredentialsAPI, func(_ api.Connection) (metricCredentialsAPI, error) {
		return setter, nil
	})
	defer cleanup()

	handler := &testMetricsRegistrationHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	testcharms.Repo.ClonedDirPath(s.SeriesPath, "metered")
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{RegisterURL: server.URL}), "local:quantal/metered-1")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:quantal/metered-1")
	s.AssertService(c, "metered", curl, 1, 0)
	c.Assert(called, jc.IsTrue)
}

func (s *DeploySuite) TestAddMetricCredentialsDefaultForUnmeteredCharm(c *gc.C) {
	var called bool
	setter := &testMetricCredentialsSetter{
		assert: func(serviceName string, data []byte) {
			called = true
			c.Assert(serviceName, gc.DeepEquals, "dummy")
			c.Assert(data, gc.DeepEquals, []byte{})
		},
	}

	cleanup := jujutesting.PatchValue(&getMetricCredentialsAPI, func(_ api.Connection) (metricCredentialsAPI, error) {
		return setter, nil
	})
	defer cleanup()

	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:trusty/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
	c.Assert(called, jc.IsFalse)
}

func (s *DeploySuite) TestAddMetricCredentialsHttp(c *gc.C) {
	handler := &testMetricsRegistrationHandler{}
	server := httptest.NewServer(handler)
	defer server.Close()

	var called bool
	setter := &testMetricCredentialsSetter{
		assert: func(serviceName string, data []byte) {
			called = true
			c.Assert(serviceName, gc.DeepEquals, "metered")
			var b []byte
			err := json.Unmarshal(data, &b)
			c.Assert(err, gc.IsNil)
			c.Assert(string(b), gc.Equals, "hello registration")
		},
	}

	cleanup := jujutesting.PatchValue(&getMetricCredentialsAPI, func(_ api.Connection) (metricCredentialsAPI, error) {
		return setter, nil
	})
	defer cleanup()

	testcharms.Repo.ClonedDirPath(s.SeriesPath, "metered")
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{RegisterURL: server.URL}), "local:quantal/metered-1")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:quantal/metered-1")
	s.AssertService(c, "metered", curl, 1, 0)
	c.Assert(called, jc.IsTrue)

	c.Assert(handler.registrationCalls, gc.HasLen, 1)
	c.Assert(handler.registrationCalls[0].CharmURL, gc.DeepEquals, "local:quantal/metered-1")
	c.Assert(handler.registrationCalls[0].ServiceName, gc.DeepEquals, "metered")
}

func (s *DeploySuite) TestDeployCharmsEndpointNotImplemented(c *gc.C) {

	s.PatchValue(&registerMeteredCharm, func(r string, s api.Connection, j *cookiejar.Jar, c string, sv, e string) error {
		return &params.Error{"IsMetered", params.CodeNotImplemented}
	})

	testcharms.Repo.ClonedDirPath(s.SeriesPath, "dummy")
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&DeployCommand{}), "local:dummy")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(c.GetTestLog(), jc.Contains, "current state server version does not support charm metering")
}
