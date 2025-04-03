// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

// AgentFinderService defines a method for finding agent binary metadata.
type AgentFinderService interface {
	FindAgents(context.Context, modelagent.FindAgentsParams) (coretools.List, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	AgentFinderService

	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
	// not exist.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// GetMachineTargetAgentVersion reports the target agent version that should
	// be running on the provided machine identified by name. The following
	// errors are possible:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound]
	// - [github.com/juju/juju/domain/model/errors.NotFound]
	GetMachineTargetAgentVersion(context.Context, machine.Name) (coreagentbinary.Version, error)

	// GetUnitTargetAgentVersion reports the target agent version that should be
	// being run on the provided unit identified by name. The following errors
	// are possible:
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] - When
	// the unit in question does not exist.
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model
	// the unit belongs to no longer exists.
	GetUnitTargetAgentVersion(context.Context, unit.Name) (coreagentbinary.Version, error)
}

// ToolsURLGetter is an interface providing the ToolsURL method.
type ToolsURLGetter interface {
	// ToolsURLs returns URLs for the tools with
	// the specified binary version.
	ToolsURLs(context.Context, controller.Config, semversion.Binary) ([]string, error)
}

// APIHostPortsForAgentsGetter is an interface providing
// the APIHostPortsForAgents method.
type APIHostPortsForAgentsGetter interface {
	// APIHostPortsForAgents returns the HostPorts for each API server that
	// are suitable for agent-to-controller API communication based on the
	// configured (if any) controller management space.
	APIHostPortsForAgents(controller.Config) ([]network.SpaceHostPorts, error)
}

// ToolsStorageGetter is an interface providing the ToolsStorage method.
type ToolsStorageGetter interface {
	// ToolsStorage returns a binarystorage.StorageCloser.
	ToolsStorage(objectstore.ObjectStore) (binarystorage.StorageCloser, error)
}

// ToolsGetter implements a common Tools method for use by various
// facades.
type ToolsGetter struct {
	controllerConfigService ControllerConfigService
	modelAgentService       ModelAgentService
	toolsStorageGetter      ToolsStorageGetter
	toolsFinder             ToolsFinder
	urlGetter               ToolsURLGetter
	getCanRead              GetAuthFunc
}

// NewToolsGetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsGetter(
	controllerConfigService ControllerConfigService,
	modelAgentService ModelAgentService,
	toolsStorageGetter ToolsStorageGetter,
	urlGetter ToolsURLGetter,
	toolsFinder ToolsFinder,
	getCanRead GetAuthFunc,
) *ToolsGetter {
	return &ToolsGetter{
		controllerConfigService: controllerConfigService,
		modelAgentService:       modelAgentService,
		toolsStorageGetter:      toolsStorageGetter,
		urlGetter:               urlGetter,
		toolsFinder:             toolsFinder,
		getCanRead:              getCanRead,
	}
}

// getEntityAgentTargetVersion is responsible for getting the target agent version for
// a given tag.
func (t *ToolsGetter) getEntityAgentTargetVersion(
	ctx context.Context,
	tag names.Tag,
) (ver coreagentbinary.Version, err error) {
	switch tag.Kind() {
	case names.ControllerTagKind:
	case names.MachineTagKind:
		ver, err = t.modelAgentService.GetMachineTargetAgentVersion(ctx, machine.Name(tag.Id()))
	case names.UnitTagKind:
		ver, err = t.modelAgentService.GetUnitTargetAgentVersion(ctx, unit.Name(tag.Id()))
	default:
		return coreagentbinary.Version{}, errors.Errorf(
			"getting agent version for unsupported entity kind %q",
			tag.Kind(),
		).Add(coreerrors.NotSupported)
	}

	isEntityNotFound := errors.IsOneOf(
		err,
		applicationerrors.UnitNotFound,
		machineerrors.MachineNotFound,
	)
	if isEntityNotFound {
		return coreagentbinary.Version{}, errors.Errorf(
			"%q not found", names.ReadableString(tag),
		).Add(coreerrors.NotFound)
	} else if errors.Is(err, modelagenterrors.AgentVersionNotFound) {
		return coreagentbinary.Version{}, errors.Errorf(
			"target agent version for %q not found", names.ReadableString(tag),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return coreagentbinary.Version{}, errors.Errorf(
			"finding target agent version for entity %q: %w", names.ReadableString(tag), err,
		)
	}

	return ver, nil
}

// Tools finds the tools necessary for the given agents.
func (t *ToolsGetter) Tools(ctx context.Context, args params.Entities) (params.ToolsResults, error) {
	result := params.ToolsResults{
		Results: make([]params.ToolsResult, len(args.Entities)),
	}
	canRead, err := t.getCanRead()
	if err != nil {
		return result, err
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		agentToolsList, err := t.oneAgentTools(ctx, canRead, tag)
		if err == nil {
			result.Results[i].ToolsList = agentToolsList
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (t *ToolsGetter) oneAgentTools(ctx context.Context, canRead AuthFunc, tag names.Tag) (coretools.List, error) {
	if !canRead(tag) {
		return nil, apiservererrors.ErrPerm
	}

	targetVersion, err := t.getEntityAgentTargetVersion(ctx, tag)
	if err != nil {
		return nil, err
	}

	controllerCfg, err := t.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Errorf("getting controller config: %v", err)
	}

	findParams := FindAgentsParams{
		ControllerCfg: controllerCfg,
		Number:        targetVersion.Number,
		// OSType is always "ubuntu" now.
		// We will eventually get rid of it.
		OSType: strings.ToLower(ostype.Ubuntu.String()),
		Arch:   targetVersion.Arch,
	}

	return t.toolsFinder.FindAgents(ctx, findParams)
}

// FindAgentsParams defines parameters for the FindAgents method.
type FindAgentsParams struct {
	// ControllerCfg is the controller config.
	ControllerCfg controller.Config

	// ModelType is the type of the model.
	ModelType state.ModelType

	// Number will be used to match tools versions exactly if non-zero.
	Number semversion.Number

	// MajorVersion will be used to match the major version if non-zero.
	MajorVersion int

	// MinorVersion will be used to match the minor version if non-zero.
	MinorVersion int

	// Arch will be used to match tools by architecture if non-empty.
	Arch string

	// OSType will be used to match tools by os type if non-empty.
	OSType string

	// AgentStream will be used to set agent stream to search
	AgentStream string
}

// ToolsFinder defines methods for finding tools.
type ToolsFinder interface {
	FindAgents(context.Context, FindAgentsParams) (coretools.List, error)
}

type toolsFinder struct {
	agentFinderService AgentFinderService
	toolsStorageGetter ToolsStorageGetter
	urlGetter          ToolsURLGetter
	newEnviron         NewEnvironFunc
	store              objectstore.ObjectStore
}

// NewToolsFinder returns a new ToolsFinder, returning tools
// with their URLs pointing at the API server.
func NewToolsFinder(
	agentFinder AgentFinderService,
	toolsStorageGetter ToolsStorageGetter,
	urlGetter ToolsURLGetter,
	newEnviron NewEnvironFunc,
	store objectstore.ObjectStore,
) *toolsFinder {
	return &toolsFinder{
		agentFinderService: agentFinder,
		toolsStorageGetter: toolsStorageGetter,
		urlGetter:          urlGetter,
		newEnviron:         newEnviron,
		store:              store,
	}
}

// FindAgents calls findMatchingTools and then rewrites the URLs
// using the provided ToolsURLGetter.
func (f *toolsFinder) FindAgents(ctx context.Context, args FindAgentsParams) (coretools.List, error) {
	storage, err := f.toolsStorageGetter.ToolsStorage(f.store)
	if err != nil {
		return nil, errors.Errorf("getting agent binary storage: %w", err)
	}
	defer func() { _ = storage.Close() }()

	p := modelagent.FindAgentsParams{
		Number:       args.Number,
		MajorVersion: args.MajorVersion,
		MinorVersion: args.MinorVersion,
		Arch:         args.Arch,
		AgentStream:  args.AgentStream,
		ToolsURLsGetter: func(ctx context.Context, v semversion.Binary) ([]string, error) {
			return f.urlGetter.ToolsURLs(ctx, args.ControllerCfg, v)
		},
		AgentStorage: storage,
	}
	return f.agentFinderService.FindAgents(ctx, p)
}

type toolsURLGetter struct {
	modelUUID          string
	apiHostPortsGetter APIHostPortsForAgentsGetter
}

// NewToolsURLGetter creates a new ToolsURLGetter that
// returns tools URLs pointing at an API server.
func NewToolsURLGetter(modelUUID string, a APIHostPortsForAgentsGetter) *toolsURLGetter {
	return &toolsURLGetter{
		modelUUID:          modelUUID,
		apiHostPortsGetter: a,
	}
}

// ToolsURLs returns a list of tools URLs pointing at an API server.
func (t *toolsURLGetter) ToolsURLs(ctx context.Context, controllerConfig controller.Config, v semversion.Binary) ([]string, error) {
	addrs, err := apiAddresses(controllerConfig, t.apiHostPortsGetter)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("no suitable API server address to pick from")
	}
	var urls []string
	for _, addr := range addrs {
		serverRoot := fmt.Sprintf("https://%s/model/%s", addr, t.modelUUID)
		url := ToolsURL(serverRoot, v.String())
		urls = append(urls, url)
	}
	return urls, nil
}

// ToolsURL returns a tools URL pointing the API server
// specified by the "serverRoot".
func ToolsURL(serverRoot string, v string) string {
	return fmt.Sprintf("%s/tools/%s", serverRoot, v)
}
