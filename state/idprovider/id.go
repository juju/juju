package idprovider

import (
	"fmt"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

type IdentityProvider interface {
	Login(st *state.State, tag names.Tag, password, nonce string) error
}

func LookupProvider(tag names.Tag) (IdentityProvider, error) {
	switch tag.Kind() {
	case names.MachineTagKind, names.UnitTagKind, names.UserTagKind:
		return NewAgentIdentityProvider(), nil
	}
	return nil, fmt.Errorf("Tag type '%s' does not have an identity provider", tag.Kind())
}
