// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/client/application"
	apiclient "github.com/juju/juju/api/client/client"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/deployer"
	apputils "github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
)

type BundleDeploySuite struct {
	coretesting.BaseSuite

	fakeAPI *fakeDeployAPI
}

func TestBundleDeploySuite(t *stdtesting.T) { tc.Run(t, &BundleDeploySuite{}) }
func (s *BundleDeploySuite) SetUpTest(c *tc.C) {
	cfg := map[string]interface{}{
		"name":           "name",
		"uuid":           "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type":           "foo",
		"secret-backend": "auto",
	}
	s.fakeAPI = vanillaFakeModelAPI(cfg)
	s.fakeAPI.deployerFactoryFunc = deployer.NewDeployerFactory
	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))
}

// DeployBundleYAML uses the given bundle content to create a bundle in the
// local repository and then deploy it. It returns the bundle deployment output
// and error.
func (s *BundleDeploySuite) DeployBundleYAML(c *tc.C, content string, extraArgs ...string) error {
	bundlePath := s.makeBundleDir(c, content)
	args := append([]string{bundlePath}, extraArgs...)
	err := s.runDeploy(c, args...)
	return err
}

func (s *BundleDeploySuite) makeBundleDir(c *tc.C, content string) string {
	bundlePath := filepath.Join(c.MkDir(), "example")
	c.Assert(os.Mkdir(bundlePath, 0777), tc.ErrorIsNil)
	err := os.WriteFile(filepath.Join(bundlePath, "bundle.yaml"), []byte(content), 0644)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(bundlePath, "README.md"), []byte("README"), 0644)
	c.Assert(err, tc.ErrorIsNil)

	return bundlePath
}

func (s *BundleDeploySuite) setupCharm(c *tc.C, url, name string, b base.Base) charm.Charm {
	return s.setupCharmMaybeForce(c, url, name, b, arch.DefaultArchitecture, false)
}

func (s *BundleDeploySuite) setupCharmMaybeForce(c *tc.C, url, name string, abase base.Base, arc string, force bool) charm.Charm {
	baseURL := charm.MustParseURL(url)
	resolveURL := charm.MustParseURL(url)
	if resolveURL.Revision < 0 {
		resolveURL.Revision = 1
	}
	noRevisionURL := charm.MustParseURL(url)
	noRevisionURL.Revision = -1
	charmHubURL := charm.MustParseURL(fmt.Sprintf("ch:%s", baseURL.Name))
	// In order to replicate what the charmstore does in terms of matching, we
	// brute force (badly) the various types of charm urls.
	// TODO (stickupkid): This is terrible, the fact that you're bruteforcing
	// a mock to replicate a charm store, means your test isn't unit testing
	// at any level. Instead we should have tests that know exactly the url
	// is and pass that. The new mocking tests do this.
	charmURLs := []*charm.URL{
		baseURL,
		resolveURL,
		noRevisionURL,
		charmHubURL,
	}
	for _, url := range charmURLs {
		for _, b := range []base.Base{{}, abase} {
			for _, a := range []string{"", arc, arch.DefaultArchitecture} {
				platform := corecharm.Platform{
					Architecture: a,
					OS:           b.OS,
					Channel:      b.Channel.Track,
				}
				origin, err := apputils.MakeOrigin(charm.Schema(url.Schema), url.Revision, charm.Channel{}, platform)
				c.Assert(err, tc.ErrorIsNil)

				s.fakeAPI.Call("ResolveCharm", url, origin, false).Returns(
					resolveURL,
					origin,
					[]base.Base{abase},
					error(nil),
				)

				origin.Risk = "stable"
				s.fakeAPI.Call("ResolveCharm", url, origin, false).Returns(
					resolveURL,
					origin,
					[]base.Base{abase},
					error(nil),
				)

				s.fakeAPI.Call("AddCharm", resolveURL, origin, force).Returns(origin, error(nil))
			}
		}
	}

	var chDir charm.Charm
	chDir, err := charmtesting.ReadCharmDir(testcharms.RepoWithSeries("bionic").CharmDirPath(name))
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			c.Fatal(err)
			return nil
		}
		chDir = testcharms.RepoForSeries("bionic").CharmArchive(c.MkDir(), "dummy")
	}
	return chDir
}

func (s *BundleDeploySuite) setupFakeBundle(c *tc.C, url string, allBase ...base.Base) {
	bundleResolveURL := charm.MustParseURL(url)
	baseURL := *bundleResolveURL
	baseURL.Revision = -1
	withCharmRepoResolvable(s.fakeAPI, &baseURL, base.Base{})

	// Resolve a bundle with no revision and return a url with a version.  Ensure
	// GetBundle expects the url with revision.
	for _, b := range allBase {
		origin, err := apputils.MakeOrigin(charm.Schema(bundleResolveURL.Schema), bundleResolveURL.Revision, charm.Channel{}, corecharm.Platform{
			OS: b.OS, Channel: b.Channel.Track})
		c.Assert(err, tc.ErrorIsNil)
		origin.Revision = nil
		s.fakeAPI.Call("ResolveBundleURL", &baseURL, origin).Returns(
			bundleResolveURL,
			origin,
			error(nil),
		)
		s.fakeAPI.Call("GetBundle", bundleResolveURL).Returns(nil, error(nil))
	}
}

func (s *BundleDeploySuite) runDeploy(c *tc.C, args ...string) error {
	deployCmd := newDeployCommandForTest(s.fakeAPI)
	_, err := cmdtesting.RunCommand(c, deployCmd, args...)
	return err
}

func (s *BundleDeploySuite) TestDeployBundleInvalidFlags(c *tc.C) {
	s.setupCharm(c, "ch:mysql-42", "mysql", base.MustParseBaseFromString("ubuntu@18.04"))
	s.setupCharm(c, "ch:wordpress-47", "wordpress", base.MustParseBaseFromString("ubuntu@18.04"))
	s.setupFakeBundle(c, "ch:wordpress-simple-1", base.Base{}, base.MustParseBaseFromString("ubuntu@18.04"), base.MustParseBaseFromString("ubuntu@16.04"))

	err := s.runDeploy(c, "ch:wordpress-simple", "--config", "config.yaml")
	c.Assert(err, tc.ErrorMatches, "options provided but not supported when deploying a bundle: --config")
	err = s.runDeploy(c, "ch:wordpress-simple", "-n", "2")
	c.Assert(err, tc.ErrorMatches, "options provided but not supported when deploying a bundle: -n")
	err = s.runDeploy(c, "ch:wordpress-simple", "--base", "ubuntu@18.04")
	c.Assert(err, tc.ErrorMatches, "options provided but not supported when deploying a bundle: --base")
}

func (s *BundleDeploySuite) TestDeployBundleLocalPathInvalidBaseWithForce(c *tc.C) {
	s.assertDeployBundleLocalPathInvalidBaseWithForce(c, true)
}

func (s *BundleDeploySuite) TestDeployBundleLocalPathInvalidBaseWithoutForce(c *tc.C) {
	s.assertDeployBundleLocalPathInvalidBaseWithForce(c, false)
}

func (s *BundleDeploySuite) assertDeployBundleLocalPathInvalidBaseWithForce(c *tc.C, force bool) {
	dir := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(dir, "dummy")

	dummyURL := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, dummyURL, charmDir, force)
	withLocalBundleCharmDeployable(
		s.fakeAPI, dummyURL, base.MustParseBaseFromString("ubuntu@12.10"),
		charmDir.Meta(), charmDir.Manifest(), force,
	)

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	path := filepath.Join(dir, "mybundle")
	data := fmt.Sprintf(`
        default-base: ubuntu@12.10
        applications:
            dummy:
                charm: %s
                num_units: 1
    `, charmDir.Path)
	err := os.WriteFile(path, []byte(data), 0644)
	c.Assert(err, tc.ErrorIsNil)
	deployArgs := []string{path}
	if force {
		deployArgs = append(deployArgs, "--force")
	}
	err = s.runDeploy(c, deployArgs...)
	c.Assert(err, tc.ErrorMatches, "cannot deploy bundle: base: ubuntu@12.10/stable not supported")
}

func (s *BundleDeploySuite) TestDeployBundleLocalPathInvalidJujuBase(c *tc.C) {
	dir := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(dir, "jammyonly")

	curl := charm.MustParseURL("local:jammyonly-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withLocalBundleCharmDeployable(
		s.fakeAPI, curl, base.MustParseBaseFromString("ubuntu@20.04"),
		charmDir.Meta(), charmDir.Manifest(), false,
	)

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	path := filepath.Join(dir, "mybundle")
	data := fmt.Sprintf(`
        default-base: ubuntu@20.04
        applications:
            jammyonly:
                charm: %s
                num_units: 1
    `, charmDir.Path)
	err := os.WriteFile(path, []byte(data), 0644)
	c.Assert(err, tc.ErrorIsNil)

	err = s.runDeploy(c, path)
	c.Assert(err, tc.ErrorMatches, `cannot deploy bundle: base "ubuntu@20.04/stable" is not supported, supported bases are: .*`)
}

var deployBundleErrorsTests = []struct {
	about   string
	content string
	err     string
}{{
	about: "local charm not found",
	content: `
       applications:
           mysql:
               charm: ./mysql
               num_units: 1
   `,
	err: `the provided bundle has the following errors:
charm path in application "mysql" does not exist: .*mysql`,
}, {
	about: "charm store charm not found",
	content: `
       applications:
           rails:
               charm: ch:xenial/rails
               num_units: 1
   `,
	err: `cannot resolve charm or bundle "rails": .* charm or bundle not found`,
}, {
	about:   "invalid bundle content",
	content: "!",
	err: `the provided bundle has the following errors:
.*`,
}, {
	about: "invalid bundle data",
	content: `
       applications:
           mysql:
               charm: ch:mysql
               num_units: -1
   `,
	err: `the provided bundle has the following errors:
negative number of units specified on application "mysql"`,
}, {
	about: "invalid constraints",
	content: `
       applications:
           mysql:
               charm: ch:mysql
               num_units: 1
               constraints: bad-wolf
   `,
	err: `the provided bundle has the following errors:
invalid constraints "bad-wolf" in application "mysql": malformed constraint "bad-wolf"`,
}, {
	about: "multiple bundle verification errors",
	content: `
       applications:
           mysql:
               charm: ch:mysql
               num_units: -1
               constraints: bad-wolf
   `,
	err: `the provided bundle has the following errors:
invalid constraints "bad-wolf" in application "mysql": malformed constraint "bad-wolf"
negative number of units specified on application "mysql"`,
}, {
	about: "bundle inception",
	content: `
       applications:
           example:
               charm: local:wordpress
               num_units: 1
   `,
	err: `cannot deploy local charm at ".*wordpress": file does not exist`,
}}

func (s *BundleDeploySuite) TestDeployBundleErrors(c *tc.C) {
	for i, test := range deployBundleErrorsTests {
		c.Logf("test %d: %s", i, test.about)

		var args *apiclient.StatusArgs
		s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

		err := s.DeployBundleYAML(c, test.content)
		pass := c.Check(err, tc.ErrorMatches, "cannot deploy bundle: "+test.err)
		if !pass {
			c.Logf("error: \n%s\n", errors.ErrorStack(err))
		}
	}
}

func (s *BundleDeploySuite) TestDeployBundleLocalDeploymentBadConfig(c *tc.C) {
	charmsPath := c.MkDir()
	mysqlPath := testcharms.RepoWithSeries("bionic").CharmArchivePath(charmsPath, "mysql")
	wordpressPath := testcharms.RepoWithSeries("bionic").CharmArchivePath(charmsPath, "wordpress")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       default-base: ubuntu@16.04
       applications:
           wordpress:
               charm: %s
               num_units: 1
           mysql:
               charm: %s
               num_units: 2
       relations:
           - ["wordpress:db", "mysql:server"]
   `, wordpressPath, mysqlPath),
		"--overlay", "missing-file")
	c.Assert(err, tc.ErrorMatches, `cannot deploy bundle: unable to process overlays: "missing-file" not found`)
}

func (s *BundleDeploySuite) TestDeployBundleLocalDeploymentLXDProfile(c *tc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(charmsPath, "lxd-profile")

	curl := charm.MustParseURL("local:lxd-profile-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withLocalBundleCharmDeployable(
		s.fakeAPI, curl, base.MustParseBaseFromString("ubuntu@20.04"),
		charmDir.Meta(), charmDir.Manifest(), false,
	)

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       default-base: ubuntu@20.04
       applications:
           lxd-profile:
               charm: %s
               num_units: 1
   `, charmDir.Path))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployBundleLocalDeploymentBadLXDProfile(c *tc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(charmsPath, "lxd-profile-fail")

	curl := charm.MustParseURL("local:lxd-profile-fail-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(
		s.fakeAPI, curl, defaultBase,
		&charm.Meta{Name: "lxd-profile"},
		false, 0, nil, nil,
	)

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       default-base: ubuntu@22.04
       applications:
           lxd-profile-fail:
               charm: %s
               num_units: 1
   `, charmDir.Path))
	c.Assert(err, tc.ErrorMatches, "cannot deploy bundle: cannot deploy local charm at .*: invalid lxd-profile.yaml: contains device type \"unix-disk\"")
}

func (s *BundleDeploySuite) TestDeployBundleLocalDeploymentBadLXDProfileWithForce(c *tc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(charmsPath, "lxd-profile-fail")

	curl := charm.MustParseURL("local:lxd-profile-fail-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, true)
	withLocalBundleCharmDeployable(
		s.fakeAPI, curl, base.MustParseBaseFromString("ubuntu@20.04"),
		charmDir.Meta(), charmDir.Manifest(), true,
	)

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       default-base: ubuntu@20.04
       applications:
           lxd-profile-fail:
               charm: %s
               num_units: 1
   `, charmDir.Path), "--force")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployBundleLocalDeploymentWithBundleOverlay(c *tc.C) {
	configDir := c.MkDir()
	configFile := filepath.Join(configDir, "config.yaml")
	c.Assert(
		os.WriteFile(
			configFile, []byte(`
               applications:
                   wordpress:
                       options:
                           blog-title: include-file://title
           `), 0644),
		tc.ErrorIsNil)
	c.Assert(
		os.WriteFile(
			filepath.Join(configDir, "title"), []byte("magic bundle config"), 0644),
		tc.ErrorIsNil)

	charmsPath := c.MkDir()
	mysqlDir := testcharms.RepoWithSeries("bionic").CharmArchive(charmsPath, "mysql")
	wordpressDir := testcharms.RepoWithSeries("bionic").CharmArchive(charmsPath, "wordpress")

	mysqlURL := charm.MustParseURL("local:mysql-1")
	wordpressURL := charm.MustParseURL("local:wordpress-3")
	withLocalCharmDeployable(s.fakeAPI, mysqlURL, mysqlDir, false)
	withLocalCharmDeployable(s.fakeAPI, wordpressURL, wordpressDir, false)
	withLocalBundleCharmDeployable(
		s.fakeAPI, mysqlURL, defaultBase,
		mysqlDir.Meta(), mysqlDir.Manifest(), false,
	)
	withLocalBundleCharmDeployable(
		s.fakeAPI, wordpressURL, defaultBase,
		wordpressDir.Meta(), wordpressDir.Manifest(), false,
	)
	deployArgs := application.DeployArgs{
		CharmID: application.CharmID{
			URL:    wordpressURL.String(),
			Origin: commoncharm.Origin{Source: "local"},
		},
		CharmOrigin:     commoncharm.Origin{Source: "local", Base: defaultBase},
		ApplicationName: "wordpress",
		NumUnits:        0,
		ConfigYAML:      "wordpress:\n  blog-title: magic bundle config\n",
	}
	s.fakeAPI.Call("Deploy", deployArgs).Returns(error(nil))
	s.fakeAPI.Call("AddRelation",
		[]interface{}{"wordpress:db", "mysql:server"}, []interface{}{},
	).Returns(
		&params.AddRelationResults{},
		error(nil),
	)

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       default-base: ubuntu@22.04
       applications:
           wordpress:
               charm: %s
               num_units: 1
           mysql:
               charm: %s
               num_units: 1
       relations:
           - ["wordpress:db", "mysql:server"]
`, wordpressDir.Path, mysqlDir.Path),
		"--overlay", configFile)

	c.Assert(err, tc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployLocalBundleWithRelativeCharmPaths(c *tc.C) {
	bundleDir := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").CharmArchive(bundleDir, "dummy")

	bundleFile := filepath.Join(bundleDir, "bundle.yaml")
	bundleContent := fmt.Sprintf(`
default-base: ubuntu@20.04
applications:
 dummy:
   charm: %s
`, charmDir.Path)
	c.Assert(
		os.WriteFile(bundleFile, []byte(bundleContent), 0644),
		tc.ErrorIsNil)

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	curl := charm.MustParseURL("local:dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withLocalBundleCharmDeployable(
		s.fakeAPI, curl, base.MustParseBaseFromString("ubuntu@20.04"),
		charmDir.Meta(), charmDir.Manifest(), false,
	)

	err := s.runDeploy(c, bundleFile)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployBundleLocalAndCharmhubCharms(c *tc.C) {
	charmsPath := c.MkDir()
	wordpressDir := s.setupCharm(c, "ch:wordpress-1", "wordpress", base.MustParseBaseFromString("ubuntu@20.04"))
	mysqlDir := testcharms.RepoWithSeries("bionic").CharmArchive(charmsPath, "mysql")
	mysqlURL := charm.MustParseURL("local:mysql-1")
	wordpressURL := charm.MustParseURL("ch:wordpress-1")
	withLocalCharmDeployable(s.fakeAPI, mysqlURL, mysqlDir, false)
	withLocalBundleCharmDeployable(
		s.fakeAPI, mysqlURL, defaultBase,
		mysqlDir.Meta(), mysqlDir.Manifest(), false,
	)
	s.fakeAPI.Call("CharmInfo", wordpressURL.String()).Returns(
		&apicommoncharms.CharmInfo{
			URL:      wordpressURL.String(),
			Meta:     wordpressDir.Meta(),
			Manifest: wordpressDir.Manifest(),
		},
		error(nil),
	)
	base := base.MustParseBaseFromString("ubuntu@20.04/stable")
	deployArgs := application.DeployArgs{
		CharmID: application.CharmID{
			URL:    wordpressURL.String(),
			Origin: commoncharm.Origin{Source: "charm-hub", Base: base, Architecture: "amd64", Risk: "stable"},
		},
		CharmOrigin:     commoncharm.Origin{Source: "charm-hub", Base: base, Architecture: "amd64", Risk: "stable"},
		ApplicationName: "wordpress",
		NumUnits:        0,
	}
	s.fakeAPI.Call("Deploy", deployArgs).Returns(error(nil))
	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "wordpress",
		NumUnits:        1,
	}).Returns([]string{"wordpress/0"}, error(nil))
	s.fakeAPI.Call("AddRelation",
		[]interface{}{"wordpress:db", "mysql:server"}, []interface{}{},
	).Returns(
		&params.AddRelationResults{},
		error(nil),
	)

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	err := s.DeployBundleYAML(c, fmt.Sprintf(`
      default-base: ubuntu@22.04
      applications:
          wordpress:
              charm: ch:wordpress
              base: ubuntu@20.04
              num_units: 1
          mysql:
              charm: %s
              num_units: 1
      relations:
          - ["wordpress:db", "mysql:server"]
  `, mysqlDir.Path))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestErrorDeployingBundlesRequiringTrust(c *tc.C) {
	specs := []struct {
		descr      string
		bundle     string
		expAppList []string
	}{
		{
			descr:      "bundle with a single app with the trust field set to true",
			bundle:     "aws-integrator-trust-single",
			expAppList: []string{"aws-integrator"},
		},
		{
			descr:      "bundle with a multiple apps with the trust field set to true",
			bundle:     "aws-integrator-trust-multi",
			expAppList: []string{"aws-integrator", "gcp-integrator"},
		},
		{
			descr:      "bundle with a single app with a 'trust: true' config option",
			bundle:     "aws-integrator-trust-conf-param",
			expAppList: []string{"aws-integrator"},
		},
	}

	for specIndex, spec := range specs {
		c.Logf("[spec %d] %s", specIndex, spec.descr)

		expErr := fmt.Sprintf(`Bundle cannot be deployed without trusting applications with your cloud credentials.
Please repeat the deploy command with the --trust argument if you consent to trust the following application(s):
  - %s`, strings.Join(spec.expAppList, "\n  - "))

		bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), spec.bundle)
		err := s.runDeploy(c, bundlePath)
		c.Assert(err, tc.Not(tc.IsNil))
		c.Assert(err.Error(), tc.Equals, expErr)
	}
}

func (s *BundleDeploySuite) TestDeployBundleWithChannel(c *tc.C) {
	// The second charm from the bundle does not require trust so no
	// additional configuration should be injected
	ubURL := charm.MustParseURL("ch:ubuntu")
	withCharmRepoResolvable(s.fakeAPI, ubURL, base.Base{})

	withCharmDeployable(
		s.fakeAPI, ubURL, defaultBase,
		&charm.Meta{Name: "ubuntu"},
		false, 0, nil, nil,
	)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "ubuntu",
		NumUnits:        1,
	}).Returns([]string{"ubuntu/0"}, error(nil))

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "basic")
	err := s.runDeploy(c, bundlePath, "--channel", "edge")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployBundlesRequiringTrust(c *tc.C) {
	inURL := charm.MustParseURL("ch:aws-integrator")
	withCharmRepoResolvable(s.fakeAPI, inURL, base.MustParseBaseFromString("ubuntu@22.04"))
	withCharmRepoResolvable(s.fakeAPI, inURL, base.Base{})

	// The aws-integrator charm requires trust and since the operator passes
	// --trust we expect to see a "trust: true" config value in the yaml
	// config passed to deploy.
	//
	// As withCharmDeployable does not support passing a "ConfigYAML"
	// it's easier to just invoke it to set up all other calls and then
	// explicitly Deploy here.
	withCharmDeployable(
		s.fakeAPI, inURL, defaultBase,
		&charm.Meta{Name: "aws-integrator"},
		false, 0, nil, nil,
	)

	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmHub,
		Architecture: arch.DefaultArchitecture,
		Base:         base.MakeDefaultBase("ubuntu", "22.04"),
	}

	deployURL := *inURL

	dArgs := application.DeployArgs{
		CharmID: application.CharmID{
			URL:    deployURL.String(),
			Origin: origin,
		},
		CharmOrigin:     origin,
		ApplicationName: inURL.Name,
		ConfigYAML:      "aws-integrator:\n  trust: \"true\"\n",
	}

	s.fakeAPI.Call("Deploy", dArgs).Returns(error(nil))
	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "aws-integrator",
		NumUnits:        1,
	}).Returns([]string{"aws-integrator/0"}, error(nil))

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	// The second charm from the bundle does not require trust so no
	// additional configuration should be injected
	ubURL := charm.MustParseURL("ch:ubuntu")
	withCharmRepoResolvable(s.fakeAPI, ubURL, base.MustParseBaseFromString("ubuntu@22.04"))
	withCharmRepoResolvable(s.fakeAPI, ubURL, base.Base{})
	withCharmDeployable(
		s.fakeAPI, ubURL, defaultBase,
		&charm.Meta{Name: "ubuntu"},
		false, 0, nil, nil,
	)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "ubuntu",
		NumUnits:        1,
	}).Returns([]string{"ubuntu/0"}, error(nil))

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "aws-integrator-trust-single")
	err := s.runDeploy(c, bundlePath, "--trust")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployBundleWithOffers(c *tc.C) {
	inURL := charm.MustParseURL("ch:apache2")
	withCharmRepoResolvable(s.fakeAPI, inURL, base.MustParseBaseFromString("ubuntu@22.04"))
	withCharmRepoResolvable(s.fakeAPI, inURL, base.Base{})

	withCharmDeployable(
		s.fakeAPI, inURL, defaultBase,
		&charm.Meta{Name: "apache2"},
		false, 0, nil, nil,
	)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "apache2",
		NumUnits:        1,
	}).Returns([]string{"apache2/0"}, error(nil))

	s.fakeAPI.Call("Offer",
		"deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"apache2",
		[]string{"apache-website", "website-cache"},
		"king",
		"my-offer",
		"",
	).Returns([]params.ErrorResult{}, nil)

	s.fakeAPI.Call("Offer",
		"deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"apache2",
		[]string{"apache-website"},
		"king",
		"my-other-offer",
		"",
	).Returns([]params.ErrorResult{}, nil)

	s.fakeAPI.Call("GrantOffer",
		"admin",
		"admin",
		[]string{"king/sword.my-offer"},
	).Returns(errors.New(`cannot grant admin access to user admin on offer admin/controller.my-offer: user already has "admin" access or greater`))
	s.fakeAPI.Call("GrantOffer",
		"bar",
		"consume",
		[]string{"king/sword.my-offer"},
	).Returns(nil)

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "apache2-with-offers-legacy")
	err := s.runDeploy(c, bundlePath)
	c.Assert(err, tc.ErrorIsNil)

	var offerCallCount int
	var grantOfferCallCount int
	for _, call := range s.fakeAPI.Calls() {
		switch call.FuncName {
		case "Offer":
			offerCallCount++
		case "GrantOffer":
			grantOfferCallCount++
		}
	}
	c.Assert(offerCallCount, tc.Equals, 2)
	c.Assert(grantOfferCallCount, tc.Equals, 2)
}

func (s *BundleDeploySuite) TestDeployBundleWithSAAS(c *tc.C) {
	inURL := charm.MustParseURL("ch:wordpress")
	withCharmRepoResolvable(s.fakeAPI, inURL, base.MustParseBaseFromString("ubuntu@22.04"))
	withCharmRepoResolvable(s.fakeAPI, inURL, base.Base{})

	withCharmDeployable(
		s.fakeAPI, inURL, defaultBase,
		&charm.Meta{Name: "wordpress"},
		false, 0, nil, nil,
	)

	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "wordpress",
		NumUnits:        1,
	}).Returns([]string{"wordpress/0"}, error(nil))

	s.fakeAPI.Call("GetConsumeDetails",
		"admin/default.mysql",
	).Returns(params.ConsumeOfferDetails{
		Offer: &params.ApplicationOfferDetailsV5{
			OfferName: "mysql",
			OfferURL:  "admin/default.mysql",
		},
		Macaroon: mac,
		ControllerInfo: &params.ExternalControllerInfo{
			ControllerTag: coretesting.ControllerTag.String(),
			Addrs:         []string{"192.168.1.0"},
			Alias:         "controller-alias",
			CACert:        coretesting.CACert,
		},
	}, nil)

	s.fakeAPI.Call("Consume",
		crossmodel.ConsumeApplicationArgs{
			Offer: params.ApplicationOfferDetailsV5{
				OfferName: "mysql",
				OfferURL:  "test:admin/default.mysql",
			},
			ApplicationAlias: "mysql",
			Macaroon:         mac,
			ControllerInfo: &crossmodel.ControllerInfo{
				ControllerUUID: coretesting.ControllerTag.Id(),
				Alias:          "controller-alias",
				Addrs:          []string{"192.168.1.0"},
				CACert:         coretesting.CACert,
			},
		},
	).Returns("mysql", nil)

	s.fakeAPI.Call("AddRelation",
		[]interface{}{"wordpress:db", "mysql:db"}, []interface{}{},
	).Returns(
		&params.AddRelationResults{},
		error(nil),
	)

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	var args *apiclient.StatusArgs
	s.fakeAPI.Call("Status", args).Returns(&params.FullStatus{}, nil)

	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "wordpress-with-saas")
	err = s.runDeploy(c, bundlePath)
	c.Assert(err, tc.ErrorIsNil)
}
