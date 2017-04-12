// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"github.com/juju/errors"
	"github.com/juju/txn"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodelcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
)

// OffersAPI implements the cross model interface and is the concrete
// implementation of the api end point.
type OffersAPI struct {
	crossmodelcommon.BaseAPI
}

// createAPI returns a new application offers OffersAPI facade.
func createOffersAPI(
	getApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers,
	backend crossmodelcommon.Backend,
	statePool crossmodelcommon.StatePool,
	authorizer facade.Authorizer,
) (*OffersAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	api := &OffersAPI{
		BaseAPI: crossmodelcommon.BaseAPI{
			Authorizer:           authorizer,
			GetApplicationOffers: getApplicationOffers,
			Backend:              backend,
			StatePool:            statePool,
		}}
	return api, nil
}

// NewOffersAPI returns a new application offers OffersAPI facade.
func NewOffersAPI(ctx facade.Context) (*OffersAPI, error) {
	return createOffersAPI(
		crossmodelcommon.GetApplicationOffers, crossmodelcommon.GetStateAccess(ctx.State()),
		crossmodelcommon.GetStatePool(ctx.StatePool()), ctx.Auth())
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *OffersAPI) Offer(all params.AddApplicationOffers) (params.ErrorResults, error) {
	if err := api.CheckAdmin(api.Backend); err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}

	result := make([]params.ErrorResult, len(all.Offers))

	for i, one := range all.Offers {
		applicationOfferParams, err := api.makeAddOfferArgsFromParams(one)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		_, err = api.GetApplicationOffers(api.Backend).AddOffer(applicationOfferParams)
		result[i].Error = common.ServerError(err)
	}
	return params.ErrorResults{Results: result}, nil
}

func (api *OffersAPI) makeAddOfferArgsFromParams(addOfferParams params.AddApplicationOffer) (jujucrossmodel.AddApplicationOfferArgs, error) {
	result := jujucrossmodel.AddApplicationOfferArgs{
		OfferName:              addOfferParams.OfferName,
		ApplicationName:        addOfferParams.ApplicationName,
		ApplicationDescription: addOfferParams.ApplicationDescription,
		Endpoints:              addOfferParams.Endpoints,
		Owner:                  api.Authorizer.GetAuthTag().Id(),
		HasRead:                []string{common.EveryoneTagName},
	}
	if result.OfferName == "" {
		result.OfferName = result.ApplicationName
	}
	application, err := api.Backend.Application(addOfferParams.ApplicationName)
	if err != nil {
		return result, errors.Annotatef(err, "getting offered application %v", addOfferParams.ApplicationName)
	}

	if result.ApplicationDescription == "" {
		ch, _, err := application.Charm()
		if err != nil {
			return result,
				errors.Annotatef(err, "getting charm for application %v", addOfferParams.ApplicationName)
		}
		result.ApplicationDescription = ch.Meta().Description
	}
	return result, nil
}

// ListApplicationOffers gets deployed details about application offers that match given filter.
// The results contain details about the deployed applications such as connection count.
func (api *OffersAPI) ListApplicationOffers(filters params.OfferFilters) (params.ListApplicationOffersResults, error) {
	var result params.ListApplicationOffersResults
	offers, err := api.GetApplicationOffersDetails(filters, true)
	if err != nil {
		return result, err
	}
	result.Results = offers
	return result, nil
}

// ModifyOfferAccess changes the application offer access granted to users.
func (api *OffersAPI) ModifyOfferAccess(args params.ModifyOfferAccessRequest) (result params.ErrorResults, _ error) {
	result = params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	canModifyController, err := api.Authorizer.HasPermission(permission.SuperuserAccess, api.Backend.ControllerTag())
	if err != nil {
		return result, errors.Trace(err)
	}
	canModifyModel, err := api.Authorizer.HasPermission(permission.AdminAccess, api.Backend.ModelTag())
	if err != nil {
		return result, errors.Trace(err)
	}
	isAdmin := canModifyController || canModifyModel

	for i, arg := range args.Changes {
		result.Results[i].Error = common.ServerError(api.modifyOneOfferAccess(isAdmin, arg))
	}
	return result, nil
}

func (api *OffersAPI) modifyOneOfferAccess(isAdmin bool, arg params.ModifyOfferAccess) error {
	offerAccess := permission.Access(arg.Access)
	if err := permission.ValidateOfferAccess(offerAccess); err != nil {
		return errors.Annotate(err, "could not modify offer access")
	}

	offerTag, err := names.ParseApplicationOfferTag(arg.OfferTag)
	if err != nil {
		return errors.Annotate(err, "could not modify offer access")
	}
	canModifyOffer, err := api.Authorizer.HasPermission(permission.AdminAccess, offerTag)
	if err != nil {
		return errors.Trace(err)
	}
	canModify := isAdmin || canModifyOffer
	if !canModify {
		return common.ErrPerm
	}

	targetUserTag, err := names.ParseUserTag(arg.UserTag)
	if err != nil {
		return errors.Annotate(err, "could not modify offer access")
	}
	return api.changeOfferAccess(offerTag, targetUserTag, arg.Action, offerAccess)
}

// changeOfferAccess performs the requested access grant or revoke action for the
// specified user on the specified application offer.
func (api *OffersAPI) changeOfferAccess(
	offerTag names.ApplicationOfferTag,
	targetUserTag names.UserTag,
	action params.OfferAction,
	access permission.Access,
) error {
	_, err := api.Backend.ApplicationOffer(offerTag.Name)
	if err != nil {
		return errors.Trace(err)
	}
	switch action {
	case params.GrantOfferAccess:
		return api.grantOfferAccess(offerTag, targetUserTag, access)
	case params.RevokeOfferAccess:
		return api.revokeOfferAccess(offerTag, targetUserTag, access)
	default:
		return errors.Errorf("unknown action %q", action)
	}
}

func (api *OffersAPI) grantOfferAccess(offerTag names.ApplicationOfferTag, targetUserTag names.UserTag, access permission.Access) error {
	err := api.Backend.CreateOfferAccess(offerTag, targetUserTag, access)
	if errors.IsAlreadyExists(err) {
		offerAccess, err := api.Backend.GetOfferAccess(offerTag, targetUserTag)
		if errors.IsNotFound(err) {
			// Conflicts with prior check, must be inconsistent state.
			err = txn.ErrExcessiveContention
		}
		if err != nil {
			return errors.Annotate(err, "could not look up offer access for user")
		}

		// Only set access if greater access is being granted.
		if offerAccess.EqualOrGreaterOfferAccessThan(access) {
			return errors.Errorf("user already has %q access or greater", access)
		}
		if err = api.Backend.UpdateOfferAccess(offerTag, targetUserTag, access); err != nil {
			return errors.Annotate(err, "could not set offer access for user")
		}
		return nil
	}
	return errors.Annotate(err, "could not grant offer access")
}

func (api *OffersAPI) revokeOfferAccess(offerTag names.ApplicationOfferTag, targetUserTag names.UserTag, access permission.Access) error {
	switch access {
	case permission.ReadAccess:
		// Revoking read access removes all access.
		err := api.Backend.RemoveOfferAccess(offerTag, targetUserTag)
		return errors.Annotate(err, "could not revoke offer access")
	case permission.ConsumeAccess:
		// Revoking consume access sets read-only.
		err := api.Backend.UpdateOfferAccess(offerTag, targetUserTag, permission.ReadAccess)
		return errors.Annotate(err, "could not set offer access to read-only")
	case permission.AdminAccess:
		// Revoking admin access sets read-consume.
		err := api.Backend.UpdateOfferAccess(offerTag, targetUserTag, permission.ConsumeAccess)
		return errors.Annotate(err, "could not set offer access to read-consume")

	default:
		return errors.Errorf("don't know how to revoke %q access", access)
	}
}
