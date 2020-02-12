//Copyright 2020 Canonical Ltd.
//Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/juju/apiserver/params"
	"gopkg.in/mgo.v2/txn"
)

// UpdateSpace describes a space that can be updated.
type UpdateSpace interface {
	Refresh() error
	Name() string
	UpdateSpaceOps(toName string) []txn.Op
}

func (api *API) UpdateSpace(args params.RenameSpacesParams) (params.ErrorResults, error) {
}
