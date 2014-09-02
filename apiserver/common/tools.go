// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/state"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/names"
)

var envtoolsFindTools = envtools.FindTools

type EntityFinderEnvironConfigGetter interface {
	state.EntityFinder
	EnvironConfigGetter
}

type EnvironConfigGetter interface {
	EnvironConfig() (*config.Config, error)
}

// ToolsGetter implements a common Tools method for use by various
// facades.
type ToolsGetter struct {
	st         EntityFinderEnvironConfigGetter
	getCanRead GetAuthFunc
}

// NewToolsGetter returns a new ToolsGetter. The GetAuthFunc will be
// used on each invocation of Tools to determine current permissions.
func NewToolsGetter(st EntityFinderEnvironConfigGetter, getCanRead GetAuthFunc) *ToolsGetter {
	return &ToolsGetter{
		st:         st,
		getCanRead: getCanRead,
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
	agentVersion, cfg, err := t.getGlobalAgentVersion()
	if err != nil {
		return result, err
	}
	// SSLHostnameVerification defaults to true, so we need to
	// invert that, for backwards-compatibility (older versions
	// will have DisableSSLHostnameVerification: false by default).
	disableSSLHostnameVerification := !cfg.SSLHostnameVerification()
	env, err := environs.New(cfg)
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		agentTools, err := t.oneAgentTools(canRead, tag, agentVersion, env)
		if err == nil {
			result.Results[i].Tools = agentTools
			result.Results[i].DisableSSLHostnameVerification = disableSSLHostnameVerification
		}
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}

func (t *ToolsGetter) getGlobalAgentVersion() (version.Number, *config.Config, error) {
	// Get the Agent Version requested in the Environment Config
	nothing := version.Number{}
	cfg, err := t.st.EnvironConfig()
	if err != nil {
		return nothing, nil, err
	}
	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		return nothing, nil, fmt.Errorf("agent version not set in environment config")
	}
	return agentVersion, cfg, nil
}

func (t *ToolsGetter) oneAgentTools(canRead AuthFunc, tag names.Tag, agentVersion version.Number, env environs.Environ) (*coretools.Tools, error) {
	if !canRead(tag) {
		return nil, ErrPerm
	}
	entity, err := t.st.FindEntity(tag)
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
	// TODO(jam): Avoid searching the provider for every machine
	// that wants to upgrade. The information could just be cached
	// in state, or even in the API servers
	return envtools.FindExactTools(env, agentVersion, existingTools.Version.Series, existingTools.Version.Arch)
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

// FindTools returns a List containing all tools matching the given parameters.
func FindTools(e EnvironConfigGetter, args params.FindToolsParams) (params.FindToolsResult, error) {
	result := params.FindToolsResult{}
	// Get the existing environment config from the state.
	envConfig, err := e.EnvironConfig()
	if err != nil {
		return result, err
	}
	env, err := environs.New(envConfig)
	if err != nil {
		return result, err
	}
	filter := coretools.Filter{
		Number: args.Number,
		Arch:   args.Arch,
		Series: args.Series,
	}
	result.List, err = envtoolsFindTools(env, args.MajorVersion, args.MinorVersion, filter, envtools.DoNotAllowRetry)
	result.Error = ServerError(err)
	return result, nil
}
