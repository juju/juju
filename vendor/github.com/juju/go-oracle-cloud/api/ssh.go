// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// CreateSSHKey adds into the oracle cloud account an ssh key
func (c *Client) CreateSHHKey(
	name string,
	key string,
	enabled bool,
) (resp response.SSH, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty ssh key name")
	}

	if key == "" {
		return resp, errors.New("go-oracle-cloud: Empty ssh key provided is empty")
	}

	ssh := struct {
		Enabled bool   `json:"enabled"`
		Key     string `json:"key"`
		Name    string `json:"name"`
	}{
		Enabled: enabled,
		Key:     key,
		Name:    name,
	}

	url := c.endpoints["sshkey"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		body: &ssh,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteSSHKey deteles a ssh key with a specific name
func (c *Client) DeleteSSHKey(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-cloud: Empty ssh key name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["sshkey"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// SSHKeyDetails returns all details of a specific key
func (c *Client) SSHKeyDetails(name string) (resp response.SSH, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty ssh key name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["sshkey"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllSShKeys returns list of all keys with all the details
// You can filter by name
func (c *Client) AllSSHKeys(filter []Filter) (resp response.AllSSH, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["sshkey"], c.identify, c.username)

	if err = c.request(paramsRequest{
		url:    url,
		verb:   "GET",
		resp:   &resp,
		filter: filter,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllSSHKeyNames returns a list of all ssh keys by names of the user
func (c *Client) AllSSHKeyNames() (resp response.AllSSHNames, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["sshkey"], c.identify, c.username)

	if err = c.request(paramsRequest{
		directory: true,
		url:       url,
		verb:      "GET",
		resp:      &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// UpdateSSHKey change the content and details of a specific ssh key
// If the key is invalid it will retrun 400 status code. Make sure the key is a valid ssh public key
func (c *Client) UpdateSSHKey(
	currentName string,
	newName string,
	key string,
	enabled bool,
) (resp response.SSH, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if currentName == "" {
		return resp, errors.New("go-oracle-cloud: Empty ssh key name")
	}

	if key == "" {
		return resp, errors.New("go-oracle-cloud: ssh key provided is empty")
	}

	if newName == "" {
		newName = currentName
	}

	ssh := struct {
		Enabled bool   `json:"enabled"`
		Key     string `json:"key"`
		Name    string `json:"name"`
	}{
		Enabled: enabled,
		Key:     key,
		Name:    newName,
	}

	url := fmt.Sprintf("%s%s", c.endpoints["sshkey"], currentName)

	if err = c.request(paramsRequest{
		body: &ssh,
		url:  url,
		verb: "PUT",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
