// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/arch"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/common/mocks"
	apiservertesting "github.com/juju/juju/v2/apiserver/testing"
	"github.com/juju/juju/v2/core/network"
	coreos "github.com/juju/juju/v2/core/os"
	"github.com/juju/juju/v2/environs"
	"github.com/juju/juju/v2/environs/config"
	envtools "github.com/juju/juju/v2/environs/tools"
	"github.com/juju/juju/v2/rpc/params"
	"github.com/juju/juju/v2/state/binarystorage"
	coretesting "github.com/juju/juju/v2/testing"
	coretools "github.com/juju/juju/v2/tools"
	jujuversion "github.com/juju/juju/v2/version"
)

type getToolsSuite struct {
	entityFinder *mocks.MockToolsFindEntity
	configGetter *mocks.MockEnvironConfigGetter
	toolsFinder  *mocks.MockToolsFinder

	machine0 *mocks.MockAgentTooler
}

var _ = gc.Suite(&getToolsSuite{})

func (s *getToolsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.entityFinder = mocks.NewMockToolsFindEntity(ctrl)
	s.configGetter = mocks.NewMockEnvironConfigGetter(ctrl)
	s.toolsFinder = mocks.NewMockToolsFinder(ctrl)

	s.machine0 = mocks.NewMockAgentTooler(ctrl)

	return ctrl
}

func (s *getToolsSuite) TestTools(c *gc.C) {
	defer s.setup(c).Finish()

	getCanRead := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == names.NewMachineTag("0") || tag == names.NewMachineTag("42")
		}, nil
	}
	tg := common.NewToolsGetter(
		s.entityFinder, s.configGetter, nil,
		nil, s.toolsFinder, getCanRead,
	)
	c.Assert(tg, gc.NotNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-0"},
			{Tag: "machine-1"},
			{Tag: "machine-42"},
		},
	}

	current := coretesting.CurrentVersion(c)
	configAttrs := map[string]interface{}{
		"name":                 "some-name",
		"type":                 "some-type",
		"uuid":                 coretesting.ModelTag.Id(),
		config.AgentVersionKey: current.Number.String(),
	}
	config, err := config.New(config.NoDefaults, configAttrs)
	c.Assert(err, jc.ErrorIsNil)
	s.configGetter.EXPECT().ModelConfig().Return(config, nil)

	s.entityFinder.EXPECT().FindEntity(names.NewMachineTag("0")).Return(s.machine0, nil)
	s.machine0.EXPECT().AgentTools().Return(&coretools.Tools{Version: current}, nil)
	s.toolsFinder.EXPECT().FindTools(params.FindToolsParams{
		Number:       current.Number,
		MajorVersion: -1,
		MinorVersion: -1,
		OSType:       current.Release,
		Arch:         current.Arch,
	}).Return(params.FindToolsResult{List: coretools.List{{
		Version: current,
		URL:     "tools:" + current.String(),
	}}}, nil)

	s.entityFinder.EXPECT().FindEntity(names.NewMachineTag("42")).Return(nil, apiservertesting.NotFoundError("machine 42"))

	result, err := tg.Tools(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].ToolsList, gc.HasLen, 1)
	tools := result.Results[0].ToolsList[0]
	c.Assert(tools.Version, gc.DeepEquals, current)
	c.Assert(tools.URL, gc.Equals, "tools:"+current.String())
	c.Assert(result.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.NotFoundError("machine 42"))
}

func (s *getToolsSuite) TestSeriesTools(c *gc.C) {
	defer s.setup(c).Finish()

	getCanRead := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == names.NewMachineTag("0")
		}, nil
	}
	tg := common.NewToolsGetter(
		s.entityFinder, s.configGetter, nil,
		nil, s.toolsFinder, getCanRead,
	)
	c.Assert(tg, gc.NotNil)

	current := coretesting.CurrentVersion(c)
	currentCopy := current
	currentCopy.Release = coretesting.HostSeries(c)
	configAttrs := map[string]interface{}{
		"name":                 "some-name",
		"type":                 "some-type",
		"uuid":                 coretesting.ModelTag.Id(),
		config.AgentVersionKey: currentCopy.Number.String(),
	}
	config, err := config.New(config.NoDefaults, configAttrs)
	c.Assert(err, jc.ErrorIsNil)
	s.configGetter.EXPECT().ModelConfig().Return(config, nil)

	s.entityFinder.EXPECT().FindEntity(names.NewMachineTag("0")).Return(s.machine0, nil)
	s.machine0.EXPECT().AgentTools().Return(&coretools.Tools{Version: currentCopy}, nil)
	s.toolsFinder.EXPECT().FindTools(params.FindToolsParams{
		Number:       currentCopy.Number,
		MajorVersion: -1,
		MinorVersion: -1,
		Series:       currentCopy.Release,
		Arch:         currentCopy.Arch,
	}).Return(params.FindToolsResult{List: coretools.List{{
		Version: current,
		URL:     "tools:" + current.String(),
	}}}, nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-0"},
		}}
	result, err := tg.Tools(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].ToolsList, gc.HasLen, 1)
	tools := result.Results[0].ToolsList[0]
	c.Assert(tools.Version, gc.DeepEquals, current)
	c.Assert(tools.URL, gc.Equals, "tools:"+current.String())
}

func (s *getToolsSuite) TestToolsError(c *gc.C) {
	defer s.setup(c).Finish()

	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	tg := common.NewToolsGetter(
		s.entityFinder, s.configGetter, nil,
		nil, s.toolsFinder, getCanRead,
	)
	c.Assert(tg, gc.NotNil)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-42"}},
	}
	result, err := tg.Tools(args)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(result.Results, gc.HasLen, 1)
}

type setToolsSuite struct {
	entityFinder *mocks.MockToolsFindEntity

	machine0 *mocks.MockAgentTooler
}

var _ = gc.Suite(&setToolsSuite{})

func (s *setToolsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.entityFinder = mocks.NewMockToolsFindEntity(ctrl)

	s.machine0 = mocks.NewMockAgentTooler(ctrl)

	return ctrl
}

func (s *setToolsSuite) TestSetTools(c *gc.C) {
	defer s.setup(c).Finish()

	getCanWrite := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == names.NewMachineTag("0") || tag == names.NewMachineTag("42")
		}, nil
	}
	ts := common.NewToolsSetter(s.entityFinder, getCanWrite)
	c.Assert(ts, gc.NotNil)

	current := coretesting.CurrentVersion(c)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: "machine-0",
			Tools: &params.Version{
				Version: current,
			},
		}, {
			Tag: "machine-1",
			Tools: &params.Version{
				Version: current,
			},
		}, {
			Tag: "machine-42",
			Tools: &params.Version{
				Version: current,
			},
		}},
	}

	s.entityFinder.EXPECT().FindEntity(names.NewMachineTag("0")).Return(s.machine0, nil)
	s.machine0.EXPECT().SetAgentVersion(current).Return(nil)

	s.entityFinder.EXPECT().FindEntity(names.NewMachineTag("42")).Return(nil, apiservertesting.NotFoundError("machine 42"))

	result, err := ts.SetTools(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.NotFoundError("machine 42"))
}

func (s *setToolsSuite) TestToolsSetError(c *gc.C) {
	defer s.setup(c).Finish()

	getCanWrite := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	ts := common.NewToolsSetter(s.entityFinder, getCanWrite)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: "machine-42",
			Tools: &params.Version{
				Version: coretesting.CurrentVersion(c),
			},
		}},
	}
	result, err := ts.SetTools(args)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(result.Results, gc.HasLen, 1)
}

type findToolsSuite struct {
	jujutesting.IsolationSuite

	toolsStorageGetter *mocks.MockToolsStorageGetter
	urlGetter          *mocks.MockToolsURLGetter
	storage            *mocks.MockStorageCloser

	bootstrapEnviron *mocks.MockBootstrapEnviron
	newEnviron       func() (environs.BootstrapEnviron, error)
}

var _ = gc.Suite(&findToolsSuite{})

func (s *findToolsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *findToolsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.toolsStorageGetter = mocks.NewMockToolsStorageGetter(ctrl)
	s.urlGetter = mocks.NewMockToolsURLGetter(ctrl)
	s.urlGetter.EXPECT().ToolsURLs(gomock.Any()).DoAndReturn(func(arg version.Binary) ([]string, error) {
		return []string{fmt.Sprintf("tools:%v", arg)}, nil
	}).AnyTimes()

	s.bootstrapEnviron = mocks.NewMockBootstrapEnviron(ctrl)
	s.storage = mocks.NewMockStorageCloser(ctrl)
	s.newEnviron = func() (environs.BootstrapEnviron, error) {
		return s.bootstrapEnviron, nil
	}
	return ctrl
}

func (s *findToolsSuite) expectMatchingStorageTools(c *gc.C, storageMetadata []binarystorage.Metadata, err error) {
	s.toolsStorageGetter.EXPECT().ToolsStorage().Return(s.storage, nil)
	s.storage.EXPECT().AllMetadata().Return(storageMetadata, err)
	s.storage.EXPECT().Close().Return(nil)
}

func (s *findToolsSuite) expectBootstrapEnvionConfig(c *gc.C) {
	current := coretesting.CurrentVersion(c)
	configAttrs := map[string]interface{}{
		"name":                 "some-name",
		"type":                 "some-type",
		"uuid":                 coretesting.ModelTag.Id(),
		config.AgentVersionKey: current.Number.String(),
		"development":          false,
	}
	config, err := config.New(config.NoDefaults, configAttrs)
	c.Assert(err, jc.ErrorIsNil)

	s.bootstrapEnviron.EXPECT().Config().Return(config)
}

func (s *findToolsSuite) TestFindTools(c *gc.C) {
	defer s.setup(c).Finish()

	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: version.MustParseBinary("123.456.0-windows-alpha"),
			Size:    2048,
			SHA256:  "badf00d",
		},
		&coretools.Tools{
			Version: version.MustParseBinary("123.456.1-windows-alpha"),
		},
	}
	s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
		c.Assert(major, gc.Equals, 123)
		c.Assert(minor, gc.Equals, 456)
		c.Assert(streams, gc.DeepEquals, []string{"released"})
		c.Assert(filter.OSType, gc.Equals, "windows")
		c.Assert(filter.Arch, gc.Equals, "alpha")
		return envtoolsList, nil
	})

	storageMetadata := []binarystorage.Metadata{{
		Version: "123.456.0-windows-alpha",
		Size:    1024,
		SHA256:  "feedface",
	}}
	s.expectMatchingStorageTools(c, storageMetadata, nil)
	s.expectBootstrapEnvionConfig(c)

	toolsFinder := common.NewToolsFinder(
		nil, s.toolsStorageGetter, s.urlGetter, s.newEnviron,
	)

	result, err := toolsFinder.FindTools(params.FindToolsParams{
		MajorVersion: 123,
		MinorVersion: 456,
		OSType:       "windows",
		Arch:         "alpha",
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, jc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: version.MustParseBinary(storageMetadata[0].Version),
			Size:    storageMetadata[0].Size,
			SHA256:  storageMetadata[0].SHA256,
			URL:     "tools:" + storageMetadata[0].Version,
		},
		&coretools.Tools{
			Version: version.MustParseBinary("123.456.1-windows-alpha"),
			URL:     "tools:123.456.1-windows-alpha",
		},
	})
}

func (s *findToolsSuite) TestFindToolsRequestAgentStream(c *gc.C) {
	defer s.setup(c).Finish()

	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: version.MustParseBinary("123.456.0-windows-alpha"),
			Size:    2048,
			SHA256:  "badf00d",
		},
		&coretools.Tools{
			Version: version.MustParseBinary("123.456.1-windows-alpha"),
		},
	}
	s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
		c.Assert(major, gc.Equals, 123)
		c.Assert(minor, gc.Equals, 456)
		c.Assert(streams, gc.DeepEquals, []string{"pretend"})
		c.Assert(filter.OSType, gc.Equals, "windows")
		c.Assert(filter.Arch, gc.Equals, "alpha")
		return envtoolsList, nil
	})

	storageMetadata := []binarystorage.Metadata{{
		Version: "123.456.0-windows-alpha",
		Size:    1024,
		SHA256:  "feedface",
	}}
	s.expectMatchingStorageTools(c, storageMetadata, nil)
	s.expectBootstrapEnvionConfig(c)

	toolsFinder := common.NewToolsFinder(
		nil, s.toolsStorageGetter, s.urlGetter, s.newEnviron,
	)
	result, err := toolsFinder.FindTools(params.FindToolsParams{
		MajorVersion: 123,
		MinorVersion: 456,
		OSType:       "windows",
		Arch:         "alpha",
		AgentStream:  "pretend",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, jc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: version.MustParseBinary(storageMetadata[0].Version),
			Size:    storageMetadata[0].Size,
			SHA256:  storageMetadata[0].SHA256,
			URL:     "tools:" + storageMetadata[0].Version,
		},
		&coretools.Tools{
			Version: version.MustParseBinary("123.456.1-windows-alpha"),
			URL:     "tools:123.456.1-windows-alpha",
		},
	})
}

// TODO(juju4) - remove
func (s *findToolsSuite) TestFindToolsOldAgent(c *gc.C) {
	defer s.setup(c).Finish()

	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: version.MustParseBinary("2.8.9-focal-amd64"),
			Size:    2048,
			SHA256:  "badf00d",
		},
	}

	s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
		c.Assert(major, gc.Equals, 2)
		c.Assert(minor, gc.Equals, 8)
		c.Assert(streams, gc.DeepEquals, []string{"released"})
		c.Assert(filter.OSType, gc.Equals, "ubuntu")
		c.Assert(filter.Arch, gc.Equals, "amd64")
		return envtoolsList, nil
	})

	s.expectMatchingStorageTools(c, []binarystorage.Metadata{{
		Version: "2.8.9-win2012-amd64",
		Size:    1024,
		SHA256:  "feedface",
	}}, nil)
	s.expectBootstrapEnvionConfig(c)

	toolsFinder := common.NewToolsFinder(
		nil, s.toolsStorageGetter, s.urlGetter, s.newEnviron,
	)
	result, err := toolsFinder.FindTools(params.FindToolsParams{
		Number:       version.MustParse("2.8.9"),
		MajorVersion: 2,
		MinorVersion: 8,
		OSType:       "ubuntu",
		Arch:         "amd64",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, jc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: version.MustParseBinary("2.8.9-ubuntu-amd64"),
			Size:    2048,
			SHA256:  "badf00d",
			URL:     "tools:2.8.9-ubuntu-amd64",
		},
	})
}

// TODO(juju4) - remove
func (s *findToolsSuite) TestFindToolsOldAgentRequestAgentStream(c *gc.C) {
	defer s.setup(c).Finish()

	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: version.MustParseBinary("2.8.9-focal-amd64"),
			Size:    2048,
			SHA256:  "badf00d",
		},
	}

	s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
		c.Assert(major, gc.Equals, 2)
		c.Assert(minor, gc.Equals, 8)
		c.Assert(streams, gc.DeepEquals, []string{"pretend"})
		c.Assert(filter.OSType, gc.Equals, "ubuntu")
		c.Assert(filter.Arch, gc.Equals, "amd64")
		return envtoolsList, nil
	})

	s.expectMatchingStorageTools(c, []binarystorage.Metadata{{
		Version: "2.8.9-win2012-amd64",
		Size:    1024,
		SHA256:  "feedface",
	}}, nil)
	s.expectBootstrapEnvionConfig(c)

	toolsFinder := common.NewToolsFinder(
		nil, s.toolsStorageGetter, s.urlGetter, s.newEnviron,
	)
	result, err := toolsFinder.FindTools(params.FindToolsParams{
		Number:       version.MustParse("2.8.9"),
		MajorVersion: 2,
		MinorVersion: 8,
		OSType:       "ubuntu",
		Arch:         "amd64",
		AgentStream:  "pretend",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, jc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: version.MustParseBinary("2.8.9-ubuntu-amd64"),
			Size:    2048,
			SHA256:  "badf00d",
			URL:     "tools:2.8.9-ubuntu-amd64",
		},
	})
}

func (s *findToolsSuite) TestFindToolsNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, stream []string, filter coretools.Filter) (list coretools.List, err error) {
		return nil, errors.NotFoundf("tools")
	})

	s.expectMatchingStorageTools(c, []binarystorage.Metadata{}, nil)
	s.expectBootstrapEnvionConfig(c)

	toolsFinder := common.NewToolsFinder(nil, s.toolsStorageGetter, nil, s.newEnviron)
	result, err := toolsFinder.FindTools(params.FindToolsParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *findToolsSuite) TestFindToolsExactInStorage(c *gc.C) {
	defer s.setup(c).Finish()

	storageMetadata := []binarystorage.Metadata{
		{Version: "1.22-beta1-ubuntu-amd64"},
		{Version: "1.22.0-ubuntu-amd64"},
	}
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })

	s.expectMatchingStorageTools(c, storageMetadata, nil)
	s.PatchValue(&jujuversion.Current, version.MustParseBinary("1.22-beta1-ubuntu-amd64").Number)
	s.testFindToolsExact(c, true, true)

	s.expectMatchingStorageTools(c, storageMetadata, nil)
	s.PatchValue(&jujuversion.Current, version.MustParseBinary("1.22.0-ubuntu-amd64").Number)
	s.testFindToolsExact(c, true, false)
}

func (s *findToolsSuite) TestFindToolsExactNotInStorage(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectMatchingStorageTools(c, []binarystorage.Metadata{}, nil)
	s.expectBootstrapEnvionConfig(c)
	s.PatchValue(&jujuversion.Current, version.MustParse("1.22-beta1"))
	s.testFindToolsExact(c, false, true)

	s.expectMatchingStorageTools(c, []binarystorage.Metadata{}, nil)
	s.expectBootstrapEnvionConfig(c)
	s.PatchValue(&jujuversion.Current, version.MustParse("1.22.0"))
	s.testFindToolsExact(c, false, false)
}

func (s *findToolsSuite) testFindToolsExact(c *gc.C, inStorage bool, develVersion bool) {
	var called bool
	current := coretesting.CurrentVersion(c)
	s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, stream []string, filter coretools.Filter) (list coretools.List, err error) {
		called = true
		c.Assert(filter.Number, gc.Equals, jujuversion.Current)
		c.Assert(filter.OSType, gc.Equals, current.Release)
		c.Assert(filter.Arch, gc.Equals, arch.HostArch())
		if develVersion {
			c.Assert(stream, gc.DeepEquals, []string{"devel", "proposed", "released"})
		} else {
			c.Assert(stream, gc.DeepEquals, []string{"released"})
		}
		return nil, errors.NotFoundf("tools")
	})
	toolsFinder := common.NewToolsFinder(nil, s.toolsStorageGetter, s.urlGetter, s.newEnviron)
	result, err := toolsFinder.FindTools(params.FindToolsParams{
		Number:       jujuversion.Current,
		MajorVersion: -1,
		MinorVersion: -1,
		OSType:       current.Release,
		Arch:         arch.HostArch(),
	})
	c.Assert(err, jc.ErrorIsNil)
	if inStorage {
		c.Assert(result.Error, gc.IsNil)
		c.Assert(called, jc.IsFalse)
	} else {
		c.Assert(result.Error, gc.ErrorMatches, "tools not found")
		c.Assert(called, jc.IsTrue)
	}
}

func (s *findToolsSuite) TestFindToolsToolsStorageError(c *gc.C) {
	defer s.setup(c).Finish()

	var called bool
	s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, stream []string, filter coretools.Filter) (list coretools.List, err error) {
		called = true
		return nil, errors.NotFoundf("tools")
	})

	s.expectMatchingStorageTools(c, nil, errors.New("AllMetadata failed"))

	toolsFinder := common.NewToolsFinder(nil, s.toolsStorageGetter, s.urlGetter, s.newEnviron)
	result, err := toolsFinder.FindTools(params.FindToolsParams{
		MajorVersion: 1,
		MinorVersion: -1,
	})
	c.Assert(err, jc.ErrorIsNil)
	// ToolsStorage errors always cause FindTools to bail. Only
	// if AllMetadata succeeds but returns nothing that matches
	// do we continue on to searching simplestreams.
	c.Assert(result.Error, gc.ErrorMatches, "AllMetadata failed")
	c.Assert(called, jc.IsFalse)
}

type getUrlSuite struct {
	apiHostPortsGetter *mocks.MockAPIHostPortsForAgentsGetter
}

func (s *getUrlSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiHostPortsGetter = mocks.NewMockAPIHostPortsForAgentsGetter(ctrl)
	return ctrl
}

func (s *getUrlSuite) TestToolsURLGetterNoAPIHostPorts(c *gc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().APIHostPortsForAgents().Return(nil, nil)

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	_, err := g.ToolsURLs(coretesting.CurrentVersion(c))
	c.Assert(err, gc.ErrorMatches, "no suitable API server address to pick from")
}

func (s *getUrlSuite) TestToolsURLGetterAPIHostPortsError(c *gc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().APIHostPortsForAgents().Return(nil, errors.New("oh noes"))

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	_, err := g.ToolsURLs(coretesting.CurrentVersion(c))
	c.Assert(err, gc.ErrorMatches, "oh noes")
}

func (s *getUrlSuite) TestToolsURLGetter(c *gc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().APIHostPortsForAgents().Return([]network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}, nil)

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	current := coretesting.CurrentVersion(c)
	urls, err := g.ToolsURLs(current)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(urls, jc.DeepEquals, []string{
		"https://0.1.2.3:1234/model/my-uuid/tools/" + current.String(),
	})
}
