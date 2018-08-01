// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// ShapeDetails retrieves the CPU and memory details of the specified shape.
func (c *Client) ShapeDetails(name string) (resp response.Shape, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty shape name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["shape"], name)

	if err = c.request(paramsRequest{
		verb: "GET",
		url:  url,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllShapes retrieves the CPU and memory details of all the available shapes.
func (c *Client) AllShapes(filter []Filter) (resp response.AllShapes, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := c.endpoints["shape"] + "/"

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
