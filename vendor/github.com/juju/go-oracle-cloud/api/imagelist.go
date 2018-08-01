// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// ImageListDetails retrieves details of the specified image list.
// You can also use this request to retrieve details of all the available
// image list entries in the specified image list.
func (c *Client) ImageListDetails(
	name string,
) (resp response.ImageList, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-api: Empty image list name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["imagelist"], name)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllImageLists retrieves details of all the available
// image lists in the specified container.
func (c *Client) AllImageLists(filter []Filter) (resp response.AllImageLists, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["imagelist"], c.identify, c.username)

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

// ImageListNames retrieves the names of objects and
// subcontainers that you can access in the specified container.
func (c *Client) ImageListNames() (resp response.DirectoryNames, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/", c.endpoints["imagelist"],
		c.identify, c.username)

	if err = c.request(paramsRequest{
		directory: true,
		verb:      "GET",
		url:       url,
		resp:      &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// CreateImageList Adds an image list to Oracle Compute Cloud Service.
func (c *Client) CreateImageList(
	def int,
	description string,
	name string,
) (resp response.ImageList, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty image list name",
		)
	}

	params := struct {
		Def         int    `json:"default"`
		Description string `json:"description"`
		Name        string `json:"name"`
	}{
		Def:         def,
		Description: description,
		Name:        name,
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["imagelist"], c.identify, c.username)

	if err = c.request(paramsRequest{
		verb: "POST",
		url:  url,
		body: &params,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteImageList deletes an image list
// You can't delete system-provided image lists
// that are available in the /oracle/public container.
func (c *Client) DeleteImageList(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-api: Empty image list name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["imagelist"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// UpdateImageList updates the description of an image list.
// You can also update the default image list entry to be used
// while launching instances using the specified image list.
func (c *Client) UpdateImageList(
	currentName string,
	newName string,
	description string,
	def int,
) (resp response.ImageList, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if currentName == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty image list current name",
		)
	}

	if newName == "" {
		newName = currentName
	}

	params := struct {
		Def         int    `json:"default"`
		Description string `json:"description,omitempty"`
		Name        string `json:"name"`
	}{
		Def:         def,
		Description: description,
		Name:        newName,
	}

	url := fmt.Sprintf("%s%s", c.endpoints["imagelist"], newName)

	if err = c.request(paramsRequest{
		verb: "PUT",
		url:  url,
		body: &params,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
