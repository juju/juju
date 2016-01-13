package server

import "github.com/juju/errors"

func NewUnitFacade(_ interface{}) *UnitFacade {
	return &UnitFacade{}
}

type UnitFacade struct {
}

func (uf UnitFacade) ResourceGet() error {
	return errors.NotImplementedf("not implemented yet")
}
