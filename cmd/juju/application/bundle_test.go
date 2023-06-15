// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/application"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicommoncharms "github.com/juju/juju/api/common/charms"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/cmd/juju/application/deployer"
	apputils "github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type BundleDeploySuite struct {
	coretesting.BaseSuite

	fakeAPI *fakeDeployAPI
}

var _ = gc.Suite(&BundleDeploySuite{})

func (s *BundleDeploySuite) SetUpTest(c *gc.C) {
	cfg := map[string]interface{}{
		"name":           "name",
		"uuid":           "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type":           "foo",
		"secret-backend": "auto",
	}
	s.fakeAPI = vanillaFakeModelAPI(cfg)
	s.fakeAPI.deployerFactoryFunc = deployer.NewDeployerFactory
	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))
	withAllWatcher(s.fakeAPI)
}

// DeployBundleYAML uses the given bundle content to create a bundle in the
// local repository and then deploy it. It returns the bundle deployment output
// and error.
func (s *BundleDeploySuite) DeployBundleYAML(c *gc.C, content string, extraArgs ...string) error {
	bundlePath := s.makeBundleDir(c, content)
	args := append([]string{bundlePath}, extraArgs...)
	err := s.runDeploy(c, args...)
	return err
}

func (s *BundleDeploySuite) makeBundleDir(c *gc.C, content string) string {
	bundlePath := filepath.Join(c.MkDir(), "example")
	c.Assert(os.Mkdir(bundlePath, 0777), jc.ErrorIsNil)
	err := os.WriteFile(filepath.Join(bundlePath, "bundle.yaml"), []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(bundlePath, "README.md"), []byte("README"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	return bundlePath
}

func (s *BundleDeploySuite) setupCharm(c *gc.C, url, name, series string) charm.Charm {
	return s.setupCharmMaybeForce(c, url, name, series, arch.DefaultArchitecture, false)
}

func (s *BundleDeploySuite) setupCharmMaybeForce(c *gc.C, url, name, aseries, arc string, force bool) charm.Charm {
	baseURL := charm.MustParseURL(url)
	baseURL.Series = ""
	deployURL := charm.MustParseURL(url)
	resolveURL := charm.MustParseURL(url)
	if resolveURL.Revision < 0 {
		resolveURL.Revision = 1
	}
	noRevisionURL := charm.MustParseURL(url)
	noRevisionURL.Series = resolveURL.Series
	noRevisionURL.Revision = -1
	charmHubURL := charm.MustParseURL(fmt.Sprintf("ch:%s", baseURL.Name))
	seriesURL := charm.MustParseURL(url)
	seriesURL.Series = aseries
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
		deployURL,
		charmHubURL,
		seriesURL,
	}
	for _, url := range charmURLs {
		for _, serie := range []string{"", url.Series, aseries} {
			var base series.Base
			if serie != "" {
				var err error
				base, err = series.GetBaseFromSeries(serie)
				c.Assert(err, jc.ErrorIsNil)
			}
			for _, a := range []string{"", arc, arch.DefaultArchitecture} {
				platform := corecharm.Platform{
					Architecture: a,
					OS:           base.OS,
					Channel:      base.Channel.Track,
				}
				origin, err := apputils.DeduceOrigin(url, charm.Channel{}, platform)
				c.Assert(err, jc.ErrorIsNil)

				s.fakeAPI.Call("ResolveCharm", url, origin, false).Returns(
					resolveURL,
					origin,
					[]string{aseries},
					error(nil),
				)

				origin.Risk = "stable"
				s.fakeAPI.Call("ResolveCharm", url, origin, false).Returns(
					resolveURL,
					origin,
					[]string{aseries},
					error(nil),
				)

				s.fakeAPI.Call("AddCharm", resolveURL, origin, force).Returns(origin, error(nil))
			}
		}
	}

	var chDir charm.Charm
	chDir, err := charm.ReadCharmDir(testcharms.RepoWithSeries(aseries).CharmDirPath(name))
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			c.Fatal(err)
			return nil
		}
		chDir = testcharms.RepoForSeries(aseries).CharmArchive(c.MkDir(), "dummy")
	}
	return chDir
}

func (s *BundleDeploySuite) setupBundle(c *gc.C, url, name string, allSeries ...string) {
	bundleResolveURL := charm.MustParseURL(url)
	baseURL := *bundleResolveURL
	baseURL.Revision = -1
	withCharmRepoResolvable(s.fakeAPI, &baseURL, "")
	bundleDir := testcharms.RepoWithSeries(allSeries[0]).BundleArchive(c.MkDir(), name)

	// Resolve a bundle with no revision and return a url with a version.  Ensure
	// GetBundle expects the url with revision.
	for _, serie := range append([]string{"", baseURL.Series}, allSeries...) {
		var base series.Base
		if serie != "" && serie != "bundle" {
			var err error
			base, err = series.GetBaseFromSeries(serie)
			c.Assert(err, jc.ErrorIsNil)
		}
		origin, err := apputils.DeduceOrigin(bundleResolveURL, charm.Channel{}, corecharm.Platform{
			OS: base.OS, Channel: base.Channel.Track})
		c.Assert(err, jc.ErrorIsNil)
		origin.Revision = nil
		s.fakeAPI.Call("ResolveBundleURL", &baseURL, origin).Returns(
			bundleResolveURL,
			origin,
			error(nil),
		)
		s.fakeAPI.Call("GetBundle", bundleResolveURL).Returns(bundleDir, error(nil))
	}
}

func (s *BundleDeploySuite) runDeploy(c *gc.C, args ...string) error {
	deployCmd := newDeployCommandForTest(s.fakeAPI)
	_, err := cmdtesting.RunCommand(c, deployCmd, args...)
	return err
}

func (s *BundleDeploySuite) TestDeployBundleInvalidFlags(c *gc.C) {
	s.setupCharm(c, "ch:xenial/mysql-42", "mysql", "bionic")
	s.setupCharm(c, "ch:xenial/wordpress-47", "wordpress", "bionic")
	s.setupBundle(c, "ch:bundle/wordpress-simple-1", "wordpress-simple", "bionic", "xenial")

	err := s.runDeploy(c, "ch:bundle/wordpress-simple", "--config", "config.yaml")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: --config")
	err = s.runDeploy(c, "ch:bundle/wordpress-simple", "-n", "2")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: -n")
	err = s.runDeploy(c, "ch:bundle/wordpress-simple", "--series", "xenial")
	c.Assert(err, gc.ErrorMatches, "options provided but not supported when deploying a bundle: --series")
}

func (s *BundleDeploySuite) TestDeployBundleLocalPathInvalidSeriesWithForce(c *gc.C) {
	s.assertDeployBundleLocalPathInvalidSeriesWithForce(c, true)
}

func (s *BundleDeploySuite) TestDeployBundleLocalPathInvalidSeriesWithoutForce(c *gc.C) {
	s.assertDeployBundleLocalPathInvalidSeriesWithForce(c, false)
}

func (s *BundleDeploySuite) assertDeployBundleLocalPathInvalidSeriesWithForce(c *gc.C, force bool) {
	restore := testing.PatchValue(&deployer.SupportedJujuSeries,
		func(time.Time, string, string) (set.Strings, error) {
			return set.NewStrings(
				"focal", "xenial", "quantal",
			), nil
		},
	)
	defer restore()

	dir := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(dir, "dummy")

	dummyURL := charm.MustParseURL("local:quantal/dummy-1")
	withLocalCharmDeployable(s.fakeAPI, dummyURL, charmDir, force)
	withLocalBundleCharmDeployable(
		s.fakeAPI, dummyURL, series.MustParseBaseFromString("ubuntu@12.10"),
		&charm.Meta{Name: "dummy"}, force,
	)
	s.fakeAPI.Call("CharmInfo", "local:quantal/dummy-1").Returns(
		&apicommoncharms.CharmInfo{
			URL:  "local:dummy",
			Meta: &charm.Meta{Name: "dummy", Series: []string{"jammy"}},
		},
		error(nil),
	)

	path := filepath.Join(dir, "mybundle")
	data := `
        series: quantal
        applications:
            dummy:
                charm: ./dummy
                num_units: 1
    `
	err := os.WriteFile(path, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	args := []string{path}
	if force {
		args = append(args, "--force")
	}
	err = s.runDeploy(c, args...)
	if !force {
		c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: dummy is not available on the following series: quantal")
		return
	}
	c.Assert(err, jc.ErrorIsNil)
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

func (s *BundleDeploySuite) TestDeployBundleErrors(c *gc.C) {
	for i, test := range deployBundleErrorsTests {
		c.Logf("test %d: %s", i, test.about)
		err := s.DeployBundleYAML(c, test.content)
		pass := c.Check(err, gc.ErrorMatches, "cannot deploy bundle: "+test.err)
		if !pass {
			c.Logf("error: \n%s\n", errors.ErrorStack(err))
		}
	}
}

func (s *BundleDeploySuite) TestDeployBundleLocalDeploymentBadConfig(c *gc.C) {
	charmsPath := c.MkDir()
	mysqlPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "mysql")
	wordpressPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(charmsPath, "wordpress")
	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       series: xenial
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
	c.Assert(err, gc.ErrorMatches, `cannot deploy bundle: unable to process overlays: "missing-file" not found`)
}

func (s *BundleDeploySuite) TestDeployBundleLocalDeploymentLXDProfile(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "lxd-profile")

	curl := charm.MustParseURL("local:focal/lxd-profile-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withLocalBundleCharmDeployable(
		s.fakeAPI, curl, series.MustParseBaseFromString("ubuntu@20.04"),
		&charm.Meta{Name: "lxd-profile"}, false,
	)

	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       series: focal
       applications:
           lxd-profile:
               charm: %s
               num_units: 1
   `, charmDir.Path))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployBundleLocalDeploymentBadLXDProfile(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "lxd-profile-fail")

	curl := charm.MustParseURL("local:jammy/lxd-profile-fail-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(
		s.fakeAPI, curl, defaultBase,
		&charm.Meta{Name: "lxd-profile"},
		nil, false, 0, nil, nil,
	)

	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       series: jammy
       applications:
           lxd-profile-fail:
               charm: %s
               num_units: 1
   `, charmDir.Path))
	c.Assert(err, gc.ErrorMatches, "cannot deploy bundle: cannot deploy local charm at .*: invalid lxd-profile.yaml: contains device type \"unix-disk\"")
}

func (s *BundleDeploySuite) TestDeployBundleLocalDeploymentBadLXDProfileWithForce(c *gc.C) {
	charmsPath := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "lxd-profile-fail")

	curl := charm.MustParseURL("local:focal/lxd-profile-fail-0")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, true)
	withLocalBundleCharmDeployable(
		s.fakeAPI, curl, series.MustParseBaseFromString("ubuntu@20.04"),
		&charm.Meta{Name: "lxd-profile-fail"}, true,
	)

	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       series: focal
       applications:
           lxd-profile-fail:
               charm: %s
               num_units: 1
   `, charmDir.Path), "--force")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployBundleLocalDeploymentWithBundleOverlay(c *gc.C) {
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
		jc.ErrorIsNil)
	c.Assert(
		os.WriteFile(
			filepath.Join(configDir, "title"), []byte("magic bundle config"), 0644),
		jc.ErrorIsNil)

	charmsPath := c.MkDir()
	mysqlDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "mysql")
	wordpressDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "wordpress")

	mysqlURL := charm.MustParseURL("local:jammy/mysql-1")
	wordpressURL := charm.MustParseURL("local:jammy/wordpress-3")
	withLocalCharmDeployable(s.fakeAPI, mysqlURL, mysqlDir, false)
	withLocalCharmDeployable(s.fakeAPI, wordpressURL, wordpressDir, false)
	withLocalBundleCharmDeployable(
		s.fakeAPI, mysqlURL, defaultBase,
		&charm.Meta{Name: "mysql"}, false,
	)
	withLocalBundleCharmDeployable(
		s.fakeAPI, wordpressURL, defaultBase,
		&charm.Meta{Name: "wordpress"}, false,
	)
	deployArgs := application.DeployArgs{
		CharmID: application.CharmID{
			URL:    wordpressURL,
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

	err := s.DeployBundleYAML(c, fmt.Sprintf(`
       series: jammy
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

	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployLocalBundleWithRelativeCharmPaths(c *gc.C) {
	bundleDir := c.MkDir()
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(bundleDir, "dummy")

	bundleFile := filepath.Join(bundleDir, "bundle.yaml")
	bundleContent := `
series: focal
applications:
 dummy:
   charm: ./dummy
`
	c.Assert(
		os.WriteFile(bundleFile, []byte(bundleContent), 0644),
		jc.ErrorIsNil)

	curl := charm.MustParseURL("local:focal/dummy-1")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withLocalBundleCharmDeployable(
		s.fakeAPI, curl, series.MustParseBaseFromString("ubuntu@20.04"),
		&charm.Meta{Name: "dummy"}, false,
	)

	err := s.runDeploy(c, bundleFile)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployBundleLocalAndCharmhubCharms(c *gc.C) {
	charmsPath := c.MkDir()
	s.setupCharm(c, "ch:bionic/wordpress-1", "wordpress", "bionic")
	mysqlDir := testcharms.RepoWithSeries("bionic").ClonedDir(charmsPath, "mysql")
	mysqlURL := charm.MustParseURL("local:jammy/mysql-1")
	wordpressURL := charm.MustParseURL("ch:bionic/wordpress-1")
	withLocalCharmDeployable(s.fakeAPI, mysqlURL, mysqlDir, false)
	withLocalBundleCharmDeployable(
		s.fakeAPI, mysqlURL, defaultBase,
		&charm.Meta{Name: "mysql"}, false,
	)
	s.fakeAPI.Call("CharmInfo", wordpressURL.String()).Returns(
		&apicommoncharms.CharmInfo{
			URL:  wordpressURL.String(),
			Meta: &charm.Meta{Name: "wordpress"},
		},
		error(nil),
	)
	base := series.MustParseBaseFromString("ubuntu@18.04/stable")
	deployArgs := application.DeployArgs{
		CharmID: application.CharmID{
			URL:    wordpressURL,
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

	err := s.DeployBundleYAML(c, fmt.Sprintf(`
      series: jammy
      applications:
          wordpress:
              charm: ch:bionic/wordpress
              series: bionic
              num_units: 1
          mysql:
              charm: %s
              num_units: 1
      relations:
          - ["wordpress:db", "mysql:server"]
  `, mysqlDir.Path))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestErrorDeployingBundlesRequiringTrust(c *gc.C) {
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
		c.Assert(err, gc.Not(gc.IsNil))
		c.Assert(err.Error(), gc.Equals, expErr)
	}
}

func (s *BundleDeploySuite) TestDeployBundleWithChannel(c *gc.C) {
	withAllWatcher(s.fakeAPI)

	// The second charm from the bundle does not require trust so no
	// additional configuration should be injected
	ubURL := charm.MustParseURL("ch:ubuntu")
	withCharmRepoResolvable(s.fakeAPI, ubURL, "")

	withCharmDeployable(
		s.fakeAPI, ubURL, defaultBase,
		&charm.Meta{Name: "ubuntu"},
		nil, false, 0, nil, nil,
	)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "ubuntu",
		NumUnits:        1,
	}).Returns([]string{"ubuntu/0"}, error(nil))

	s.fakeAPI.Call("ListSpaces").Returns([]params.Space{{Name: "alpha", Id: "0"}}, error(nil))

	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "basic")
	err := s.runDeploy(c, bundlePath, "--channel", "edge")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployBundlesRequiringTrust(c *gc.C) {
	withAllWatcher(s.fakeAPI)

	inURL := charm.MustParseURL("ch:aws-integrator")
	withCharmRepoResolvable(s.fakeAPI, inURL, "jammy")
	withCharmRepoResolvable(s.fakeAPI, inURL, "")

	// The aws-integrator charm requires trust and since the operator passes
	// --trust we expect to see a "trust: true" config value in the yaml
	// config passed to deploy.
	//
	// As withCharmDeployable does not support passing a "ConfigYAML"
	// it's easier to just invoke it to set up all other calls and then
	// explicitly Deploy here.
	withCharmDeployable(
		s.fakeAPI, inURL, defaultBase,
		&charm.Meta{Name: "aws-integrator", Series: []string{"jammy"}},
		nil, false, 0, nil, nil,
	)

	origin := commoncharm.Origin{
		Source:       commoncharm.OriginCharmHub,
		Architecture: arch.DefaultArchitecture,
		Base:         series.MakeDefaultBase("ubuntu", "22.04"),
	}

	deployURL := *inURL
	deployURL.Series = "jammy"

	dArgs := application.DeployArgs{
		CharmID: application.CharmID{
			URL:    &deployURL,
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
	withCharmRepoResolvable(s.fakeAPI, ubURL, "jammy")
	withCharmRepoResolvable(s.fakeAPI, ubURL, "")
	withCharmDeployable(
		s.fakeAPI, ubURL, defaultBase,
		&charm.Meta{Name: "ubuntu", Series: []string{"jammy"}},
		nil, false, 0, nil, nil,
	)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "ubuntu",
		NumUnits:        1,
	}).Returns([]string{"ubuntu/0"}, error(nil))

	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "aws-integrator-trust-single")
	err := s.runDeploy(c, bundlePath, "--trust")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BundleDeploySuite) TestDeployBundleWithOffers(c *gc.C) {
	withAllWatcher(s.fakeAPI)

	inURL := charm.MustParseURL("ch:apache2")
	withCharmRepoResolvable(s.fakeAPI, inURL, "jammy")
	withCharmRepoResolvable(s.fakeAPI, inURL, "")

	withCharmDeployable(
		s.fakeAPI, inURL, defaultBase,
		&charm.Meta{Name: "apache2", Series: []string{"jammy"}},
		nil, false, 0, nil, nil,
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

	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "apache2-with-offers-legacy")
	err := s.runDeploy(c, bundlePath)
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(offerCallCount, gc.Equals, 2)
	c.Assert(grantOfferCallCount, gc.Equals, 2)
}

func (s *BundleDeploySuite) TestDeployBundleWithSAAS(c *gc.C) {
	withAllWatcher(s.fakeAPI)

	inURL := charm.MustParseURL("ch:wordpress")
	withCharmRepoResolvable(s.fakeAPI, inURL, "jammy")
	withCharmRepoResolvable(s.fakeAPI, inURL, "")

	withCharmDeployable(
		s.fakeAPI, inURL, defaultBase,
		&charm.Meta{Name: "wordpress", Series: []string{"jammy"}},
		nil, false, 0, nil, nil,
	)

	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)

	s.fakeAPI.Call("AddUnits", application.AddUnitsParams{
		ApplicationName: "wordpress",
		NumUnits:        1,
	}).Returns([]string{"wordpress/0"}, error(nil))

	s.fakeAPI.Call("GetConsumeDetails",
		"admin/default.mysql",
	).Returns(params.ConsumeOfferDetails{
		Offer: &params.ApplicationOfferDetails{
			OfferName: "mysql",
			OfferURL:  "admin/default.mysql",
		},
		Macaroon:  mac,
		AuthToken: "auth-token",
		ControllerInfo: &params.ExternalControllerInfo{
			ControllerTag: coretesting.ControllerTag.String(),
			Addrs:         []string{"192.168.1.0"},
			Alias:         "controller-alias",
			CACert:        coretesting.CACert,
		},
	}, nil)

	s.fakeAPI.Call("Consume",
		crossmodel.ConsumeApplicationArgs{
			Offer: params.ApplicationOfferDetails{
				OfferName: "mysql",
				OfferURL:  "test:admin/default.mysql",
			},
			ApplicationAlias: "mysql",
			Macaroon:         mac,
			AuthToken:        "auth-token",
			ControllerInfo: &crossmodel.ControllerInfo{
				ControllerTag: coretesting.ControllerTag,
				Alias:         "controller-alias",
				Addrs:         []string{"192.168.1.0"},
				CACert:        coretesting.CACert,
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

	bundlePath := testcharms.RepoWithSeries("bionic").ClonedBundleDirPath(c.MkDir(), "wordpress-with-saas")
	err = s.runDeploy(c, bundlePath)
	c.Assert(err, jc.ErrorIsNil)
}
