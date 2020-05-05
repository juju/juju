// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	coretools "github.com/juju/juju/tools"
)

var envtoolsFindTools = envtools.FindTools

// ToolsURLGetter is an interface providing the ToolsURL method.
type ToolsURLGetter interface {
	// ToolsURLs returns URLs for the tools with
	// the specified binary version.
	ToolsURLs(v version.Binary) ([]string, error)
}

// APIHostPortsGetter is an interface providing the APIHostPortsForAgents
// method.
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

// ToolsGetter implements a common Tools method for use by various
// facades.
type ToolsGetter struct {
	newEnviron         NewEnvironFunc
	entityFinder       state.EntityFinder
	configGetter       environs.EnvironConfigGetter
	toolsStorageGetter ToolsStorageGetter
	urlGetter          ToolsURLGetter
	getCanRead         GetAuthFunc
}

// NewToolsGetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsGetter(
	f state.EntityFinder,
	c environs.EnvironConfigGetter,
	s ToolsStorageGetter,
	t ToolsURLGetter,
	getCanRead GetAuthFunc,
	newEnviron NewEnvironFunc,
) *ToolsGetter {
	return &ToolsGetter{
		newEnviron:         newEnviron,
		entityFinder:       f,
		configGetter:       c,
		toolsStorageGetter: s,
		urlGetter:          t,
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
	toolsStorage, err := t.toolsStorageGetter.ToolsStorage()
	if err != nil {
		return result, err
	}
	defer toolsStorage.Close()

	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		agentToolsList, err := t.oneAgentTools(canRead, tag, agentVersion, toolsStorage)
		if err == nil {
			result.Results[i].ToolsList = agentToolsList
			// TODO(axw) Get rid of this in 1.22, when all upgraders
			// are known to ignore the flag.
			result.Results[i].DisableSSLHostnameVerification = true
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}

func (t *ToolsGetter) getGlobalAgentVersion() (version.Number, error) {
	// Get the Agent Version requested in the Environment Config
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

func (t *ToolsGetter) oneAgentTools(canRead AuthFunc, tag names.Tag, agentVersion version.Number, storage binarystorage.Storage) (coretools.List, error) {
	if !canRead(tag) {
		return nil, ErrPerm
	}
	entity, err := t.entityFinder.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	tooler, ok := entity.(state.AgentTooler)
	if !ok {
		return nil, NotSupportedError(tag, "agent binaries")
	}
	existingTools, err := tooler.AgentTools()
	if err != nil {
		return nil, err
	}
	toolsFinder := NewToolsFinder(t.configGetter, t.toolsStorageGetter, t.urlGetter, t.newEnviron)
	list, err := toolsFinder.findTools(params.FindToolsParams{
		Number:       agentVersion,
		MajorVersion: -1,
		MinorVersion: -1,
		Series:       existingTools.Version.Series,
		Arch:         existingTools.Version.Arch,
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// ToolsSetter implements a common Tools method for use by various
// facades.
type ToolsSetter struct {
	st          state.EntityFinder
	getCanWrite GetAuthFunc
}

// NewToolsSetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsSetter(st state.EntityFinder, getCanWrite GetAuthFunc) *ToolsSetter {
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
			results.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		err = t.setOneAgentVersion(tag, agentTools.Tools.Version, canWrite)
		results.Results[i].Error = ServerError(err)
	}
	return results, nil
}

func (t *ToolsSetter) setOneAgentVersion(tag names.Tag, vers version.Binary, canWrite AuthFunc) error {
	if !canWrite(tag) {
		return ErrPerm
	}
	entity0, err := t.st.FindEntity(tag)
	if err != nil {
		return err
	}
	entity, ok := entity0.(state.AgentTooler)
	if !ok {
		return NotSupportedError(tag, "agent binaries")
	}
	return entity.SetAgentVersion(vers)
}

type ToolsFinder struct {
	configGetter       environs.EnvironConfigGetter
	toolsStorageGetter ToolsStorageGetter
	urlGetter          ToolsURLGetter
	newEnviron         NewEnvironFunc
}

// NewToolsFinder returns a new ToolsFinder, returning tools
// with their URLs pointing at the API server.
func NewToolsFinder(
	c environs.EnvironConfigGetter, s ToolsStorageGetter, t ToolsURLGetter,
	newEnviron NewEnvironFunc,
) *ToolsFinder {
	return &ToolsFinder{c, s, t, newEnviron}
}

// FindTools returns a List containing all tools matching the given parameters.
func (f *ToolsFinder) FindTools(args params.FindToolsParams) (params.FindToolsResult, error) {
	result := params.FindToolsResult{}
	list, err := f.findTools(args)
	if err != nil {
		result.Error = ServerError(err)
	} else {
		result.List = list
	}
	return result, nil
}

// findTools calls findMatchingTools and then rewrites the URLs
// using the provided ToolsURLGetter.
func (f *ToolsFinder) findTools(args params.FindToolsParams) (coretools.List, error) {
	list, err := f.findMatchingTools(args)
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

// findMatchingTools searches tools storage and simplestreams for tools matching the
// given parameters. If an exact match is specified (number, series and arch)
// and is found in tools storage, then simplestreams will not be searched.
func (f *ToolsFinder) findMatchingTools(args params.FindToolsParams) (coretools.List, error) {
	exactMatch := args.Number != version.Zero && args.Series != "" && args.Arch != ""
	storageList, err := f.matchingStorageTools(args)
	if err == nil && exactMatch {
		return storageList, nil
	} else if err != nil && err != coretools.ErrNoMatches {
		return nil, err
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
	simplestreamsList, err := envtoolsFindTools(
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
func (f *ToolsFinder) matchingStorageTools(args params.FindToolsParams) (coretools.List, error) {
	storage, err := f.toolsStorageGetter.ToolsStorage()
	if err != nil {
		return nil, err
	}
	defer storage.Close()
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
	return coretools.Filter{
		Number: args.Number,
		Arch:   args.Arch,
		Series: args.Series,
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
