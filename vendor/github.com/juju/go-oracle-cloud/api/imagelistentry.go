// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// ImageListEntryDetails retrieves details of the specified image list entry.
func (c *Client) ImageListEntryDetails(
	name string,
	version int,
) (resp response.ImageListEntry, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty image list entry name",
		)
	}

	if version == 0 {
		return resp, errors.New(
			"go-oracle-cloud: Empty image list entry version",
		)
	}

	url := fmt.Sprintf("%s%s/entry/%d",
		c.endpoints["imagelistentrie"], name, version)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteImageListEntry deletes an Image List Entry
func (c *Client) DeleteImageListEntry(
	name string,
	version int,
) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud: Empty image list entry name",
		)
	}

	if version == 0 {
		return errors.New(
			"go-oracle-cloud: Empty image list entry verion",
		)
	}

	url := fmt.Sprintf("%s%s/entry/%d",
		c.endpoints["imagelistentrie"], name, version)

	if err = c.request(paramsRequest{
		verb: "DELETE",
		url:  url,
	}); err != nil {
		return err
	}

	return nil
}

// CreateImageListEntry adds an image list entry to Oracle Compute Cloud
// Each machine image in an image list is identified by an image list entry.
func (c *Client) CreateImageListEntry(
	name string,
	attributes map[string]interface{},
	version int,
	machineImages []string,
) (resp response.ImageListEntryAdd, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty image list entry name",
		)
	}

	if machineImages == nil {
		return resp, errors.New(
			"go-oracle-cloud: Empty image list entry machine images",
		)
	}

	if version == 0 {
		return resp, errors.New(
			"go-oracle-cloud: Empty image list entry verion",
		)
	}

	params := struct {
		Attributes    map[string]interface{} `json:"attributes,omitempty"`
		MachineImages []string               `json:"machineimages"`
		Version       int                    `json:"version"`
	}{
		Attributes:    attributes,
		MachineImages: machineImages,
		Version:       version,
	}

	url := fmt.Sprintf("%s%s/entry/",
		c.endpoints["imagelistentrie"], name)

	if err = c.request(paramsRequest{
		verb: "POST",
		url:  url,
		resp: &resp,
		body: &params,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
