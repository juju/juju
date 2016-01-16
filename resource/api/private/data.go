package private

import (
	"github.com/juju/juju/apiserver/params"

	"github.com/juju/juju/resource/api"
)

type ListResourcesArgs struct {
	ResourceNames []string
}

type ResourcesResult struct {
	params.ErrorResult

	Resources []ResourceResult
}

type ResourceResult struct {
	params.ErrorResult

	Resource api.Resource
}
