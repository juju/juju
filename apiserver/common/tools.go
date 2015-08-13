// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/toolstorage"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var envtoolsFindTools = envtools.FindTools

// ToolsURLGetter is an interface providing the ToolsURL method.
type ToolsURLGetter interface {
	// ToolsURL returns a URL for the tools with
	// the specified binary version.
	ToolsURL(v version.Binary) (string, error)
}

type EnvironConfigGetter interface {
	EnvironConfig() (*config.Config, error)
}

// APIHostPortsGetter is an interface providing the APIHostPorts method.
type APIHostPortsGetter interface {
	// APIHostPorst returns the HostPorts for each API server.
	APIHostPorts() ([][]network.HostPort, error)
}

// ToolsStorageGetter is an interface providing the ToolsStorage method.
type ToolsStorageGetter interface {
	// ToolsStorage returns a toolstorage.StorageCloser.
	ToolsStorage() (toolstorage.StorageCloser, error)
}

// ToolsGetter implements a common Tools method for use by various
// facades.
type ToolsGetter struct {
	entityFinder       state.EntityFinder
	configGetter       EnvironConfigGetter
	toolsStorageGetter ToolsStorageGetter
	urlGetter          ToolsURLGetter
	getCanRead         GetAuthFunc
}

// NewToolsGetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsGetter(f state.EntityFinder, c EnvironConfigGetter, s ToolsStorageGetter, t ToolsURLGetter, getCanRead GetAuthFunc) *ToolsGetter {
	return &ToolsGetter{f, c, s, t, getCanRead}
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
		agentTools, err := t.oneAgentTools(canRead, tag, agentVersion, toolsStorage)
		if err == nil {
			result.Results[i].Tools = agentTools
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
	cfg, err := t.configGetter.EnvironConfig()
	if err != nil {
		return nothing, err
	}
	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		return nothing, errors.New("agent version not set in environment config")
	}
	return agentVersion, nil
}

func (t *ToolsGetter) oneAgentTools(canRead AuthFunc, tag names.Tag, agentVersion version.Number, storage toolstorage.Storage) (*coretools.Tools, error) {
	if !canRead(tag) {
		return nil, ErrPerm
	}
	entity, err := t.entityFinder.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	tooler, ok := entity.(state.AgentTooler)
	if !ok {
		return nil, NotSupportedError(tag, "agent tools")
	}
	existingTools, err := tooler.AgentTools()
	if err != nil {
		return nil, err
	}
	toolsFinder := NewToolsFinder(t.configGetter, t.toolsStorageGetter, t.urlGetter)
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
	return list[0], nil
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
		return NotSupportedError(tag, "agent tools")
	}
	return entity.SetAgentVersion(vers)
}

type ToolsFinder struct {
	configGetter       EnvironConfigGetter
	toolsStorageGetter ToolsStorageGetter
	urlGetter          ToolsURLGetter
}

// NewToolsFinder returns a new ToolsFinder, returning tools
// with their URLs pointing at the API server.
func NewToolsFinder(c EnvironConfigGetter, s ToolsStorageGetter, t ToolsURLGetter) *ToolsFinder {
	return &ToolsFinder{c, s, t}
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
	// Rewrite the URLs so they point at the API server. If the
	// tools are not in toolstorage, then the API server will
	// download and cache them if the client requests that version.
	for _, tools := range list {
		url, err := f.urlGetter.ToolsURL(tools.Version)
		if err != nil {
			return nil, err
		}
		tools.URL = url
	}
	return list, nil
}

// findMatchingTools searches toolstorage and simplestreams for tools matching the
// given parameters. If an exact match is specified (number, series and arch)
// and is found in toolstorage, then simplestreams will not be searched.
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
	cfg, err := f.configGetter.EnvironConfig()
	if err != nil {
		return nil, err
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	filter := toolsFilter(args)
	stream := envtools.PreferredStream(&args.Number, cfg.Development(), cfg.AgentStream())
	simplestreamsList, err := envtoolsFindTools(
		env, args.MajorVersion, args.MinorVersion, stream, filter,
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
// metadata entry in the toolstorage that matches the given parameters.
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
		list[i] = &coretools.Tools{
			Version: m.Version,
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
		if args.MajorVersion > 0 && tools.Version.Major != args.MajorVersion {
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
	envUUID            string
	apiHostPortsGetter APIHostPortsGetter
}

// NewToolsURLGetter creates a new ToolsURLGetter that
// returns tools URLs pointing at an API server.
func NewToolsURLGetter(envUUID string, a APIHostPortsGetter) *toolsURLGetter {
	return &toolsURLGetter{envUUID, a}
}

func (t *toolsURLGetter) ToolsURL(v version.Binary) (string, error) {
	apiHostPorts, err := t.apiHostPortsGetter.APIHostPorts()
	if err != nil {
		return "", err
	}
	if len(apiHostPorts) == 0 {
		return "", errors.New("no API host ports")
	}
	// TODO(axw) return all known URLs, so clients can try each one.
	//
	// The clients currently accept a single URL; we should change
	// the clients to disregard the URL, and have them download
	// straight from the API server.
	//
	// For now we choose a API server at random, and then select its
	// cloud-local address. The only user that will care about the URL
	// is the upgrader, and that is cloud-local.
	hostPorts := apiHostPorts[rand.Int()%len(apiHostPorts)]
	apiAddress := network.SelectInternalHostPort(hostPorts, false)
	if apiAddress == "" {
		return "", errors.Errorf("no suitable API server address to pick from %v", hostPorts)
	}
	serverRoot := fmt.Sprintf("https://%s/environment/%s", apiAddress, t.envUUID)
	return ToolsURL(serverRoot, v), nil
}

// ToolsURL returns a tools URL pointing the API server
// specified by the "serverRoot".
func ToolsURL(serverRoot string, v version.Binary) string {
	return fmt.Sprintf("%s/tools/%s", serverRoot, v.String())
}
