// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/os/v2"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/binarystorage"
)

type getToolsSuite struct {
	modelAgentService *mocks.MockModelAgentService
	toolsFinder       *mocks.MockToolsFinder
	store             *mocks.MockObjectStore
}

var _ = gc.Suite(&getToolsSuite{})

func (s *getToolsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelAgentService = mocks.NewMockModelAgentService(ctrl)
	s.toolsFinder = mocks.NewMockToolsFinder(ctrl)
	s.store = mocks.NewMockObjectStore(ctrl)

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
		s.modelAgentService, nil,
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

	agentBinary := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
	}
	s.modelAgentService.EXPECT().GetMachineTargetAgentVersion(gomock.Any(), machine.Name("0")).Return(agentBinary, nil)
	s.modelAgentService.EXPECT().GetMachineTargetAgentVersion(gomock.Any(), machine.Name("42")).
		Return(coreagentbinary.Version{}, machineerrors.MachineNotFound)

	current := coretesting.CurrentVersion()
	s.toolsFinder.EXPECT().FindAgents(context.Background(), common.FindAgentsParams{
		Number: current.Number,
		OSType: os.Ubuntu.String(),
		Arch:   current.Arch,
	}).Return(coretools.List{{
		Version: current,
		URL:     "tools:" + current.String(),
	}}, nil)

	result, err := tg.Tools(context.Background(), args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].ToolsList, gc.HasLen, 1)
	tools := result.Results[0].ToolsList[0]
	c.Assert(tools.Version, gc.DeepEquals, current)
	c.Assert(tools.URL, gc.Equals, "tools:"+current.String())
	c.Assert(result.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.NotFoundError(`"machine 42"`))
}

func (s *getToolsSuite) TestToolsError(c *gc.C) {
	defer s.setup(c).Finish()

	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	tg := common.NewToolsGetter(
		s.modelAgentService, nil,
		nil, s.toolsFinder, getCanRead,
	)
	c.Assert(tg, gc.NotNil)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-42"}},
	}
	result, err := tg.Tools(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(result.Results, gc.HasLen, 1)
}

type findToolsSuite struct {
	jujutesting.IsolationSuite

	toolsStorageGetter *mocks.MockToolsStorageGetter
	urlGetter          *mocks.MockToolsURLGetter
	storage            *mocks.MockStorageCloser
	store              *mocks.MockObjectStore

	bootstrapEnviron *mocks.MockBootstrapEnviron
	newEnviron       func(context.Context) (environs.BootstrapEnviron, error)
}

var _ = gc.Suite(&findToolsSuite{})

func (s *findToolsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *findToolsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.toolsStorageGetter = mocks.NewMockToolsStorageGetter(ctrl)
	s.urlGetter = mocks.NewMockToolsURLGetter(ctrl)
	s.urlGetter.EXPECT().ToolsURLs(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ controller.Config, arg semversion.Binary) ([]string, error) {
		return []string{fmt.Sprintf("tools:%v", arg)}, nil
	}).AnyTimes()

	s.bootstrapEnviron = mocks.NewMockBootstrapEnviron(ctrl)
	s.storage = mocks.NewMockStorageCloser(ctrl)
	s.newEnviron = func(_ context.Context) (environs.BootstrapEnviron, error) {
		return s.bootstrapEnviron, nil
	}
	s.store = mocks.NewMockObjectStore(ctrl)
	return ctrl
}

func (s *findToolsSuite) expectMatchingStorageTools(storageMetadata []binarystorage.Metadata, err error) {
	s.toolsStorageGetter.EXPECT().ToolsStorage(gomock.Any()).Return(s.storage, nil)
	s.storage.EXPECT().AllMetadata().Return(storageMetadata, err)
	s.storage.EXPECT().Close().Return(nil)
}

func (s *findToolsSuite) expectBootstrapEnvironConfig(c *gc.C) {
	current := coretesting.CurrentVersion()
	configAttrs := map[string]interface{}{
		"name":                 "some-name",
		"type":                 "some-type",
		"uuid":                 coretesting.ModelTag.Id(),
		config.AgentVersionKey: current.Number.String(),
		"secret-backend":       "auto",
	}
	cfg, err := config.New(config.NoDefaults, configAttrs)
	c.Assert(err, jc.ErrorIsNil)

	s.bootstrapEnviron.EXPECT().Config().Return(cfg)
}

func (s *findToolsSuite) TestFindToolsMatchMajor(c *gc.C) {
	defer s.setup(c).Finish()

	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.0-windows-alpha"),
			Size:    2048,
			SHA256:  "badf00d",
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-windows-alpha"),
		},
	}
	s.PatchValue(common.EnvtoolsFindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
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
	}, {
		Version: "666.456.0-windows-alpha",
		Size:    1024,
		SHA256:  "feedface666",
	}}
	s.expectMatchingStorageTools(storageMetadata, nil)
	s.expectBootstrapEnvironConfig(c)

	toolsFinder := common.NewToolsFinder(controllerConfigService{}, s.toolsStorageGetter, s.urlGetter, s.newEnviron, s.store)

	result, err := toolsFinder.FindAgents(context.Background(), common.FindAgentsParams{
		MajorVersion: 123,
		MinorVersion: 456,
		OSType:       "windows",
		Arch:         "alpha",
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary(storageMetadata[0].Version),
			Size:    storageMetadata[0].Size,
			SHA256:  storageMetadata[0].SHA256,
			URL:     "tools:" + storageMetadata[0].Version,
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-windows-alpha"),
			URL:     "tools:123.456.1-windows-alpha",
		},
	})
}

func (s *findToolsSuite) TestFindToolsRequestAgentStream(c *gc.C) {
	defer s.setup(c).Finish()

	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.0-windows-alpha"),
			Size:    2048,
			SHA256:  "badf00d",
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-windows-alpha"),
		},
	}
	s.PatchValue(common.EnvtoolsFindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
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
	s.expectMatchingStorageTools(storageMetadata, nil)
	s.expectBootstrapEnvironConfig(c)

	toolsFinder := common.NewToolsFinder(controllerConfigService{}, s.toolsStorageGetter, s.urlGetter, s.newEnviron, s.store)
	result, err := toolsFinder.FindAgents(context.Background(), common.FindAgentsParams{
		MajorVersion: 123,
		MinorVersion: 456,
		OSType:       "windows",
		Arch:         "alpha",
		AgentStream:  "pretend",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary(storageMetadata[0].Version),
			Size:    storageMetadata[0].Size,
			SHA256:  storageMetadata[0].SHA256,
			URL:     "tools:" + storageMetadata[0].Version,
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-windows-alpha"),
			URL:     "tools:123.456.1-windows-alpha",
		},
	})
}

func (s *findToolsSuite) TestFindToolsNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.PatchValue(common.EnvtoolsFindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, stream []string, filter coretools.Filter) (list coretools.List, err error) {
		return nil, errors.NotFoundf("tools")
	})

	s.expectMatchingStorageTools([]binarystorage.Metadata{}, nil)
	s.expectBootstrapEnvironConfig(c)

	toolsFinder := common.NewToolsFinder(controllerConfigService{}, s.toolsStorageGetter, nil, s.newEnviron, s.store)
	_, err := toolsFinder.FindAgents(context.Background(), common.FindAgentsParams{})
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *findToolsSuite) TestFindToolsExactInStorage(c *gc.C) {
	defer s.setup(c).Finish()

	storageMetadata := []binarystorage.Metadata{
		{Version: "1.22-beta1-ubuntu-amd64"},
		{Version: "1.22.0-ubuntu-amd64"},
	}
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	s.expectMatchingStorageTools(storageMetadata, nil)
	s.PatchValue(&jujuversion.Current, semversion.MustParseBinary("1.22-beta1-ubuntu-amd64").Number)
	s.testFindToolsExact(c, true, true)

	s.expectMatchingStorageTools(storageMetadata, nil)
	s.PatchValue(&jujuversion.Current, semversion.MustParseBinary("1.22.0-ubuntu-amd64").Number)
	s.testFindToolsExact(c, true, false)
}

func (s *findToolsSuite) TestFindToolsExactNotInStorage(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectMatchingStorageTools([]binarystorage.Metadata{}, nil)
	s.expectBootstrapEnvironConfig(c)
	s.PatchValue(&jujuversion.Current, semversion.MustParse("1.22-beta1"))
	s.testFindToolsExact(c, false, true)

	s.expectMatchingStorageTools([]binarystorage.Metadata{}, nil)
	s.expectBootstrapEnvironConfig(c)
	s.PatchValue(&jujuversion.Current, semversion.MustParse("1.22.0"))
	s.testFindToolsExact(c, false, false)
}

func (s *findToolsSuite) testFindToolsExact(c *gc.C, inStorage bool, develVersion bool) {
	var called bool
	current := coretesting.CurrentVersion()
	s.PatchValue(common.EnvtoolsFindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, stream []string, filter coretools.Filter) (list coretools.List, err error) {
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
	toolsFinder := common.NewToolsFinder(controllerConfigService{}, s.toolsStorageGetter, s.urlGetter, s.newEnviron, s.store)
	_, err := toolsFinder.FindAgents(context.Background(), common.FindAgentsParams{
		Number: jujuversion.Current,
		OSType: current.Release,
		Arch:   arch.HostArch(),
	})
	if inStorage {
		c.Assert(err, gc.IsNil)
		c.Assert(called, jc.IsFalse)
	} else {
		c.Assert(err, gc.ErrorMatches, "tools not found")
		c.Assert(called, jc.IsTrue)
	}
}

func (s *findToolsSuite) TestFindToolsToolsStorageError(c *gc.C) {
	defer s.setup(c).Finish()

	var called bool
	s.PatchValue(common.EnvtoolsFindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, stream []string, filter coretools.Filter) (list coretools.List, err error) {
		called = true
		return nil, errors.NotFoundf("tools")
	})

	s.expectMatchingStorageTools(nil, errors.New("AllMetadata failed"))

	toolsFinder := common.NewToolsFinder(controllerConfigService{}, s.toolsStorageGetter, s.urlGetter, s.newEnviron, s.store)
	_, err := toolsFinder.FindAgents(context.Background(), common.FindAgentsParams{})
	// ToolsStorage errors always cause FindAgents to bail. Only
	// if AllMetadata succeeds but returns nothing that matches
	// do we continue on to searching simplestreams.
	c.Assert(err, gc.ErrorMatches, "AllMetadata failed")
	c.Assert(called, jc.IsFalse)
}

var _ = gc.Suite(&getUrlSuite{})

type getUrlSuite struct {
	apiHostPortsGetter *mocks.MockAPIHostPortsForAgentsGetter
}

var _ = gc.Suite(&getUrlSuite{})

func (s *getUrlSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiHostPortsGetter = mocks.NewMockAPIHostPortsForAgentsGetter(ctrl)
	return ctrl
}

func (s *getUrlSuite) TestToolsURLGetterNoAPIHostPorts(c *gc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().APIHostPortsForAgents(gomock.Any()).Return(nil, nil)

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	_, err := g.ToolsURLs(context.Background(), coretesting.FakeControllerConfig(), coretesting.CurrentVersion())
	c.Assert(err, gc.ErrorMatches, "no suitable API server address to pick from")
}

func (s *getUrlSuite) TestToolsURLGetterAPIHostPortsError(c *gc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().APIHostPortsForAgents(gomock.Any()).Return(nil, errors.New("oh noes"))

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	_, err := g.ToolsURLs(context.Background(), coretesting.FakeControllerConfig(), coretesting.CurrentVersion())
	c.Assert(err, gc.ErrorMatches, "oh noes")
}

func (s *getUrlSuite) TestToolsURLGetter(c *gc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().APIHostPortsForAgents(gomock.Any()).Return([]network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}, nil)

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	current := coretesting.CurrentVersion()
	urls, err := g.ToolsURLs(context.Background(), coretesting.FakeControllerConfig(), current)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(urls, jc.DeepEquals, []string{
		"https://0.1.2.3:1234/model/my-uuid/tools/" + current.String(),
	})
}

type controllerConfigService struct{}

func (controllerConfigService) ControllerConfig(context.Context) (controller.Config, error) {
	return coretesting.FakeControllerConfig(), nil
}
