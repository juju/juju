// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bundle provides access to the bundle api facade.
// This facade contains api calls that are specific to bundles.
package bundle

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
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
	if bestVer := c.BestAPIVersion(); bestVer < 4 {
		return result, errors.Errorf("this controller version does not support bundle get changes as map args feature.")
	}
	if err := c.facade.FacadeCall("GetChangesMapArgs", params.BundleChangesParams{
		BundleURL:      bundleURL,
		BundleDataYAML: bundleDataYAML,
	}, &result); err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

// ExportBundle exports the current model configuration.
func (c *Client) ExportBundle() (string, error) {
	var result params.StringResult
	if bestVer := c.BestAPIVersion(); bestVer < 2 {
		return "", errors.Errorf("this controller version does not support bundle export feature.")
	}

	if err := c.facade.FacadeCall("ExportBundle", nil, &result); err != nil {
		return "", errors.Trace(err)
	}

	if result.Error != nil {
		return "", errors.Trace(result.Error)
	}

	return result.Result, nil
}
