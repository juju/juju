// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils"
)

const ActionTagKind = "action"

type ActionTag struct {
	// Tags that are serialized need to have fields exported.
	ID utils.UUID
}

// NewActionTag returns the tag of an action with the given id (UUID).
func NewActionTag(id string) ActionTag {
	uuid, err := utils.UUIDFromString(id)
	if err != nil {
		panic(err)
	}
	return ActionTag{ID: uuid}
}

// ParseActionTag parses an action tag string.
func ParseActionTag(actionTag string) (ActionTag, error) {
	tag, err := ParseTag(actionTag)
	if err != nil {
		return ActionTag{}, err
	}
	at, ok := tag.(ActionTag)
	if !ok {
		return ActionTag{}, invalidTagError(actionTag, ActionTagKind)
	}
	return at, nil
}

func (t ActionTag) String() string { return t.Kind() + "-" + t.Id() }
func (t ActionTag) Kind() string   { return ActionTagKind }
func (t ActionTag) Id() string     { return t.ID.String() }

// IsValidAction returns whether id is a valid action id (UUID).
func IsValidAction(id string) bool {
	return utils.IsValidUUIDString(id)
}

// ActionReceiverTag returns an ActionReceiver Tag from a
// machine or unit name.
func ActionReceiverTag(name string) (Tag, error) {
	if IsValidUnit(name) {
		return NewUnitTag(name), nil
	}
	if IsValidApplication(name) {
		// TODO(jcw4) enable when leader elections complete
		//return NewApplicationTag(name), nil
	}
	if IsValidMachine(name) {
		return NewMachineTag(name), nil
	}
	return nil, fmt.Errorf("invalid actionreceiver name %q", name)
}

// ActionReceiverFrom Tag returns an ActionReceiver tag from
// a machine or unit tag.
func ActionReceiverFromTag(tag string) (Tag, error) {
	unitTag, err := ParseUnitTag(tag)
	if err == nil {
		return unitTag, nil
	}
	machineTag, err := ParseMachineTag(tag)
	if err == nil {
		return machineTag, nil
	}
	return nil, errors.Errorf("invalid actionreceiver tag %q", tag)
}
