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
	subnets []Subnet
}

type Subnet interface {
	UpdateSpaceOps(spaceID string) []txn.Op
}

func (o *spaceRemoveModelOp) Done(err error) error {
	return err
}

func NewRemoveSpaceOp(space RemoveSpace, subnets []Subnet) *spaceRemoveModelOp {
	return &spaceRemoveModelOp{
		space:   space,
		subnets: subnets,
	}
}

func (o *spaceRemoveModelOp) Build(attempt int) ([]txn.Op, error) {
	var totalOps []txn.Op

	if attempt > 0 {
		if err := o.space.Refresh(); err != nil {
			return nil, errors.Trace(err)
		}
	}

	for _, subnet := range o.subnets {
		totalOps = append(totalOps, subnet.UpdateSpaceOps(network.AlphaSpaceId)...)
	}

	removeOps := o.space.RemoveSpaceOps()
	totalOps = append(totalOps, removeOps...)
	return totalOps, nil
}

// RemoveSpace removes a space.
// Returns SpaceResults if entities/settings are found which makes the deletion not possible.
func (api *API) RemoveSpace(spaceParams params.RemoveSpaceParams) (params.RemoveSpaceResults, error) {
	result := params.RemoveSpaceResults{}

	if err := api.ensureSpacesAreMutable(); err != nil {
		return result, err
	}

	result.Results = make([]params.RemoveSpaceResult, len(spaceParams.SpaceParams))
	for i, spaceParam := range spaceParams.SpaceParams {
		spacesTag, err := names.ParseSpaceTag(spaceParam.Space.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		if !api.checkSpaceIsRemovable(i, spacesTag, &result, spaceParam.Force) {
			continue
		}

		operation, err := api.opFactory.NewRemoveSpaceOp(spacesTag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}

		if spaceParam.DryRun {
			continue
		}

		if err = api.backing.ApplyOperation(operation); err != nil {
			result.Results[i].Error = common.ServerError(errors.Trace(err))
			continue
		}
	}
	return result, nil
}

func (api *API) checkSpaceIsRemovable(
	index int, spacesTag names.Tag, results *params.RemoveSpaceResults, force bool,
) bool {
	removable := true

	if spacesTag.Id() == network.AlphaSpaceName {
		newErr := errors.New("the alpha space cannot be removed")
		results.Results[index].Error = common.ServerError(newErr)
		return false
	}
	space, err := api.backing.SpaceByName(spacesTag.Id())
	if err != nil {
		results.Results[index].Error = common.ServerError(errors.Trace(err))
		return false
	}
	bindingTags, err := api.applicationTagsForSpace(space.Id())
	if err != nil {
		results.Results[index].Error = common.ServerError(errors.Trace(err))
		return false
	}
	constraintTags, err := api.entityTagsForSpaceConstraintsBlockingRemove(space.Name())
	if err != nil {
		results.Results[index].Error = common.ServerError(errors.Trace(err))
		return false
	}
	settingMatches, err := api.getSpaceControllerSettings(space.Name())
	if err != nil {
		results.Results[index].Error = common.ServerError(errors.Trace(err))
		return false
	}

	if force {
		return true
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

// applicationTagsForSpace returns the tags for all applications with an
// endpoint bound to a space with the input name.
func (api *API) applicationTagsForSpace(spaceID string) ([]names.Tag, error) {
	applications, err := api.applicationsBoundToSpace(spaceID)
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

// entityTagsForSpaceConstraintsBlockingRemove returns tags for entities
// with constraints for the input space name, that disallow removal of the
// space. I.e. those other than units and machines.
func (api *API) entityTagsForSpaceConstraintsBlockingRemove(spaceName string) ([]names.Tag, error) {
	allTags, err := api.entityTagsForSpaceConstraints(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var notSkipping []names.Tag
	for _, tag := range allTags {
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

// entityTagsForSpaceConstraints returns the tags for all entities
// with constraints that refer to the input space name.
func (api *API) entityTagsForSpaceConstraints(spaceName string) ([]names.Tag, error) {
	cons, err := api.backing.ConstraintsBySpaceName(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	tags := make([]names.Tag, len(cons))
	for i, doc := range cons {
		tag := state.TagFromDocID(doc.ID())
		if tag == nil {
			return nil, errors.Errorf("Could not parse id: %q", doc.ID())
		}
		tags[i] = tag
	}
	return tags, nil
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
