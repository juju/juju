// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"context"
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	conf "github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	envutils "github.com/juju/juju/environs/utils"
	"github.com/juju/juju/juju/keys"
)

type toolsDatasourceFuncId struct {
	id string
	f  ToolsDataSourceFunc
}

var (
	toolsDatasourceFuncsMu sync.RWMutex
	toolsDatasourceFuncs   []toolsDatasourceFuncId
)

// ToolsDataSourceFunc is a function type that takes an environment and
// returns a simplestreams datasource.
//
// ToolsDataSourceFunc will be used in GetMetadataSources.
// Any error satisfying errors.IsNotSupported will be ignored;
// any other error will be cause GetMetadataSources to fail.
type ToolsDataSourceFunc func(environs.Environ) (simplestreams.DataSource, error)

// RegisterToolsDataSourceFunc registers an ToolsDataSourceFunc
// with the specified id, overwriting any function previously registered
// with the same id.
func RegisterToolsDataSourceFunc(id string, f ToolsDataSourceFunc) {
	toolsDatasourceFuncsMu.Lock()
	defer toolsDatasourceFuncsMu.Unlock()
	for i := range toolsDatasourceFuncs {
		if toolsDatasourceFuncs[i].id == id {
			toolsDatasourceFuncs[i].f = f
			return
		}
	}
	toolsDatasourceFuncs = append(toolsDatasourceFuncs, toolsDatasourceFuncId{id, f})
}

// UnregisterToolsDataSourceFunc unregisters an ToolsDataSourceFunc
// with the specified id.
func UnregisterToolsDataSourceFunc(id string) {
	toolsDatasourceFuncsMu.Lock()
	defer toolsDatasourceFuncsMu.Unlock()
	for i, f := range toolsDatasourceFuncs {
		if f.id == id {
			head := toolsDatasourceFuncs[:i]
			tail := toolsDatasourceFuncs[i+1:]
			toolsDatasourceFuncs = append(head, tail...)
			return
		}
	}
}

// GetMetadataSources returns the sources to use when looking for
// simplestreams tools metadata for the given stream.
func GetMetadataSources(env environs.BootstrapEnviron, dataSourceFactory simplestreams.DataSourceFactory) ([]simplestreams.DataSource, error) {
	config := env.Config()

	// Add configured and environment-specific datasources.
	var sources []simplestreams.DataSource
	if userURL, ok := config.AgentMetadataURL(); ok {
		dataSourceConfig := simplestreams.Config{
			Description:          conf.AgentMetadataURLKey,
			BaseURL:              userURL,
			PublicSigningKey:     keys.JujuPublicKey,
			HostnameVerification: config.SSLHostnameVerification(),
			Priority:             simplestreams.SPECIFIC_CLOUD_DATA,
		}
		if err := dataSourceConfig.Validate(); err != nil {
			return nil, errors.Annotate(err, "simplestreams config validation failed")
		}
		dataSource := dataSourceFactory.NewDataSource(dataSourceConfig)
		sources = append(sources, dataSource)
	}

	envDataSources, err := environmentDataSources(context.TODO(), env)
	if err != nil {
		return nil, err
	}
	sources = append(sources, envDataSources...)

	// Add the default, public datasource.
	defaultURL, err := ToolsURL(DefaultBaseURL)
	if err != nil {
		return nil, err
	}
	if defaultURL != "" {
		dataSourceConfig := simplestreams.Config{
			Description:          "default simplestreams",
			BaseURL:              defaultURL,
			PublicSigningKey:     keys.JujuPublicKey,
			HostnameVerification: true,
			Priority:             simplestreams.DEFAULT_CLOUD_DATA,
			RequireSigned:        DefaultBaseURL == streamsAgentURL,
		}
		if err := dataSourceConfig.Validate(); err != nil {
			return nil, errors.Annotate(err, "simplestreams config validation failed")
		}
		dataSource := dataSourceFactory.NewDataSource(dataSourceConfig)
		sources = append(sources, dataSource)
	}
	return sources, nil
}

// environmentDataSources returns simplestreams datasources for the environment
// by calling the functions registered in RegisterToolsDataSourceFunc.
// The datasources returned will be in the same order the functions were registered.
func environmentDataSources(ctx context.Context, bootstrapEnviron environs.BootstrapEnviron) ([]simplestreams.DataSource, error) {
	toolsDatasourceFuncsMu.RLock()
	defer toolsDatasourceFuncsMu.RUnlock()

	var datasources []simplestreams.DataSource
	env, ok := bootstrapEnviron.(environs.Environ)
	if !ok {
		logger.Debugf(ctx, "environmentDataSources is supported for IAAS, environ %#v is not Environ", bootstrapEnviron)
		// ignore for CAAS
		return datasources, nil
	}
	for _, f := range toolsDatasourceFuncs {
		logger.Debugf(ctx, "trying datasource %q", f.id)
		datasource, err := f.f(env)
		if err != nil {
			if errors.Is(err, errors.NotSupported) {
				continue
			}
			return nil, err
		}
		datasources = append(datasources, datasource)
	}
	return datasources, nil
}

// ToolsURL returns a valid tools URL constructed from source.
// source may be a directory, or a URL like file://foo or http://foo.
func ToolsURL(source string) (string, error) {
	if source == "" {
		return "", nil
	}

	return envutils.GetURL(source, storage.BaseToolsPath)
}
