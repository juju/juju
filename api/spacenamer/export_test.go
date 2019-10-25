package spacenamer

import (
	"github.com/juju/juju/api/base"
)

func NewStateFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade: caller,
	}
}
