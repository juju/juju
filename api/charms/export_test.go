package charms

import "github.com/juju/juju/api/base"

func NewClientWithFacade(facade base.FacadeCaller) *Client {
	return &Client{facade: facade}
}
