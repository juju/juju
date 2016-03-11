// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// TODO(ericsnow) Eliminate the dependence on apiserver if possible.

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/txn"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/state"
)

// Resource2API converts a resource.Resource into
// a Resource struct.
func Resource2API(res resource.Resource) Resource {
	return Resource{
		CharmResource: CharmResource2API(res.Resource),
		ID:            res.ID,
		PendingID:     res.PendingID,
		ServiceID:     res.ServiceID,
		Username:      res.Username,
		Timestamp:     res.Timestamp,
	}
}

// APIResult2ServiceResources converts a ResourcesResult into a resource.ServiceResources.
func APIResult2ServiceResources(apiResult ResourcesResult) (resource.ServiceResources, error) {
	var result resource.ServiceResources

	if apiResult.Error != nil {
		// TODO(ericsnow) Return the resources too?
		err := RestoreError(apiResult.Error)
		return resource.ServiceResources{}, errors.Trace(err)
	}

	for _, apiRes := range apiResult.Resources {
		res, err := API2Resource(apiRes)
		if err != nil {
			// This could happen if the server is misbehaving
			// or non-conforming.
			// TODO(ericsnow) Aggregate errors?
			return resource.ServiceResources{}, errors.Annotate(err, "got bad data from server")
		}
		result.Resources = append(result.Resources, res)
	}

	for _, unitRes := range apiResult.UnitResources {
		tag, err := names.ParseUnitTag(unitRes.Tag)
		if err != nil {
			return resource.ServiceResources{}, errors.Annotate(err, "got bad data from server")
		}
		unitResources := resource.UnitResources{Tag: tag}
		for _, apiRes := range unitRes.Resources {
			res, err := API2Resource(apiRes)
			if err != nil {
				return resource.ServiceResources{}, errors.Annotate(err, "got bad data from server")
			}
			unitResources.Resources = append(unitResources.Resources, res)
		}
		result.UnitResources = append(result.UnitResources, unitResources)
	}

	for _, chRes := range apiResult.CharmStoreResources {
		res, err := API2CharmResource(chRes)
		if err != nil {
			return resource.ServiceResources{}, errors.Annotate(err, "got bad data from server")
		}
		result.CharmStoreResources = append(result.CharmStoreResources, res)
	}

	return result, nil
}

func ServiceResources2APIResult(svcRes resource.ServiceResources, units []names.UnitTag) ResourcesResult {
	var result ResourcesResult
	for _, res := range svcRes.Resources {
		result.Resources = append(result.Resources, Resource2API(res))
	}
	unitResources := make(map[names.UnitTag]resource.UnitResources, len(svcRes.UnitResources))
	for _, unitRes := range svcRes.UnitResources {
		unitResources[unitRes.Tag] = unitRes
	}

	result.UnitResources = make([]UnitResources, len(units))
	for i, tag := range units {
		apiRes := UnitResources{
			Entity: params.Entity{Tag: tag.String()},
		}
		for _, res := range unitResources[tag].Resources {
			apiRes.Resources = append(apiRes.Resources, Resource2API(res))
		}
		result.UnitResources[i] = apiRes
	}

	result.CharmStoreResources = make([]CharmResource, len(svcRes.CharmStoreResources))
	for i, chRes := range svcRes.CharmStoreResources {
		result.CharmStoreResources[i] = CharmResource2API(chRes)
	}
	return result
}

// API2Resource converts an API Resource struct into
// a resource.Resource.
func API2Resource(apiRes Resource) (resource.Resource, error) {
	var res resource.Resource

	charmRes, err := API2CharmResource(apiRes.CharmResource)
	if err != nil {
		return res, errors.Trace(err)
	}

	res = resource.Resource{
		Resource:  charmRes,
		ID:        apiRes.ID,
		PendingID: apiRes.PendingID,
		ServiceID: apiRes.ServiceID,
		Username:  apiRes.Username,
		Timestamp: apiRes.Timestamp,
	}

	if err := res.Validate(); err != nil {
		return res, errors.Trace(err)
	}

	return res, nil
}

// CharmResource2API converts a charm resource into
// a CharmResource struct.
func CharmResource2API(res charmresource.Resource) CharmResource {
	return CharmResource{
		Name:        res.Name,
		Type:        res.Type.String(),
		Path:        res.Path,
		Description: res.Description,
		Origin:      res.Origin.String(),
		Revision:    res.Revision,
		Fingerprint: res.Fingerprint.Bytes(),
		Size:        res.Size,
	}
}

// API2CharmResource converts an API CharmResource struct into
// a charm resource.
func API2CharmResource(apiInfo CharmResource) (charmresource.Resource, error) {
	var res charmresource.Resource

	rtype, err := charmresource.ParseType(apiInfo.Type)
	if err != nil {
		return res, errors.Trace(err)
	}

	origin, err := charmresource.ParseOrigin(apiInfo.Origin)
	if err != nil {
		return res, errors.Trace(err)
	}

	fp, err := resource.DeserializeFingerprint(apiInfo.Fingerprint)
	if err != nil {
		return res, errors.Trace(err)
	}

	res = charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        apiInfo.Name,
			Type:        rtype,
			Path:        apiInfo.Path,
			Description: apiInfo.Description,
		},
		Origin:      origin,
		Revision:    apiInfo.Revision,
		Fingerprint: fp,
		Size:        apiInfo.Size,
	}

	if err := res.Validate(); err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}

var singletonErrorCodes = map[error]string{}

func singletonError(err error) (error, bool) {
	sameErr := func(err2 error) (error, bool) {
		return err, err.Error() == err2.Error()
	}
	switch params.ErrCode(err) {
	case params.CodeCannotEnterScopeYet:
		return sameErr(state.ErrCannotEnterScopeYet)
	case params.CodeCannotEnterScope:
		return sameErr(state.ErrCannotEnterScope)
	case params.CodeUnitHasSubordinates:
		return sameErr(state.ErrUnitHasSubordinates)
	case params.CodeDead:
		return sameErr(state.ErrDead)
	case params.CodeExcessiveContention:
		return sameErr(txn.ErrExcessiveContention)
	case params.CodeLeadershipClaimDenied:
		return sameErr(leadership.ErrClaimDenied)
	case params.CodeLeaseClaimDenied:
		return sameErr(lease.ErrClaimDenied)
	case params.CodeNotFound:
		if err, ok := sameErr(common.ErrBadId); ok {
			return err, ok
		}
		return sameErr(common.ErrUnknownWatcher)
	case params.CodeUnauthorized:
		if err, ok := sameErr(common.ErrBadCreds); ok {
			return err, ok
		}
		if err, ok := sameErr(common.ErrPerm); ok {
			return err, ok
		}
		return sameErr(common.ErrNotLoggedIn)
	case params.CodeStopped:
		return sameErr(common.ErrStoppedWatcher)
	case params.CodeTryAgain:
		return sameErr(common.ErrTryAgain)
	case params.CodeActionNotAvailable:
		return sameErr(common.ErrActionNotAvailable)
	default:
		return nil, false
	}
}

// RestoreError makes a best effort at converting the given error
// back into an error originally converted by ServerError().
func RestoreError(err error) error {
	err = errors.Cause(err)

	if apiErr, ok := err.(*params.Error); !ok {
		return err
	} else if apiErr == nil {
		return nil
	}
	if params.ErrCode(err) == "" {
		return err
	}
	msg := err.Error()

	if singleton, ok := singletonError(err); ok {
		return singleton
	}

	// TODO(ericsnow) Support the other error types handled by ServerError().
	switch {
	case params.IsCodeUnauthorized(err):
		return errors.NewUnauthorized(nil, msg)
	case params.IsCodeNotFound(err):
		// TODO(ericsnow) UnknownModelError should be handled here too.
		// ...by parsing msg?
		return errors.NewNotFound(nil, msg)
	case params.IsCodeAlreadyExists(err):
		return errors.NewAlreadyExists(nil, msg)
	case params.IsCodeNotAssigned(err):
		return errors.NewNotAssigned(nil, msg)
	case params.IsCodeHasAssignedUnits(err):
		// TODO(ericsnow) Handle state.HasAssignedUnitsError here.
		// ...by parsing msg?
		return err
	case params.IsCodeNoAddressSet(err):
		// TODO(ericsnow) Handle isNoAddressSetError here.
		// ...by parsing msg?
		return err
	case params.IsCodeNotProvisioned(err):
		return errors.NewNotProvisioned(nil, msg)
	case params.IsCodeUpgradeInProgress(err):
		// TODO(ericsnow) Handle state.UpgradeInProgressError here.
		// ...by parsing msg?
		return err
	case params.IsCodeMachineHasAttachedStorage(err):
		// TODO(ericsnow) Handle state.HasAttachmentsError here.
		// ...by parsing msg?
		return err
	case params.IsCodeNotSupported(err):
		return errors.NewNotSupported(nil, msg)
	case params.IsBadRequest(err):
		return errors.NewBadRequest(nil, msg)
	case params.IsMethodNotAllowed(err):
		return errors.NewMethodNotAllowed(nil, msg)
	case params.ErrCode(err) == params.CodeDischargeRequired:
		// TODO(ericsnow) Handle DischargeRequiredError here.
		return err
	default:
		return err
	}
}
