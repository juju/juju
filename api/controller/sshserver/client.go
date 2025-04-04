// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Client holds the SSH server client for it's respective worker.
type Client struct {
	facade base.FacadeCaller
	*common.ControllerConfigAPI
}

// NewClient returns an SSH server facade client.
func NewClient(caller base.APICaller) (*Client, error) {
	facadeCaller := base.NewFacadeCaller(caller, "SSHServer")
	return &Client{
		facade:              facadeCaller,
		ControllerConfigAPI: common.NewControllerConfig(facadeCaller),
	}, nil
}

// WatchControllerConfig provides a watcher for changes on controller config.
func (c *Client) WatchControllerConfig() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	if err := c.facade.FacadeCall("WatchControllerConfig", nil, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}

// SSHServerHostKey returns the private host key for the controller's SSH server.
func (c *Client) SSHServerHostKey() (string, error) {
	var result params.StringResult
	err := c.facade.FacadeCall("SSHServerHostKey", nil, &result)
	if err != nil {
		return "", err
	}
	if err := result.Error; err != nil {
		return "", err
	}
	return result.Result, nil
}

// HostKeyForTarget returns the private host key for the target machine/unit
func (c *Client) HostKeyForTarget(arg params.SSHHostKeyRequestArg) ([]byte, error) {
	var result params.SSHHostKeyResult
	err := c.facade.FacadeCall("HostKeyForTarget", arg, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	return result.HostKey, nil
}

// ListAuthorizedKeysForModel calls the ListAuthorizedKeysForModel facade, parses the authorized keys and returns the public keys
// in a slice.
func (c *Client) ListPublicKeysForModel(sshPKIAuthArgs params.ListAuthorizedKeysArgs) ([]gossh.PublicKey, error) {
	var result params.ListAuthorizedKeysResult
	publicKeys := make([]gossh.PublicKey, 0)
	err := c.facade.FacadeCall("ListAuthorizedKeysForModel", sshPKIAuthArgs, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	for _, key := range result.AuthorizedKeys {
		pubKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			continue
		}
		publicKeys = append(publicKeys, pubKey)
	}
	return publicKeys, nil
}
