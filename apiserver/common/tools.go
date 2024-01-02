// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	coretools "github.com/juju/juju/tools"
)

var envtoolsFindTools = envtools.FindTools

type ToolsFindEntity interface {
	FindEntity(tag names.Tag) (state.Entity, error)
}

// ToolsURLGetter is an interface providing the ToolsURL method.
type ToolsURLGetter interface {
	// ToolsURLs returns URLs for the tools with
	// the specified binary version.
	ToolsURLs(v version.Binary) ([]string, error)
}

// APIHostPortsForAgentsGetter is an interface providing
// the APIHostPortsForAgents method.
type APIHostPortsForAgentsGetter interface {
	// APIHostPortsForAgents returns the HostPorts for each API server that
	// are suitable for agent-to-controller API communication based on the
	// configured (if any) controller management space.
	APIHostPortsForAgents() ([]network.SpaceHostPorts, error)
}

// ToolsStorageGetter is an interface providing the ToolsStorage method.
type ToolsStorageGetter interface {
	// ToolsStorage returns a binarystorage.StorageCloser.
	ToolsStorage() (binarystorage.StorageCloser, error)
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
	configGetter       environs.EnvironConfigGetter
	toolsStorageGetter ToolsStorageGetter
	toolsFinder        ToolsFinder
	urlGetter          ToolsURLGetter
	getCanRead         GetAuthFunc
}

// NewToolsGetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsGetter(
	entityFinder ToolsFindEntity,
	configGetter environs.EnvironConfigGetter,
	toolsStorageGetter ToolsStorageGetter,
	urlGetter ToolsURLGetter,
	toolsFinder ToolsFinder,
	getCanRead GetAuthFunc,
) *ToolsGetter {
	return &ToolsGetter{
		entityFinder:       entityFinder,
		configGetter:       configGetter,
		toolsStorageGetter: toolsStorageGetter,
		urlGetter:          urlGetter,
		toolsFinder:        toolsFinder,
		getCanRead:         getCanRead,
	}
}

// Tools finds the tools necessary for the given agents.
func (t *ToolsGetter) Tools(args params.Entities) (params.ToolsResults, error) {
	result := params.ToolsResults{
		Results: make([]params.ToolsResult, len(args.Entities)),
	}
	canRead, err := t.getCanRead()
	if err != nil {
		return result, err
	}
	agentVersion, err := t.getGlobalAgentVersion()
	if err != nil {
		return result, err
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		agentToolsList, err := t.oneAgentTools(canRead, tag, agentVersion)
		if err == nil {
			result.Results[i].ToolsList = agentToolsList
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (t *ToolsGetter) getGlobalAgentVersion() (version.Number, error) {
	// Get the Agent Version requested in the Model Config
	nothing := version.Number{}
	cfg, err := t.configGetter.ModelConfig()
	if err != nil {
		return nothing, err
	}
	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		return nothing, errors.New("agent version not set in model config")
	}
	return agentVersion, nil
}

func (t *ToolsGetter) oneAgentTools(canRead AuthFunc, tag names.Tag, agentVersion version.Number) (coretools.List, error) {
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

	return t.toolsFinder.FindAgents(findParams)
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
func (t *ToolsSetter) SetTools(args params.EntitiesVersion) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.AgentTools)),
	}
	canWrite, err := t.getCanWrite()
	if err != nil {
		return results, errors.Trace(err)
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
	FindAgents(args FindAgentsParams) (coretools.List, error)
}

type toolsFinder struct {
	configGetter       environs.EnvironConfigGetter
	toolsStorageGetter ToolsStorageGetter
	urlGetter          ToolsURLGetter
	newEnviron         NewEnvironFunc
}

// NewToolsFinder returns a new ToolsFinder, returning tools
// with their URLs pointing at the API server.
func NewToolsFinder(
	configGetter environs.EnvironConfigGetter,
	toolsStorageGetter ToolsStorageGetter,
	urlGetter ToolsURLGetter,
	newEnviron NewEnvironFunc,
) *toolsFinder {
	return &toolsFinder{configGetter, toolsStorageGetter, urlGetter, newEnviron}
}

// FindAgents calls findMatchingTools and then rewrites the URLs
// using the provided ToolsURLGetter.
func (f *toolsFinder) FindAgents(args FindAgentsParams) (coretools.List, error) {
	list, err := f.findMatchingAgents(args)
	if err != nil {
		return nil, err
	}

	// Rewrite the URLs so they point at the API servers. If the
	// tools are not in tools storage, then the API server will
	// download and cache them if the client requests that version.
	var fullList coretools.List
	for _, baseTools := range list {
		urls, err := f.urlGetter.ToolsURLs(baseTools.Version)
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
func (f *toolsFinder) findMatchingAgents(args FindAgentsParams) (result coretools.List, _ error) {
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
	env, err := f.newEnviron()
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
	simplestreamsList, err := envtoolsFindTools(ss,
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
	storage, err := f.toolsStorageGetter.ToolsStorage()
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
			return nil, errors.Annotatef(err, "unexpected bad version %q of agent binary in storage", m.Version)
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
	return &toolsURLGetter{modelUUID, a}
}

func (t *toolsURLGetter) ToolsURLs(v version.Binary) ([]string, error) {
	addrs, err := apiAddresses(t.apiHostPortsGetter)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.Errorf("no suitable API server address to pick from")
	}
	var urls []string
	for _, addr := range addrs {
		serverRoot := fmt.Sprintf("https://%s/model/%s", addr, t.modelUUID)
		url := ToolsURL(serverRoot, v)
		urls = append(urls, url)
	}
	return urls, nil
}

// ToolsURL returns a tools URL pointing the API server
// specified by the "serverRoot".
func ToolsURL(serverRoot string, v version.Binary) string {
	return fmt.Sprintf("%s/tools/%s", serverRoot, v.String())
}
