// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/gui"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

const (
	useDefaultKeys = true
	noKeysDefined  = false
)

type bootstrapSuite struct {
	coretesting.BaseSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	s.PatchValue(&juju.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	// Patch the function used to retrieve GUI archive info from simplestreams.
	s.PatchValue(bootstrap.GUIFetchMetadata, func(string, ...simplestreams.DataSource) ([]*gui.Metadata, error) {
		return nil, nil
	})
}

func (s *bootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestBootstrapNeedsSettings(c *gc.C) {
	env := newEnviron("bar", noKeysDefined, nil)
	s.setDummyStorage(c, env)
	fixEnv := func(key string, value interface{}) {
		cfg, err := env.Config().Apply(map[string]interface{}{
			key: value,
		})
		c.Assert(err, jc.ErrorIsNil)
		env.cfg = cfg
	}

	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "model configuration has no admin-secret")

	fixEnv("admin-secret", "whatever")
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "model configuration has no ca-cert")

	fixEnv("ca-cert", coretesting.CACert)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "model configuration has no ca-private-key")

	fixEnv("ca-private-key", coretesting.CAKey)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestBootstrapEmptyConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	env.args.AvailableTools = nil
	c.Assert(env.args, gc.DeepEquals, environs.BootstrapParams{})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	bootstrapCons := constraints.MustParse("cpu-cores=3 mem=7G")
	modelCons := constraints.MustParse("cpu-cores=2 mem=4G")
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		BootstrapConstraints: bootstrapCons,
		ModelConstraints:     modelCons,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.BootstrapConstraints, gc.DeepEquals, bootstrapCons)
	c.Assert(env.args.ModelConstraints, gc.DeepEquals, modelCons)
}

func (s *bootstrapSuite) TestBootstrapSpecifiedBootstrapSeries(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	cfg, err := env.Config().Apply(map[string]interface{}{
		"default-series": "wily",
	})
	c.Assert(err, jc.ErrorIsNil)
	env.cfg = cfg

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		BootstrapSeries: "trusty",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(env.bootstrapCount, gc.Equals, 1)
	c.Check(env.args.BootstrapSeries, gc.Equals, "trusty")
	c.Check(env.args.AvailableTools.AllSeries(), jc.SameContents, []string{"trusty"})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedPlacement(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	placement := "directive"
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{Placement: placement})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.Placement, gc.DeepEquals, placement)
}

func (s *bootstrapSuite) TestBootstrapImage(c *gc.C) {
	s.PatchValue(&series.HostSeries, func() string { return "precise" })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	metadataDir, metadata := createImageMetadata(c)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := bootstrapEnvironWithRegion{
		newEnviron("foo", useDefaultKeys, nil),
		simplestreams.CloudSpec{
			Region:   "nether",
			Endpoint: "hearnoretheir",
		},
	}
	s.setDummyStorage(c, env.bootstrapEnviron)

	bootstrapCons := constraints.MustParse("arch=amd64")
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		BootstrapImage:       "img-id",
		BootstrapSeries:      "precise",
		BootstrapConstraints: bootstrapCons,
		MetadataDir:          metadataDir,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.ImageMetadata, gc.HasLen, 1)
	c.Assert(env.args.ImageMetadata[0], jc.DeepEquals, &imagemetadata.ImageMetadata{
		Id:         "img-id",
		Arch:       "amd64",
		Version:    "12.04",
		RegionName: "nether",
		Endpoint:   "hearnoretheir",
		Stream:     "released",
	})
	c.Assert(env.instanceConfig.CustomImageMetadata, gc.HasLen, 2)
	c.Assert(env.instanceConfig.CustomImageMetadata[0], jc.DeepEquals, metadata[0])
	c.Assert(env.instanceConfig.CustomImageMetadata[1], jc.DeepEquals, env.args.ImageMetadata[0])
	c.Assert(env.instanceConfig.Constraints, jc.DeepEquals, bootstrapCons)
}

// TestBootstrapImageMetadataFromAllSources tests that we are looking for
// image metadata in all data sources available to environment.
// Abandoning look up too soon led to misleading bootstrap failures:
// Juju reported no images available for a particular configuration,
// despite image metadata in other data sources compatible with the same configuration as well.
// Related to bug#1560625.
func (s *bootstrapSuite) TestBootstrapImageMetadataFromAllSources(c *gc.C) {
	s.PatchValue(&series.HostSeries, func() string { return "raring" })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	// Ensure that we can find at least one image metadata
	// early on in the image metadata lookup.
	// We should continue looking despite it.
	metadataDir, _ := createImageMetadata(c)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := bootstrapEnvironWithRegion{
		newEnviron("foo", useDefaultKeys, nil),
		simplestreams.CloudSpec{
			Region:   "region",
			Endpoint: "endpoint",
		},
	}
	s.setDummyStorage(c, env.bootstrapEnviron)

	bootstrapCons := constraints.MustParse("arch=amd64")
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		BootstrapConstraints: bootstrapCons,
		MetadataDir:          metadataDir,
	})
	c.Assert(err, jc.ErrorIsNil)

	datasources, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	for _, source := range datasources {
		// make sure we looked in each and all...
		c.Assert(c.GetTestLog(), jc.Contains, fmt.Sprintf("image metadata in %s", source.Description()))
	}
}

func (s *bootstrapSuite) TestBootstrapNoToolsNonReleaseStream(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}

	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(environs.Environ, int, int, string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"agent-stream": "proposed"})
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	// bootstrap.Bootstrap leaves it to the provider to
	// locate bootstrap tools.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestBootstrapNoToolsDevelopmentConfig(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}

	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(environs.Environ, int, int, string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"development": true})
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	// bootstrap.Bootstrap leaves it to the provider to
	// locate bootstrap tools.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestSetBootstrapTools(c *gc.C) {
	availableVersions := []version.Binary{
		version.MustParseBinary("1.18.0-trusty-arm64"),
		version.MustParseBinary("1.18.1-trusty-arm64"),
		version.MustParseBinary("1.18.1.1-trusty-arm64"),
		version.MustParseBinary("1.18.1.2-trusty-arm64"),
		version.MustParseBinary("1.18.1.3-trusty-arm64"),
	}
	availableTools := make(tools.List, len(availableVersions))
	for i, v := range availableVersions {
		availableTools[i] = &tools.Tools{Version: v}
	}

	type test struct {
		currentVersion       version.Number
		expectedTools        version.Number
		expectedAgentVersion version.Number
	}
	tests := []test{{
		currentVersion:       version.MustParse("1.18.0"),
		expectedTools:        version.MustParse("1.18.0"),
		expectedAgentVersion: version.MustParse("1.18.1.3"),
	}, {
		currentVersion:       version.MustParse("1.18.1.4"),
		expectedTools:        version.MustParse("1.18.1.3"),
		expectedAgentVersion: version.MustParse("1.18.1.3"),
	}, {
		// build number is ignored unless major/minor don't
		// match the latest.
		currentVersion:       version.MustParse("1.18.1.2"),
		expectedTools:        version.MustParse("1.18.1.3"),
		expectedAgentVersion: version.MustParse("1.18.1.3"),
	}, {
		// If the current patch level exceeds whatever's in
		// the tools source (e.g. when bootstrapping from trunk)
		// then the latest available tools will be chosen.
		currentVersion:       version.MustParse("1.18.2"),
		expectedTools:        version.MustParse("1.18.1.3"),
		expectedAgentVersion: version.MustParse("1.18.1.3"),
	}}

	env := newEnviron("foo", useDefaultKeys, nil)
	for i, t := range tests {
		c.Logf("test %d: %+v", i, t)
		cfg, err := env.Config().Remove([]string{"agent-version"})
		c.Assert(err, jc.ErrorIsNil)
		err = env.SetConfig(cfg)
		c.Assert(err, jc.ErrorIsNil)
		s.PatchValue(&jujuversion.Current, t.currentVersion)
		bootstrapTools, err := bootstrap.SetBootstrapTools(env, availableTools)
		c.Assert(err, jc.ErrorIsNil)
		for _, tools := range bootstrapTools {
			c.Assert(tools.Version.Number, gc.Equals, t.expectedTools)
		}
		agentVersion, _ := env.Config().AgentVersion()
		c.Assert(agentVersion, gc.Equals, t.expectedAgentVersion)
	}
}

func (s *bootstrapSuite) TestBootstrapGUISuccessRemote(c *gc.C) {
	s.PatchValue(bootstrap.GUIFetchMetadata, func(stream string, sources ...simplestreams.DataSource) ([]*gui.Metadata, error) {
		c.Assert(stream, gc.Equals, gui.ReleasedStream)
		c.Assert(sources[0].Description(), gc.Equals, "gui simplestreams")
		c.Assert(sources[0].RequireSigned(), jc.IsTrue)
		return []*gui.Metadata{{
			Version:  version.MustParse("2.0.42"),
			FullPath: "https://1.2.3.4/juju-gui-2.0.42.tar.bz2",
			SHA256:   "hash-2.0.42",
			Size:     42,
		}, {
			Version:  version.MustParse("2.0.47"),
			FullPath: "https://1.2.3.4/juju-gui-2.0.47.tar.bz2",
			SHA256:   "hash-2.0.47",
			Size:     47,
		}}, nil
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := coretesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env, bootstrap.BootstrapParams{
		GUIDataSourceBaseURL: "https://1.2.3.4/gui/sources",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), jc.Contains, "Preparing for Juju GUI 2.0.42 release installation\n")

	// The most recent GUI release info has been stored.
	c.Assert(env.instanceConfig.GUI.URL, gc.Equals, "https://1.2.3.4/juju-gui-2.0.42.tar.bz2")
	c.Assert(env.instanceConfig.GUI.Version.String(), gc.Equals, "2.0.42")
	c.Assert(env.instanceConfig.GUI.Size, gc.Equals, int64(42))
	c.Assert(env.instanceConfig.GUI.SHA256, gc.Equals, "hash-2.0.42")
}

func (s *bootstrapSuite) TestBootstrapGUISuccessLocal(c *gc.C) {
	path := makeGUIArchive(c, "jujugui-2.2.0")
	s.PatchEnvironment("JUJU_GUI", path)
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := coretesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), jc.Contains, "Preparing for Juju GUI 2.2.0 installation from local archive\n")

	// Check GUI URL and version.
	c.Assert(env.instanceConfig.GUI.URL, gc.Equals, "file://"+path)
	c.Assert(env.instanceConfig.GUI.Version.String(), gc.Equals, "2.2.0")

	// Check GUI size.
	f, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	info, err := f.Stat()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.instanceConfig.GUI.Size, gc.Equals, info.Size())

	// Check GUI hash.
	h := sha256.New()
	_, err = io.Copy(h, f)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.instanceConfig.GUI.SHA256, gc.Equals, fmt.Sprintf("%x", h.Sum(nil)))
}

func (s *bootstrapSuite) TestBootstrapGUISuccessNoGUI(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := coretesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), jc.Contains, "Juju GUI installation has been disabled\n")
	c.Assert(env.instanceConfig.GUI, gc.IsNil)
}

func (s *bootstrapSuite) TestBootstrapGUINoStreams(c *gc.C) {
	s.PatchValue(bootstrap.GUIFetchMetadata, func(string, ...simplestreams.DataSource) ([]*gui.Metadata, error) {
		return nil, nil
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := coretesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env, bootstrap.BootstrapParams{
		GUIDataSourceBaseURL: "https://1.2.3.4/gui/sources",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), jc.Contains, "No available Juju GUI archives found\n")
	c.Assert(env.instanceConfig.GUI, gc.IsNil)
}

func (s *bootstrapSuite) TestBootstrapGUIStreamsFailure(c *gc.C) {
	s.PatchValue(bootstrap.GUIFetchMetadata, func(string, ...simplestreams.DataSource) ([]*gui.Metadata, error) {
		return nil, errors.New("bad wolf")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := coretesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env, bootstrap.BootstrapParams{
		GUIDataSourceBaseURL: "https://1.2.3.4/gui/sources",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), jc.Contains, "Unable to fetch Juju GUI info: bad wolf\n")
	c.Assert(env.instanceConfig.GUI, gc.IsNil)
}

func (s *bootstrapSuite) TestBootstrapGUIErrorNotFound(c *gc.C) {
	s.PatchEnvironment("JUJU_GUI", "/no/such/file")
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := coretesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), jc.Contains, `Cannot use Juju GUI at "/no/such/file": cannot open Juju GUI archive:`)
}

func (s *bootstrapSuite) TestBootstrapGUIErrorInvalidArchive(c *gc.C) {
	path := filepath.Join(c.MkDir(), "gui.bz2")
	err := ioutil.WriteFile(path, []byte("invalid"), 0666)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchEnvironment("JUJU_GUI", path)
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := coretesting.Context(c)
	err = bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), jc.Contains, fmt.Sprintf("Cannot use Juju GUI at %q: cannot read Juju GUI archive", path))
}

func (s *bootstrapSuite) TestBootstrapGUIErrorInvalidVersion(c *gc.C) {
	path := makeGUIArchive(c, "jujugui-invalid")
	s.PatchEnvironment("JUJU_GUI", path)
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := coretesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), jc.Contains, fmt.Sprintf(`Cannot use Juju GUI at %q: cannot parse version "invalid"`, path))
}

func (s *bootstrapSuite) TestBootstrapGUIErrorUnexpectedArchive(c *gc.C) {
	path := makeGUIArchive(c, "not-a-gui")
	s.PatchEnvironment("JUJU_GUI", path)
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := coretesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), jc.Contains, fmt.Sprintf("Cannot use Juju GUI at %q: cannot find Juju GUI version", path))
}

func makeGUIArchive(c *gc.C, dir string) string {
	if runtime.GOOS == "windows" {
		c.Skip("tar command not available")
	}
	target := filepath.Join(c.MkDir(), "gui.tar.bz2")
	source := c.MkDir()
	err := os.Mkdir(filepath.Join(source, dir), 0777)
	c.Assert(err, jc.ErrorIsNil)
	err = exec.Command("tar", "cjf", target, "-C", source, dir).Run()
	c.Assert(err, jc.ErrorIsNil)
	return target
}

// createImageMetadata creates some image metadata in a local directory.
func createImageMetadata(c *gc.C) (dir string, _ []*imagemetadata.ImageMetadata) {
	// Generate some image metadata.
	im := []*imagemetadata.ImageMetadata{{
		Id:         "1234",
		Arch:       "amd64",
		Version:    "13.04",
		RegionName: "region",
		Endpoint:   "endpoint",
	}}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	sourceDir := c.MkDir()
	sourceStor, err := filestorage.NewFileStorageWriter(sourceDir)
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata("raring", im, cloudSpec, sourceStor)
	c.Assert(err, jc.ErrorIsNil)
	return sourceDir, im
}

func (s *bootstrapSuite) TestBootstrapMetadata(c *gc.C) {
	environs.UnregisterImageDataSourceFunc("bootstrap metadata")

	metadataDir, metadata := createImageMetadata(c)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		MetadataDir: metadataDir,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(envtools.DefaultBaseURL, gc.Equals, metadataDir)

	datasources, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(datasources, gc.HasLen, 3)
	c.Assert(datasources[0].Description(), gc.Equals, "bootstrap metadata")
	// This data source does not require to contain signed data.
	// However, it may still contain it.
	// Since we will always try to read signed data first,
	// we want to be able to try to read this signed data
	// with a user provided key.
	// for this test, user provided key is empty.
	// Bugs #1542127, #1542131
	c.Assert(datasources[0].PublicSigningKey(), gc.Equals, "")
	c.Assert(env.instanceConfig, gc.NotNil)
	c.Assert(env.instanceConfig.CustomImageMetadata, gc.HasLen, 1)
	c.Assert(env.instanceConfig.CustomImageMetadata[0], gc.DeepEquals, metadata[0])
}

func (s *bootstrapSuite) TestPublicKeyEnvVar(c *gc.C) {
	path := filepath.Join(c.MkDir(), "key")
	ioutil.WriteFile(path, []byte("publickey"), 0644)
	s.PatchEnvironment("JUJU_STREAMS_PUBLICKEY_FILE", path)

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.instanceConfig.PublicImageSigningKey, gc.Equals, "publickey")
}

func (s *bootstrapSuite) TestBootstrapMetadataImagesMissing(c *gc.C) {
	environs.UnregisterImageDataSourceFunc("bootstrap metadata")

	noImagesDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(noImagesDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		MetadataDir: noImagesDir,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)

	datasources, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(datasources, gc.HasLen, 2)
	c.Assert(datasources[0].Description(), gc.Equals, "default cloud images")
	c.Assert(datasources[1].Description(), gc.Equals, "default ubuntu cloud images")
}

func (s *bootstrapSuite) setupBootstrapSpecificVersion(c *gc.C, clientMajor, clientMinor int, toolsVersion *version.Number) (error, int, version.Number) {
	currentVersion := jujuversion.Current
	currentVersion.Major = clientMajor
	currentVersion.Minor = clientMinor
	currentVersion.Tag = ""
	s.PatchValue(&jujuversion.Current, currentVersion)
	s.PatchValue(&series.HostSeries, func() string { return "trusty" })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	envtools.RegisterToolsDataSourceFunc("local storage", func(environs.Environ) (simplestreams.DataSource, error) {
		return storage.NewStorageSimpleStreamsDataSource("test datasource", env.storage, "tools", simplestreams.CUSTOM_CLOUD_DATA, false), nil
	})
	defer envtools.UnregisterToolsDataSourceFunc("local storage")

	toolsBinaries := []version.Binary{
		version.MustParseBinary("10.11.12-trusty-amd64"),
		version.MustParseBinary("10.11.13-trusty-amd64"),
		version.MustParseBinary("10.11-beta1-trusty-amd64"),
	}
	stream := "released"
	if toolsVersion != nil && toolsVersion.Tag != "" {
		stream = "devel"
		currentVersion.Tag = toolsVersion.Tag
	}
	_, err := envtesting.UploadFakeToolsVersions(env.storage, stream, stream, toolsBinaries...)
	c.Assert(err, jc.ErrorIsNil)

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		AgentVersion: toolsVersion,
	})
	vers, _ := env.cfg.AgentVersion()
	return err, env.bootstrapCount, vers
}

func (s *bootstrapSuite) TestBootstrapSpecificVersion(c *gc.C) {
	toolsVersion := version.MustParse("10.11.12")
	err, bootstrapCount, vers := s.setupBootstrapSpecificVersion(c, 10, 11, &toolsVersion)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bootstrapCount, gc.Equals, 1)
	c.Assert(vers, gc.DeepEquals, version.Number{
		Major: 10,
		Minor: 11,
		Patch: 12,
	})
}

func (s *bootstrapSuite) TestBootstrapSpecificVersionWithTag(c *gc.C) {
	toolsVersion := version.MustParse("10.11-beta1")
	err, bootstrapCount, vers := s.setupBootstrapSpecificVersion(c, 10, 11, &toolsVersion)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bootstrapCount, gc.Equals, 1)
	c.Assert(vers, gc.DeepEquals, version.Number{
		Major: 10,
		Minor: 11,
		Patch: 1,
		Tag:   "beta",
	})
}

func (s *bootstrapSuite) TestBootstrapNoSpecificVersion(c *gc.C) {
	// bootstrap with no specific version will use latest major.minor tools version.
	err, bootstrapCount, vers := s.setupBootstrapSpecificVersion(c, 10, 11, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bootstrapCount, gc.Equals, 1)
	c.Assert(vers, gc.DeepEquals, version.Number{
		Major: 10,
		Minor: 11,
		Patch: 13,
	})
}

func (s *bootstrapSuite) TestBootstrapSpecificVersionClientMinorMismatch(c *gc.C) {
	// bootstrap using a specified version only works if the patch number is different.
	// The bootstrap client major and minor versions need to match the tools asked for.
	toolsVersion := version.MustParse("10.11.12")
	err, bootstrapCount, _ := s.setupBootstrapSpecificVersion(c, 10, 1, &toolsVersion)
	c.Assert(strings.Replace(err.Error(), "\n", "", -1), gc.Matches, ".* no tools are available .*")
	c.Assert(bootstrapCount, gc.Equals, 0)
}

func (s *bootstrapSuite) TestBootstrapSpecificVersionClientMajorMismatch(c *gc.C) {
	// bootstrap using a specified version only works if the patch number is different.
	// The bootstrap client major and minor versions need to match the tools asked for.
	toolsVersion := version.MustParse("10.11.12")
	err, bootstrapCount, _ := s.setupBootstrapSpecificVersion(c, 1, 11, &toolsVersion)
	c.Assert(strings.Replace(err.Error(), "\n", "", -1), gc.Matches, ".* no tools are available .*")
	c.Assert(bootstrapCount, gc.Equals, 0)
}

type bootstrapEnviron struct {
	cfg              *config.Config
	environs.Environ // stub out all methods we don't care about.

	// The following fields are filled in when Bootstrap is called.
	bootstrapCount              int
	finalizerCount              int
	supportedArchitecturesCount int
	args                        environs.BootstrapParams
	instanceConfig              *instancecfg.InstanceConfig
	storage                     storage.Storage
}

func newEnviron(name string, defaultKeys bool, extraAttrs map[string]interface{}) *bootstrapEnviron {
	m := dummy.SampleConfig().Merge(extraAttrs)
	if !defaultKeys {
		m = m.Delete(
			"ca-cert",
			"ca-private-key",
			"admin-secret",
		)
	}
	m["name"] = name // overwrite name provided by dummy.SampleConfig
	cfg, err := config.New(config.NoDefaults, m)
	if err != nil {
		panic(fmt.Errorf("cannot create config from %#v: %v", m, err))
	}
	return &bootstrapEnviron{
		cfg: cfg,
	}
}

// setDummyStorage injects the local provider's fake storage implementation
// into the given environment, so that tests can manipulate storage as if it
// were real.
func (s *bootstrapSuite) setDummyStorage(c *gc.C, env *bootstrapEnviron) {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	env.storage = stor
	s.AddCleanup(func(c *gc.C) { closer.Close() })
}

func (e *bootstrapEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	e.bootstrapCount++
	e.args = args
	finalizer := func(_ environs.BootstrapContext, icfg *instancecfg.InstanceConfig) error {
		e.finalizerCount++
		e.instanceConfig = icfg
		return nil
	}
	series := series.HostSeries()
	if args.BootstrapSeries != "" {
		series = args.BootstrapSeries
	}
	return &environs.BootstrapResult{Arch: arch.HostArch(), Series: series, Finalize: finalizer}, nil
}

func (e *bootstrapEnviron) Config() *config.Config {
	return e.cfg
}

func (e *bootstrapEnviron) SetConfig(cfg *config.Config) error {
	e.cfg = cfg
	return nil
}

func (e *bootstrapEnviron) Storage() storage.Storage {
	return e.storage
}

func (e *bootstrapEnviron) SupportedArchitectures() ([]string, error) {
	e.supportedArchitecturesCount++
	return []string{arch.AMD64, arch.ARM64}, nil
}

func (e *bootstrapEnviron) ConstraintsValidator() (constraints.Validator, error) {
	return constraints.NewValidator(), nil
}

type bootstrapEnvironWithRegion struct {
	*bootstrapEnviron
	region simplestreams.CloudSpec
}

func (e bootstrapEnvironWithRegion) Region() (simplestreams.CloudSpec, error) {
	return e.region, nil
}
