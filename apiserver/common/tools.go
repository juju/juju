// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"math/rand"
	"path"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
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

// ToolsStorager is an interface providing the ToolsStorage method.
type ToolsStorager interface {
	// ToolsStorage returns a toolstorage.ToolsStorage.
	ToolsStorage() (toolstorage.StorageCloser, error)
}

// ToolsGetter implements a common Tools method for use by various
// facades.
type ToolsGetter struct {
	entityFinder  state.EntityFinder
	configGetter  EnvironConfigGetter
	toolsStorager ToolsStorager
	urlGetter     ToolsURLGetter
	getCanRead    GetAuthFunc
}

// NewToolsGetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsGetter(f state.EntityFinder, c EnvironConfigGetter, s ToolsStorager, t ToolsURLGetter, getCanRead GetAuthFunc) *ToolsGetter {
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
	toolsStorage, err := t.toolsStorager.ToolsStorage()
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
			var url string
			url, err = t.urlGetter.ToolsURL(agentTools.Version)
			if err == nil {
				agentTools.URL = url
				result.Results[i].Tools = agentTools
				// TODO(axw) Get rid of this in 1.22, when all clients
				// are known to ignore the flag.
				result.Results[i].DisableSSLHostnameVerification = true
			}
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
	allMetadata, err := storage.AllMetadata()
	if err != nil {
		return nil, err
	}
	list, err := findMatchingTools(allMetadata, t.urlGetter, params.FindToolsParams{
		Number:       agentVersion,
		MajorVersion: -1,
		MinorVersion: -1,
		Series:       existingTools.Version.Series,
		Arch:         existingTools.Version.Arch,
	})
	if err == coretools.ErrNoMatches {
		err = errors.NewNotFound(err, "tools not found")
	}
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
	toolsStorager ToolsStorager
	urlGetter     ToolsURLGetter
}

// NewToolsFinder returns a new ToolsFinder, returning tools
// with their URLs pointing at the API server.
func NewToolsFinder(s ToolsStorager, t ToolsURLGetter) *ToolsFinder {
	return &ToolsFinder{s, t}
}

// FindTools returns a List containing all tools matching the given parameters.
func (f *ToolsFinder) FindTools(args params.FindToolsParams) (params.FindToolsResult, error) {
	result := params.FindToolsResult{}
	storage, err := f.toolsStorager.ToolsStorage()
	if err != nil {
		return result, err
	}
	defer storage.Close()
	allMetadata, err := storage.AllMetadata()
	if err != nil {
		return result, err
	}
	list, err := findMatchingTools(allMetadata, f.urlGetter, args)
	if err == coretools.ErrNoMatches {
		err = errors.NewNotFound(err, "tools not found")
	}
	if err != nil {
		result.Error = ServerError(err)
	} else {
		result.List = list
	}
	return result, nil
}

func findMatchingTools(metadata []toolstorage.Metadata, urlGetter ToolsURLGetter, args params.FindToolsParams) (coretools.List, error) {
	list := make(coretools.List, len(metadata))
	for i, m := range metadata {
		url, err := urlGetter.ToolsURL(m.Version)
		if err != nil {
			return nil, err
		}
		tools := &coretools.Tools{
			Version: m.Version,
			Size:    m.Size,
			SHA256:  m.SHA256,
			URL:     url,
		}
		list[i] = tools
	}
	sort.Sort(list)
	filter := coretools.Filter{
		Number: args.Number,
		Arch:   args.Arch,
		Series: args.Series,
	}
	list, err := list.Match(filter)
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
	serverRoot := fmt.Sprintf("https://%s/environment/%s", apiAddress, t.envUUID)
	return ToolsURL(serverRoot, v), nil
}

// ToolsURL returns a tools URL pointing the API server
// specified by the "serverRoot".
func ToolsURL(serverRoot string, v version.Binary) string {
	return path.Join(serverRoot, "tools", v.String())
}
