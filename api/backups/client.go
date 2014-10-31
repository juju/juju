// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"net/http"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

type httpClient interface {
	SendHTTPRequest(method, path string, args interface{}) (*http.Request, *http.Response, error)
}

// Client wraps the backups API for the client.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	http   httpClient
}

type httpAPICallCloser interface {
	base.APICallCloser
	httpClient
}

// NewClient returns a new backups API client.
func NewClient(st httpAPICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Backups")
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		http:         st,
	}
}

// Restore is responsable for finishing a restore after a placeholder
// machine has been bootstraped, it receives the name of a backup
// file on server and will return error on failure.
func (c *Client) Restore(backupFileName, backupId string) error {
	params := params.RestoreArgs{
			FileName: backupFileName,
			BackupId: backupId,
			Machine:  "0"}
	err := c.facade.FacadeCall("Restore", params, nil)
	return err
}

// Preparerestore puts the server into a state that only allows
// for restore to be called. This is to avoid the data loss if
// users try to perform actions that are going to be overwritten
// by restore
func (c *Client) PrepareRestore() error {
	return c.facade.FacadeCall("PrepareRestore", nil, nil)
}

// FinishRestore since Restore call will end up with a reset
// state server, finish restore will check that the the newly
// placed state server has the mark of restore complete
func (c *Client) FinishRestore() error {
	return c.facade.FacadeCall("FinishRestore", nil, nil)
}

// PublicAddress returns the public address of the specified
// machine or unit. For a machine, target is an id not a tag.
func (c *Client) PublicAddress(target string) (string, error) {
	var results params.PublicAddressResults
	p := params.PublicAddress{Target: target}
	err := c.facade.FacadeCall("PublicAddress", p, &results)
	return results.PublicAddress, err
}
