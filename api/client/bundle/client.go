// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Client allows access to the bundle API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the bundle api.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Bundle")
	return &Client{
		ClientFacade: frontend,
		facade:       backend}
}

// GetChanges returns back the changes for a given bundle that need to be
// applied.
// GetChanges is superseded by GetChangesMapArgs, use that where possible, by
// detecting the BestAPIVersion to use.
func (c *Client) GetChanges(bundleURL, bundleDataYAML string) (params.BundleChangesResults, error) {
	var result params.BundleChangesResults
	if err := c.facade.FacadeCall("GetChanges", params.BundleChangesParams{
		BundleURL:      bundleURL,
		BundleDataYAML: bundleDataYAML,
	}, &result); err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

// GetChangesMapArgs returns back the changes for a given bundle that need to be
// applied, with the args of a method as a map.
func (c *Client) GetChangesMapArgs(bundleURL, bundleDataYAML string) (params.BundleChangesMapArgsResults, error) {
	var result params.BundleChangesMapArgsResults
	if err := c.facade.FacadeCall("GetChangesMapArgs", params.BundleChangesParams{
		BundleURL:      bundleURL,
		BundleDataYAML: bundleDataYAML,
	}, &result); err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

// ExportBundle exports the current model configuration.
func (c *Client) ExportBundle(includeDefaults bool, includeSeries bool) (string, error) {
	var result params.StringResult
	arg := params.ExportBundleParams{
		IncludeCharmDefaults: includeDefaults,
		IncludeSeries:        includeSeries,
	}
	if err := c.facade.FacadeCall("ExportBundle", arg, &result); err != nil {
		return "", errors.Trace(err)
	}

	if result.Error != nil {
		return "", errors.Trace(result.Error)
	}

	return result.Result, nil
}
