// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/core/network"
	coreos "github.com/juju/juju/v2/core/os"
	coreseries "github.com/juju/juju/v2/core/series"
	"github.com/juju/juju/v2/environs"
	"github.com/juju/juju/v2/environs/simplestreams"
	envtools "github.com/juju/juju/v2/environs/tools"
	"github.com/juju/juju/v2/rpc/params"
	"github.com/juju/juju/v2/state"
	"github.com/juju/juju/v2/state/binarystorage"
	coretools "github.com/juju/juju/v2/tools"
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

	findParams := params.FindToolsParams{
		Number:       agentVersion,
		MajorVersion: -1,
		MinorVersion: -1,
		OSType:       existingTools.Version.Release,
		Arch:         existingTools.Version.Arch,
	}
	// Older agents will ask for tools based on series.
	// We now store tools based on OS name so update the find params
	// if needed to ensure the correct search is done.
	allSeries, err := coreseries.AllWorkloadSeries("", "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	if allSeries.Contains(existingTools.Version.Release) {
		findParams.Series = existingTools.Version.Release
		findParams.OSType = ""
	}

	tools, err := t.toolsFinder.FindTools(findParams)
	if err != nil {
		return nil, err
	}
	return tools.List, nil
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

// ToolsFinder defines methods for finding tools.
type ToolsFinder interface {
	FindTools(args params.FindToolsParams) (params.FindToolsResult, error)
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

// FindTools returns a List containing all tools matching the given parameters.
func (f *toolsFinder) FindTools(args params.FindToolsParams) (params.FindToolsResult, error) {
	list, err := f.findTools(args)
	if err != nil {
		return params.FindToolsResult{Error: apiservererrors.ServerError(err)}, nil
	}

	return params.FindToolsResult{List: list}, nil
}

// findTools calls findMatchingTools and then rewrites the URLs
// using the provided ToolsURLGetter.
func (f *toolsFinder) findTools(args params.FindToolsParams) (coretools.List, error) {
	list, err := f.findMatchingTools(args)
	if err != nil {
		return nil, err
	}

	// This handles clients and agents that may be attempting to find tools in
	// the context of series instead of OS type.
	// If we get a request by series we ensure that any matched OS tools are
	// converted to the requested series.
	// Conversely, if we get a request by OS type, matching series tools are
	// converted to match the OS.
	// TODO: Remove this block and the called methods for Juju 3/4.
	if args.Number.Major == 2 && args.Number.Minor <= 8 && (args.OSType != "" || args.Series != "") {
		if args.OSType != "" {
			list = f.resultForOSTools(list, args.OSType)
		}
		if args.Series != "" {
			list = f.resultForSeriesTools(list, args.Series)
		}
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

// TODO: Remove for Juju 3/4.
func (f *toolsFinder) resultForOSTools(list coretools.List, osType string) coretools.List {
	added := make(map[version.Binary]bool)
	var matched coretools.List
	for _, t := range list {
		converted := *t

		// t might be for a series so convert to an OS type.
		if !coreos.IsValidOSTypeName(t.Version.Release) {
			osTypeName, err := coreseries.GetOSFromSeries(t.Version.Release)
			if err != nil {
				continue
			}
			converted.Version.Release = strings.ToLower(osTypeName.String())
		}

		if converted.Version.Release != osType {
			continue
		}
		if added[converted.Version] {
			continue
		}

		matched = append(matched, &converted)
		added[converted.Version] = true
	}

	return matched
}

// TODO: Remove for Juju 3/4.
func (f *toolsFinder) resultForSeriesTools(list coretools.List, series string) coretools.List {
	osType := coreseries.DefaultOSTypeNameFromSeries(series)

	added := make(map[version.Binary]bool)
	var matched coretools.List
	for _, t := range list {
		converted := *t

		if coreos.IsValidOSTypeName(t.Version.Release) {
			if osType != t.Version.Release {
				continue
			}
			converted.Version.Release = series
		} else if series != t.Version.Release {
			continue
		}
		if added[converted.Version] {
			continue
		}

		matched = append(matched, &converted)
		added[converted.Version] = true
	}

	return matched
}

// findMatchingTools searches tools storage and simplestreams for tools
// matching the given parameters.
// If an exact match is specified (number, series and arch) and is found in
// tools storage, then simplestreams will not be searched.
func (f *toolsFinder) findMatchingTools(args params.FindToolsParams) (result coretools.List, _ error) {
	exactMatch := args.Number != version.Zero && (args.OSType != "" || args.Series != "") && args.Arch != ""

	storageList, err := f.matchingStorageTools(args)
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
	simplestreamsList, err := envtoolsFindTools(ss,
		env, args.MajorVersion, args.MinorVersion, streams, filter,
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

// matchingStorageTools returns a coretools.List, with an entry for each
// metadata entry in the tools storage that matches the given parameters.
func (f *toolsFinder) matchingStorageTools(args params.FindToolsParams) (coretools.List, error) {
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
	var matching coretools.List
	for _, tools := range list {
		if args.MajorVersion != -1 && tools.Version.Major != args.MajorVersion {
			continue
		}
		if args.MinorVersion != -1 && tools.Version.Minor != args.MinorVersion {
			continue
		}
		matching = append(matching, tools)
	}
	if len(matching) == 0 {
		return nil, coretools.ErrNoMatches
	}
	return matching, nil
}

func toolsFilter(args params.FindToolsParams) coretools.Filter {
	var release string
	if args.Series != "" {
		release = coreseries.DefaultOSTypeNameFromSeries(args.Series)
	}
	if args.OSType != "" {
		release = args.OSType
	}
	return coretools.Filter{
		Number: args.Number,
		Arch:   args.Arch,
		OSType: release,
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
