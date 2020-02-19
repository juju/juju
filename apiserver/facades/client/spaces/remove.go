// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// RemoveSpace describes a space that can be removed.
type RemoveSpace interface {
	Refresh() error
	RemoveSpaceOps() []txn.Op
}

type spaceRemoveModelOp struct {
	space   RemoveSpace
	subnets []RemoveSubnet
}

// RemoveSubnet describes a subnet which can be moved
type RemoveSubnet interface {
	MoveSubnetOps(spaceID string) []txn.Op
}

func (o *spaceRemoveModelOp) Done(err error) error {
	return err
}

func NewRemoveSpaceModelOp(space RemoveSpace, subnets []RemoveSubnet) *spaceRemoveModelOp {
	return &spaceRemoveModelOp{
		space:   space,
		subnets: subnets,
	}
}

func (sp *spaceRemoveModelOp) Build(attempt int) ([]txn.Op, error) {
	var totalOps []txn.Op

	if attempt > 0 {
		if err := sp.space.Refresh(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	for _, subnet := range sp.subnets {
		totalOps = append(totalOps, subnet.MoveSubnetOps(network.AlphaSpaceId)...)
	}

	removeOps := sp.space.RemoveSpaceOps()
	totalOps = append(totalOps, removeOps...)
	return totalOps, nil
}

// RemoveSpace removes a space.
// Returns SpaceResults if entities/settings are found which makes the deletion not possible.
func (api *API) RemoveSpace(entities params.Entities) (params.RemoveSpaceResults, error) {
	var results params.RemoveSpaceResults

	err := api.checkSpacesCRUDPermissions()
	if err != nil {
		return results, err
	}

	results = params.RemoveSpaceResults{
		Results: make([]params.RemoveSpaceResult, len(entities.Entities)),
	}
	for i, entity := range entities.Entities {
		spacesTag, err := names.ParseSpaceTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		removable := api.checkSpaceIsRemovable(i, spacesTag, &results)
		if !removable {
			continue
		}

		operation, err := api.opFactory.NewRemoveSpaceModelOp(spacesTag.Id())
		if err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}
		if err = api.backing.ApplyOperation(operation); err != nil {
			results.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}
	}
	return results, nil
}

func (api *API) constraintsTagForSpaceName(name string) ([]names.Tag, error) {
	cons, err := api.backing.ConstraintsBySpaceName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	tags := make([]names.Tag, len(cons))
	for i, doc := range cons {
		tag := state.ParseLocalIDToTags(doc.ID())
		if tag == nil {
			return nil, errors.Errorf("Could not parse id: %q", doc.ID())
		}
		tags[i] = tag
	}
	return tags, nil
}

func (api *API) checkSpaceIsRemovable(index int, spacesTag names.Tag, results *params.RemoveSpaceResults) bool {
	removable := true
	space, err := api.backing.SpaceByName(spacesTag.Id())
	if err != nil {
		results.Results[index].Error = common.ServerError(errors.Trace(err))
		return false
	}
	bindingTags, err := api.getApplicationTagsPerSpace(space.Id())
	if err != nil {
		results.Results[index].Error = common.ServerError(errors.Trace(err))
		return false
	}
	constraintTags, err := api.getConstraintsTagsPerSpace(space.Name())
	if err != nil {
		results.Results[index].Error = common.ServerError(errors.Trace(err))
		return false
	}
	settingMatches, err := api.getSpaceControllerSettings(space.Name())
	if err != nil {
		results.Results[index].Error = common.ServerError(errors.Trace(err))
		return false
	}

	if len(settingMatches) != 0 {
		results.Results[index].ControllerSettings = settingMatches
		removable = false
	}
	if len(bindingTags) != 0 {
		results.Results[index].Bindings = convertTagsToEntities(bindingTags)
		removable = false
	}
	if len(constraintTags) != 0 {
		results.Results[index].Constraints = convertTagsToEntities(constraintTags)
		removable = false
	}
	return removable
}

func (api *API) getApplicationTagsPerSpace(spaceID string) ([]names.Tag, error) {
	applications, err := api.getApplicationsBindSpace(spaceID)
	if err != nil {
		return nil, errors.Trace(nil)
	}
	tags := make([]names.Tag, len(applications))
	for i, app := range applications {
		tags[i] = names.NewApplicationTag(app)
	}
	return tags, nil
}

func convertTagsToEntities(tags []names.Tag) []params.Entity {
	entities := make([]params.Entity, len(tags))
	for i, tag := range tags {
		entities[i].Tag = tag.String()
	}

	return entities
}

func (api *API) getConstraintsTagsPerSpace(spaceName string) ([]names.Tag, error) {
	tags, err := api.constraintsTagForSpaceName(spaceName)
	var notSkipping []names.Tag
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, tag := range tags {
		if tag.Kind() == names.UnitTagKind {
			continue
		}
		if tag.Kind() == names.MachineTagKind {
			continue
		}
		notSkipping = append(notSkipping, tag)
	}
	return notSkipping, nil
}

func (api *API) getSpaceControllerSettings(spaceName string) ([]string, error) {
	var matches []string

	if !api.backing.IsController() {
		return matches, nil
	}

	currentControllerConfig, err := api.backing.ControllerConfig()
	if err != nil {
		return matches, errors.Trace(err)
	}

	if mgmtSpace := currentControllerConfig.JujuManagementSpace(); mgmtSpace == spaceName {
		matches = append(matches, controller.JujuManagementSpace)
	}
	if haSpace := currentControllerConfig.JujuHASpace(); haSpace == spaceName {
		matches = append(matches, controller.JujuHASpace)
	}
	return matches, nil
}
