// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	//"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

type UnitStatusSetter struct {
	StatusSetter
}

// NewUnitStatusSetter returns a new UnitStatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewUnitStatusSetter(st state.EntityFinder, getCanModify GetAuthFunc) *UnitStatusSetter {
	statusSetter := &UnitStatusSetter{
		StatusSetter{
			st:           st,
			getCanModify: getCanModify,
		},
	}
	setterFunc := func() SetterFunc { return statusSetter.setStatus }
	statusSetter.getSetterFunc = setterFunc
	return statusSetter

}

func (s *UnitStatusSetter) setUnitEntityStatus(tag names.UnitTag, status params.Status, info string, data map[string]interface{}) error {
	entity, err := s.st.FindEntity(tag)
	if err != nil {
		return err
	}
	switch entity := entity.(type) {
	case state.StatusSetter:
		return entity.SetStatus(state.Status(status), info, data)
	default:
		return NotSupportedError(tag, fmt.Sprintf("setting unit status, %T", entity))
	}
}

func (s *UnitStatusSetter) setStatus(tag names.Tag, status params.Status, info string, data map[string]interface{}) error {
	unitTag := tag.(names.UnitTag)
	return s.setUnitEntityStatus(unitTag, status, info, data)
}
