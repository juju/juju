// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/connection_mock.go github.com/juju/juju/api Connection
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/stream_mock.go github.com/juju/juju/api/base Stream

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

func NewClientFromAPIConnection(conn api.Connection) *Client {
	return &Client{
		conn: conn,
	}
}

func NewClientFromFacadeCaller(facade base.FacadeCaller) *Client {
	return &Client{
		facade: facade,
	}
}
