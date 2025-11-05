// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/names/v6"
	"github.com/juju/os/v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
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
	ToolsURLs(context.Context, semversion.Binary) ([]string, error)
}

// ToolsGetter implements a common Tools method for use by various
// facades.
type ToolsGetter struct {
	modelAgentService ModelAgentService
	toolsFinder       ToolsFinder
	urlGetter         ToolsURLGetter
	getCanRead        GetAuthFunc
}

// NewToolsGetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsGetter(
	modelAgentService ModelAgentService,
	urlGetter ToolsURLGetter,
	toolsFinder ToolsFinder,
	getCanRead GetAuthFunc,
) *ToolsGetter {
	return &ToolsGetter{
		modelAgentService: modelAgentService,
		urlGetter:         urlGetter,
		toolsFinder:       toolsFinder,
		getCanRead:        getCanRead,
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
	canRead, err := t.getCanRead(ctx)
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

	findParams := FindAgentsParams{
		Number: targetVersion.Number,
		// OSType is always "ubuntu" now.
		// We will eventually get rid of it.
		OSType: strings.ToLower(os.Ubuntu.String()),
		Arch:   targetVersion.Arch,
	}

	return t.toolsFinder.FindAgents(ctx, findParams)
}

// FindAgentsParams defines parameters for the FindAgents method.
type FindAgentsParams struct {
	// ControllerCfg is the controller config.
	ControllerCfg controller.Config

	// ModelType is the type of the model.
	ModelType coremodel.ModelType

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
	urlGetter          ToolsURLGetter
	agentBinaryService AgentBinaryService
}

// AgentBinaryService is an interface for getting the available agent binaries
// within a controller.
type AgentBinaryService interface {
	// GetAvailableAgentBinaryiesForVersion returns a list of all agent binaries
	// available for the specified version across all architectures supported.
	GetAvailableAgentBinaryiesForVersion(
		context.Context, semversion.Number,
	) ([]domainagentbinary.AgentBinary, error)
}

// NewToolsFinder returns a new ToolsFinder, returning tools
// with their URLs pointing at the API server.
func NewToolsFinder(
	urlGetter ToolsURLGetter,
	store objectstore.ObjectStore,
	agentBinaryService AgentBinaryService,
) *toolsFinder {
	return &toolsFinder{
		urlGetter:          urlGetter,
		agentBinaryService: agentBinaryService,
	}
}

// FindAgents calls findMatchingTools and then rewrites the URLs
// using the provided ToolsURLGetter.
func (f *toolsFinder) FindAgents(ctx context.Context, args FindAgentsParams) (coretools.List, error) {
	list, err := f.findMatchingAgents(ctx, args)
	if err != nil {
		return nil, err
	}

	// Rewrite the URLs so they point at the API servers. If the
	// tools are not in tools storage, then the API server will
	// download and cache them if the client requests that version.
	var fullList coretools.List
	for _, baseTools := range list {
		urls, err := f.urlGetter.ToolsURLs(ctx, baseTools.Version)
		if err != nil {
			return nil, err
		}
		for _, url := range urls {
			tools := *baseTools
			tools.URL = url
			fullList = append(fullList, &tools)
		}
	}
	return fullList, nil
}

// findMatchingAgents searches agent storage and simplestreams for agents
// matching the given parameters.
// If an exact match is specified (number, ostype and arch) and is found in
// agent storage, then simplestreams will not be searched.
func (f *toolsFinder) findMatchingAgents(ctx context.Context, args FindAgentsParams) (coretools.List, error) {
	agentBinaries, err := f.agentBinaryService.GetAvailableAgentBinaryiesForVersion(
		ctx, args.Number,
	)
	if err != nil {
		return nil, internalerrors.Errorf(
			"getting available agent binaries: %w", err,
		)
	}

	retVal := make(coretools.List, 0, len(agentBinaries))
	for _, ab := range agentBinaries {
		retVal = append(retVal, &coretools.Tools{
			SHA256: ab.SHA256,
			Size:   int64(ab.Size),
			Version: semversion.Binary{
				Arch:    ab.Architecture.String(),
				Release: strings.ToLower(os.Ubuntu.String()),
				Number:  ab.Version,
			},
		})
	}

	sort.Sort(retVal)
	return retVal, nil
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
func (t *toolsURLGetter) ToolsURLs(ctx context.Context, v semversion.Binary) ([]string, error) {
	addrs, err := t.apiHostPortsGetter.GetAllAPIAddressesForAgents(ctx)
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
