package uniter

import "github.com/juju/juju/apiserver/common"

func init() {
	// We can re-use the v1 API.
	common.RegisterStandardFacade("Uniter", 2, NewUniterAPIV1)
}
