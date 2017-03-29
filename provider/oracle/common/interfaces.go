package common

import "github.com/juju/go-oracle-cloud/response"

type Instancer interface {
	InstanceDetails(string) (response.Instance, error)
}

type Composer interface {
	ComposeName(string) string
}
