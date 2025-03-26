// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"
	"sort"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
	// not exist.
	GetModelTargetAgentVersion(context.Context) (version.Number, error)

	// GetMachineTargetAgentVersion reports the target agent version that should
	// be running on the provided machine identified by name. The following
	// errors are possible:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound]
	// - [github.com/juju/juju/domain/model/errors.NotFound]
	GetMachineTargetAgentVersion(context.Context, machine.Name) (version.Number, error)

	// GetUnitTargetAgentVersion reports the target agent version that should be
	// being run on the provided unit identified by name. The following errors
	// are possible:
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] - When
	// the unit in question does not exist.
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model
	// the unit belongs to no longer exists.
	GetUnitTargetAgentVersion(context.Context, string) (version.Number, error)
}

var envtoolsFindTools = envtools.FindTools

type ToolsFindEntity interface {
	FindEntity(tag names.Tag) (state.Entity, error)
}

// ToolsURLGetter is an interface providing the ToolsURL method.
type ToolsURLGetter interface {
	// ToolsURLs returns URLs for the tools with
	// the specified binary version.
	ToolsURLs(context.Context, controller.Config, version.Binary) ([]string, error)
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

// AgentTooler is implemented by entities
// that have associated agent tools.
type AgentTooler interface {
	AgentTools() (*coretools.Tools, error)
	SetAgentVersion(version.Binary) error

	// Tag is included in this interface only so the generated mock of
	// AgentTooler implements state.Entity, returned by FindEntity
	Tag() names.Tag
}

// ToolsGetter implements a common Tools method for use by various
// facades.
type ToolsGetter struct {
	entityFinder       ToolsFindEntity
	modelAgentService  ModelAgentService
	toolsStorageGetter ToolsStorageGetter
	toolsFinder        ToolsFinder
	urlGetter          ToolsURLGetter
	getCanRead         GetAuthFunc
}

// NewToolsGetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsGetter(
	entityFinder ToolsFindEntity,
	modelAgentService ModelAgentService,
	toolsStorageGetter ToolsStorageGetter,
	urlGetter ToolsURLGetter,
	toolsFinder ToolsFinder,
	getCanRead GetAuthFunc,
) *ToolsGetter {
	return &ToolsGetter{
		entityFinder:       entityFinder,
		modelAgentService:  modelAgentService,
		toolsStorageGetter: toolsStorageGetter,
		urlGetter:          urlGetter,
		toolsFinder:        toolsFinder,
		getCanRead:         getCanRead,
	}
}

// getEntityAgentVersion is responsible for getting the target agent version for
// a given tag.
func (t *ToolsGetter) getEntityAgentVersion(
	ctx context.Context,
	tag names.Tag,
) (ver version.Number, err error) {
	switch tag.Kind() {
	case names.ControllerTagKind:
	case names.ModelTagKind:
		ver, err = t.modelAgentService.GetModelTargetAgentVersion(ctx)
	case names.MachineTagKind:
		ver, err = t.modelAgentService.GetMachineTargetAgentVersion(ctx, machine.Name(tag.Id()))
	case names.UnitTagKind:
		ver, err = t.modelAgentService.GetUnitTargetAgentVersion(ctx, tag.Id())
	default:
		return version.Zero, errors.Errorf(
			"getting agent version for unsupported entity kind %q",
			tag.Kind(),
		).Add(coreerrors.NotSupported)
	}

	isNotFound := errors.IsOneOf(
		err,
		applicationerrors.ApplicationNotFound,
		applicationerrors.UnitNotFound,
		machineerrors.MachineNotFound,
		modelerrors.NotFound,
	)
	if isNotFound {
		return version.Zero, errors.Errorf(
			"entity %q does not exist", tag.String(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return version.Zero, errors.Errorf(
			"finding agent version for entity %q: %w", tag.String(), err,
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

		agentVersion, err := t.getEntityAgentVersion(ctx, tag)
		if err != nil {
			return result, err
		}

		agentToolsList, err := t.oneAgentTools(ctx, canRead, tag, agentVersion)
		if err == nil {
			result.Results[i].ToolsList = agentToolsList
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (t *ToolsGetter) oneAgentTools(ctx context.Context, canRead AuthFunc, tag names.Tag, agentVersion version.Number) (coretools.List, error) {
	if !canRead(tag) {
		return nil, apiservererrors.ErrPerm
	}
	entity, err := t.entityFinder.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	tooler, ok := entity.(AgentTooler)
	if !ok {
		return nil, apiservererrors.NotSupportedError(tag, "agent binaries")
	}
	existingTools, err := tooler.AgentTools()
	if err != nil {
		return nil, err
	}

	findParams := FindAgentsParams{
		Number: agentVersion,
		OSType: existingTools.Version.Release,
		Arch:   existingTools.Version.Arch,
	}

	return t.toolsFinder.FindAgents(ctx, findParams)
}

// ToolsSetter implements a common Tools method for use by various
// facades.
type ToolsSetter struct {
	st          ToolsFindEntity
	getCanWrite GetAuthFunc
}

// NewToolsSetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsSetter(st ToolsFindEntity, getCanWrite GetAuthFunc) *ToolsSetter {
	return &ToolsSetter{
		st:          st,
		getCanWrite: getCanWrite,
	}
}

// SetTools updates the recorded tools version for the agents.
func (t *ToolsSetter) SetTools(ctx context.Context, args params.EntitiesVersion) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.AgentTools)),
	}
	canWrite, err := t.getCanWrite()
	if err != nil {
		return results, err
	}
	for i, agentTools := range args.AgentTools {
		tag, err := names.ParseTag(agentTools.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = t.setOneAgentVersion(tag, agentTools.Tools.Version, canWrite)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (t *ToolsSetter) setOneAgentVersion(tag names.Tag, vers version.Binary, canWrite AuthFunc) error {
	if !canWrite(tag) {
		return apiservererrors.ErrPerm
	}
	entity0, err := t.st.FindEntity(tag)
	if err != nil {
		return err
	}
	entity, ok := entity0.(AgentTooler)
	if !ok {
		return apiservererrors.NotSupportedError(tag, "agent binaries")
	}
	return entity.SetAgentVersion(vers)
}

// FindAgentsParams defines parameters for the FindAgents method.
type FindAgentsParams struct {
	// ControllerCfg is the controller config.
	ControllerCfg controller.Config

	// ModelType is the type of the model.
	ModelType state.ModelType

	// Number will be used to match tools versions exactly if non-zero.
	Number version.Number

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
	controllerConfigService ControllerConfigService
	toolsStorageGetter      ToolsStorageGetter
	urlGetter               ToolsURLGetter
	newEnviron              NewEnvironFunc
	store                   objectstore.ObjectStore
}

// NewToolsFinder returns a new ToolsFinder, returning tools
// with their URLs pointing at the API server.
func NewToolsFinder(
	controllerConfigService ControllerConfigService,
	toolsStorageGetter ToolsStorageGetter,
	urlGetter ToolsURLGetter,
	newEnviron NewEnvironFunc,
	store objectstore.ObjectStore,
) *toolsFinder {
	return &toolsFinder{
		controllerConfigService: controllerConfigService,
		toolsStorageGetter:      toolsStorageGetter,
		urlGetter:               urlGetter,
		newEnviron:              newEnviron,
		store:                   store,
	}
}

// FindAgents calls findMatchingTools and then rewrites the URLs
// using the provided ToolsURLGetter.
func (f *toolsFinder) FindAgents(ctx context.Context, args FindAgentsParams) (coretools.List, error) {
	list, err := f.findMatchingAgents(ctx, args)
	if err != nil {
		return nil, err
	}

	controllerConfig, err := f.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Rewrite the URLs so they point at the API servers. If the
	// tools are not in tools storage, then the API server will
	// download and cache them if the client requests that version.
	var fullList coretools.List
	for _, baseTools := range list {
		urls, err := f.urlGetter.ToolsURLs(ctx, controllerConfig, baseTools.Version)
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
func (f *toolsFinder) findMatchingAgents(ctx context.Context, args FindAgentsParams) (result coretools.List, _ error) {
	exactMatch := args.Number != version.Zero && args.OSType != "" && args.Arch != ""

	storageList, err := f.matchingStorageAgent(args)
	if err != nil && err != coretools.ErrNoMatches {
		return nil, err
	}
	if len(storageList) > 0 && exactMatch {
		return storageList, nil
	}

	// Look for tools in simplestreams too, but don't replace
	// any versions found in storage.
	env, err := f.newEnviron(ctx)
	if err != nil {
		return nil, err
	}
	filter := toolsFilter(args)
	cfg := env.Config()
	requestedStream := cfg.AgentStream()
	if args.AgentStream != "" {
		requestedStream = args.AgentStream
	}

	streams := envtools.PreferredStreams(&args.Number, cfg.Development(), requestedStream)
	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	majorVersion := args.Number.Major
	minorVersion := args.Number.Minor
	if args.Number == version.Zero {
		majorVersion = args.MajorVersion
		minorVersion = args.MinorVersion
	}
	simplestreamsList, err := envtoolsFindTools(ctx, ss,
		env, majorVersion, minorVersion, streams, filter,
	)
	if len(storageList) == 0 && err != nil {
		return nil, err
	}

	list := storageList
	found := make(map[version.Binary]bool)
	for _, tools := range storageList {
		found[tools.Version] = true
	}
	for _, tools := range simplestreamsList {
		if !found[tools.Version] {
			list = append(list, tools)
		}
	}
	sort.Sort(list)
	return list, nil
}

// matchingStorageAgent returns a coretools.List, with an entry for each
// metadata entry in the agent storage that matches the given parameters.
func (f *toolsFinder) matchingStorageAgent(args FindAgentsParams) (coretools.List, error) {
	storage, err := f.toolsStorageGetter.ToolsStorage(f.store)
	if err != nil {
		return nil, err
	}
	defer func() { _ = storage.Close() }()

	allMetadata, err := storage.AllMetadata()
	if err != nil {
		return nil, err
	}
	list := make(coretools.List, len(allMetadata))
	for i, m := range allMetadata {
		vers, err := version.ParseBinary(m.Version)
		if err != nil {
			return nil, errors.Errorf(
				"unexpected bad version %q of agent binary in storage: %w",
				m.Version, err,
			)
		}
		list[i] = &coretools.Tools{
			Version: vers,
			Size:    m.Size,
			SHA256:  m.SHA256,
		}
	}
	list, err = list.Match(toolsFilter(args))
	if err != nil {
		return nil, err
	}
	// Return early if we are doing an exact match.
	if args.Number != version.Zero {
		if len(list) == 0 {
			return nil, coretools.ErrNoMatches
		}
		return list, nil
	}
	// At this point, we are matching just on major or minor version
	// rather than an exact match.
	var matching coretools.List
	for _, tools := range list {
		if tools.Version.Major != args.MajorVersion {
			continue
		}
		if args.MinorVersion > 0 && tools.Version.Minor != args.MinorVersion {
			continue
		}
		matching = append(matching, tools)
	}
	if len(matching) == 0 {
		return nil, coretools.ErrNoMatches
	}
	return matching, nil
}

func toolsFilter(args FindAgentsParams) coretools.Filter {
	return coretools.Filter{
		Number: args.Number,
		Arch:   args.Arch,
		OSType: args.OSType,
	}
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
func (t *toolsURLGetter) ToolsURLs(ctx context.Context, controllerConfig controller.Config, v version.Binary) ([]string, error) {
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
