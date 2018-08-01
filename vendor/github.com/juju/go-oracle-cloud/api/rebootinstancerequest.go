// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

//You can reboot a running instance by creating a rebootinstancerequest object.

// CreateRebootInstanceRequest is used when we want to launch a restart on a instnace
// If your instance hangs after it starts running, you can use this request to reboot
// your instance. After creating this request, use GET /rebootinstancerequest/{name}
// to retrieve the status of the request. When the status of the rebootinstancerequest
// changes to complete, you know that the instance has been rebooted.
func (c *Client) CreateRebootInstanceRequest(
	hard bool,
	instanceName string,
) (resp response.RebootInstanceRequest, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if instanceName == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty reboot instance request name",
		)
	}

	url := c.endpoints["rebootinstancerequest"] + "/"

	params := struct {
		Name string `json:"name"`
		Hard bool   `json:"hard"`
	}{
		Name: instanceName,
		Hard: hard,
	}

	if err = c.request(paramsRequest{
		url:  url,
		body: &params,
		resp: &resp,
		verb: "POST",
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteRebootInstanceRequest deletes a reboot instance request.
// No response is returned for the delete action.
func (c *Client) DeleteRebootInstanceRequest(instanceName string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if instanceName == "" {
		return errors.New("go-oracle-cloud: Empty reboot instance request name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["rebootinstancerequest"], instanceName)

	if err = c.request(paramsRequest{
		verb: "DELETE",
		url:  url,
	}); err != nil {
		return err
	}

	return nil
}

// RebootInstanceRequestDetails retrieves details of the specified reboot instance request.
// You can use this request when you want to find out the status of a reboot instance request.
func (c *Client) RebootInstanceRequestDetails(
	instanceName string,
) (resp response.RebootInstanceRequest, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if instanceName == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty reboot instance request name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["rebootinstancerequest"], instanceName)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

//AllRebootInstanceRequests retrieves details of the reboot instance requests that are available in the specified container
// You can filte by hard, instance, and name
func (c *Client) AllRebootInstanceRequests(filter []Filter) (resp response.AllRebootInstanceRequests, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["rebootinstancerequest"], c.identify, c.username)

	if err = c.request(paramsRequest{
		verb:   "GET",
		url:    url,
		resp:   &resp,
		filter: filter,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
