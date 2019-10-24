package spacenamer

import (
	"github.com/juju/juju/api/base"
	apispacenamer "github.com/juju/juju/api/spacenamer"
)

func NewClient(apiCaller base.APICaller) SpaceNamerAPI {
	facade := apispacenamer.NewClient(apiCaller)
	return facade
}
