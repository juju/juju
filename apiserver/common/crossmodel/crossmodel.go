// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/firewall"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/network"
)

var logger = loggo.GetLogger("juju.apiserver.common.crossmodel")

// PublishRelationChange applies the relation change event to the specified backend.
func PublishRelationChange(backend Backend, relationTag names.Tag, change params.RemoteRelationChangeEvent) error {
	logger.Debugf("publish into model %v change for %v: %+v", backend.ModelUUID(), relationTag, change)

	dyingOrDead := change.Life != "" && change.Life != life.Alive
	// Ensure the relation exists.
	rel, err := backend.KeyRelation(relationTag.Id())
	if errors.IsNotFound(err) {
		if dyingOrDead {
			return nil
		}
	}
	if err != nil {
		return errors.Trace(err)
	}

	// Update the relation suspended status.
	currentStatus := rel.Suspended()
	if !dyingOrDead && change.Suspended != nil && currentStatus != *change.Suspended {
		var (
			newStatus status.Status
			message   string
		)
		if *change.Suspended {
			newStatus = status.Suspending
			message = change.SuspendedReason
			if message == "" {
				message = "suspending after update from remote model"
			}
		}
		if err := rel.SetSuspended(*change.Suspended, message); err != nil {
			return errors.Trace(err)
		}
		if !*change.Suspended {
			newStatus = status.Joining
			message = ""
		}
		if err := rel.SetStatus(status.StatusInfo{
			Status:  newStatus,
			Message: message,
		}); err != nil && !errors.IsNotValid(err) {
			return errors.Trace(err)
		}
	}

	// Look up the application on the remote side of this relation
	// ie from the model which published this change.
	applicationTag, err := backend.GetRemoteEntity(change.ApplicationToken)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("application tag for token %+v is %v in model %v", change.ApplicationToken, applicationTag, backend.ModelUUID())

	// If the remote model has destroyed the relation, do it here also.
	forceCleanUp := change.ForceCleanup != nil && *change.ForceCleanup
	if dyingOrDead {
		logger.Debugf("remote consuming side of %v died", relationTag)
		if forceCleanUp {
			logger.Debugf("forcing cleanup of units for %v", applicationTag.Id())
			remoteUnits, err := rel.AllRemoteUnits(applicationTag.Id())
			if err != nil {
				return errors.Trace(err)
			}
			logger.Debugf("got %v relation units to clean", len(remoteUnits))
			for _, ru := range remoteUnits {
				if err := ru.LeaveScope(); err != nil {
					return errors.Trace(err)
				}
			}
		}

		if err := rel.Destroy(); err != nil {
			return errors.Trace(err)
		}
		// See if we need to remove the remote application proxy - we do this
		// on the offering side as there is 1:1 between proxy and consuming app.
		remoteApp, err := backend.RemoteApplication(applicationTag.Id())
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		if err == nil && remoteApp.IsConsumerProxy() {
			logger.Debugf("destroy consuming app proxy for %v", applicationTag.Id())
			if err := remoteApp.Destroy(); err != nil {
				return errors.Trace(err)
			}
		}

		// If we are forcing cleanup, we can exit early here.
		if forceCleanUp {
			return nil
		}
	}

	// TODO(wallyworld) - deal with remote application being removed
	if applicationTag == nil {
		logger.Infof("no remote application found for %v", relationTag.Id())
		return nil
	}
	logger.Debugf("remote application for changed relation %v is %v in model %v",
		relationTag.Id(), applicationTag.Id(), backend.ModelUUID())

	// Allow sending an empty non-nil map to clear all the settings.
	if change.ApplicationSettings != nil {
		logger.Debugf("remote application %v in %v settings changed to %v",
			applicationTag.Id(), relationTag.Id(), change.ApplicationSettings)
		err := rel.ReplaceApplicationSettings(applicationTag.Id(), change.ApplicationSettings)
		if err != nil {
			return errors.Trace(err)
		}
	}

	for _, id := range change.DepartedUnits {
		unitTag := names.NewUnitTag(fmt.Sprintf("%s/%v", applicationTag.Id(), id))
		logger.Debugf("unit %v has departed relation %v", unitTag.Id(), relationTag.Id())
		ru, err := rel.RemoteUnit(unitTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("%s leaving scope", unitTag.Id())
		if err := ru.LeaveScope(); err != nil {
			return errors.Trace(err)
		}
	}

	for _, change := range change.ChangedUnits {
		unitTag := names.NewUnitTag(fmt.Sprintf("%s/%v", applicationTag.Id(), change.UnitId))
		logger.Debugf("changed unit tag for unit id %v is %v", change.UnitId, unitTag)
		ru, err := rel.RemoteUnit(unitTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
		inScope, err := ru.InScope()
		if err != nil {
			return errors.Trace(err)
		}
		settings := make(map[string]interface{})
		for k, v := range change.Settings {
			settings[k] = v
		}
		if !inScope {
			logger.Debugf("%s entering scope (%v)", unitTag.Id(), settings)
			err = ru.EnterScope(settings)
		} else {
			logger.Debugf("%s updated settings (%v)", unitTag.Id(), settings)
			err = ru.ReplaceSettings(settings)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// GetOfferingRelationTokens returns the tokens for the relation and the offer
// of the passed in relation tag.
func GetOfferingRelationTokens(backend Backend, tag names.RelationTag) (string, string, error) {
	offerName, err := backend.OfferNameForRelation(tag.Id())
	if err != nil {
		return "", "", errors.Trace(err)
	}
	relationToken, err := backend.GetToken(tag)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	appToken, err := backend.GetToken(names.NewApplicationTag(offerName))
	if err != nil {
		// TODO(babbageclunk): do we need to try getting the appToken
		// for the application name instead (opposite of the fallback
		// in
		// apiserver/facades/controller/crossmodelrelations/crossmodelrelations.go:296)?
		// I don't think so because this method is only called from
		// API methods that were added after the transition to offer
		// name as the app token.
		return "", "", errors.Trace(err)
	}
	return relationToken, appToken, nil
}

// GetConsumingRelationTokens returns the tokens for the relation and the local
// application of the passed in relation tag.
func GetConsumingRelationTokens(backend Backend, tag names.RelationTag) (string, string, error) {
	relation, err := backend.KeyRelation(tag.Id())
	if err != nil {
		return "", "", errors.Trace(err)
	}
	localAppName, err := getLocalApplicationName(backend, relation)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	relationToken, err := backend.GetToken(tag)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	appToken, err := backend.GetToken(names.NewApplicationTag(localAppName))
	if err != nil {
		return "", "", errors.Trace(err)
	}
	return relationToken, appToken, nil
}

func getLocalApplicationName(backend Backend, relation Relation) (string, error) {
	for _, ep := range relation.Endpoints() {
		_, err := backend.Application(ep.ApplicationName)
		if errors.IsNotFound(err) {
			// Not found, so it's the remote application. Try the next endpoint.
			continue
		} else if err != nil {
			return "", errors.Trace(err)
		}
		return ep.ApplicationName, nil
	}
	return "", errors.NotFoundf("local application for %s", names.ReadableString(relation.Tag()))
}

// WatchRelationUnits returns a watcher for changes to the units on the specified relation.
func WatchRelationUnits(backend Backend, tag names.RelationTag) (common.RelationUnitsWatcher, error) {
	relation, err := backend.KeyRelation(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	localAppName, err := getLocalApplicationName(backend, relation)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w, err := relation.WatchUnits(localAppName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	wrapped, err := common.RelationUnitsWatcherFromState(w)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return wrapped, nil
}

// ExpandChange converts a params.RelationUnitsChange into a
// params.RemoteRelationChangeEvent by filling out the extra
// information from the passed backend. This takes relation and
// application token so that it can still return sensible results if
// the relation has been removed (just departing units).
func ExpandChange(
	backend Backend,
	relationToken string,
	appToken string,
	change params.RelationUnitsChange,
) (params.RemoteRelationChangeEvent, error) {
	var empty params.RemoteRelationChangeEvent

	var departed []int
	for _, unitName := range change.Departed {
		num, err := names.UnitNumber(unitName)
		if err != nil {
			return empty, errors.Trace(err)
		}
		departed = append(departed, num)
	}

	relationTag, err := backend.GetRemoteEntity(relationToken)
	if errors.IsNotFound(err) {
		// This can happen when the last unit leaves scope on a dying
		// relation and the relation is removed. In that case there
		// aren't any application- or unit-level settings to send; we
		// just send the departed units so they can leave scope on
		// the other side of a cross-model relation.
		return params.RemoteRelationChangeEvent{
			RelationToken:    relationToken,
			ApplicationToken: appToken,
			DepartedUnits:    departed,
		}, nil

	} else if err != nil {
		return empty, errors.Trace(err)
	}

	relation, err := backend.KeyRelation(relationTag.Id())
	if err != nil {
		return empty, errors.Trace(err)
	}
	localAppName, err := getLocalApplicationName(backend, relation)
	if err != nil {
		return empty, errors.Trace(err)
	}

	var appSettings map[string]interface{}
	if len(change.AppChanged) > 0 {
		appSettings, err = relation.ApplicationSettings(localAppName)
		if err != nil {
			return empty, errors.Trace(err)
		}
	}

	var unitChanges []params.RemoteRelationUnitChange
	for unitName := range change.Changed {
		relUnit, err := relation.Unit(unitName)
		if err != nil {
			return empty, errors.Annotatef(err, "getting unit %q in %q", unitName, relationTag.Id())
		}
		unitSettings, err := relUnit.Settings()
		if err != nil {
			return empty, errors.Annotatef(err, "getting settings for %q in %q", unitName, relationTag.Id())
		}
		num, err := names.UnitNumber(unitName)
		if err != nil {
			return empty, errors.Trace(err)
		}
		unitChanges = append(unitChanges, params.RemoteRelationUnitChange{
			UnitId:   num,
			Settings: unitSettings,
		})
	}

	result := params.RemoteRelationChangeEvent{
		RelationToken:       relationToken,
		ApplicationToken:    appToken,
		ApplicationSettings: appSettings,
		ChangedUnits:        unitChanges,
		DepartedUnits:       departed,
	}

	return result, nil
}

// WrappedUnitsWatcher is a relation units watcher that remembers
// details about the relation it came from so changes can be expanded
// for sending outside this model.
type WrappedUnitsWatcher struct {
	common.RelationUnitsWatcher
	RelationToken    string
	ApplicationToken string
}

// RelationUnitSettings returns the unit settings for the specified relation unit.
func RelationUnitSettings(backend Backend, ru params.RelationUnit) (params.Settings, error) {
	relationTag, err := names.ParseRelationTag(ru.Relation)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rel, err := backend.KeyRelation(relationTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitTag, err := names.ParseUnitTag(ru.Unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	unit, err := rel.Unit(unitTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	settings, err := unit.Settings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	paramsSettings := make(params.Settings)
	for k, v := range settings {
		vString, ok := v.(string)
		if !ok {
			return nil, errors.Errorf(
				"invalid relation setting %q: expected string, got %T", k, v,
			)
		}
		paramsSettings[k] = vString
	}
	return paramsSettings, nil
}

// PublishIngressNetworkChange saves the specified ingress networks for a relation.
func PublishIngressNetworkChange(backend Backend, relationTag names.Tag, change params.IngressNetworksChangeEvent) error {
	logger.Debugf("publish into model %v network change for %v: %+v", backend.ModelUUID(), relationTag, change)

	// Ensure the relation exists.
	rel, err := backend.KeyRelation(relationTag.Id())
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("relation %v requires ingress networks %v", rel, change.Networks)
	if err := validateIngressNetworks(backend, change.Networks); err != nil {
		return errors.Trace(err)
	}

	_, err = backend.SaveIngressNetworks(rel.Tag().Id(), change.Networks)
	return err
}

func validateIngressNetworks(backend Backend, networks []string) error {
	if len(networks) == 0 {
		return nil
	}

	// Check that the required ingress is allowed.
	rule, err := backend.FirewallRule(firewall.JujuApplicationOfferRule)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if errors.IsNotFound(err) {
		return nil
	}
	var whitelistCIDRs, requestedCIDRs []*net.IPNet
	if err := parseCIDRs(&whitelistCIDRs, rule.WhitelistCIDRs()); err != nil {
		return errors.Trace(err)
	}
	if err := parseCIDRs(&requestedCIDRs, networks); err != nil {
		return errors.Trace(err)
	}
	if len(whitelistCIDRs) > 0 {
		for _, n := range requestedCIDRs {
			if !network.SubnetInAnyRange(whitelistCIDRs, n) {
				return &params.Error{
					Code:    params.CodeForbidden,
					Message: fmt.Sprintf("subnet %v not in firewall whitelist", n),
				}
			}
		}
	}
	return nil
}

func parseCIDRs(cidrs *[]*net.IPNet, values []string) error {
	for _, cidrStr := range values {
		if _, ipNet, err := net.ParseCIDR(cidrStr); err != nil {
			return err
		} else {
			*cidrs = append(*cidrs, ipNet)
		}
	}
	return nil
}

type relationGetter interface {
	// KeyRelation returns the relation identified by the input key.
	KeyRelation(string) (Relation, error)
	// IsMigrationActive returns true if the current model is
	// in the process of being migrated to another controller.
	IsMigrationActive() (bool, error)
}

// GetRelationLifeSuspendedStatusChange returns a life/suspended status change
// struct for a specified relation key.
func GetRelationLifeSuspendedStatusChange(
	st relationGetter, key string,
) (*params.RelationLifeSuspendedStatusChange, error) {
	rel, err := st.KeyRelation(key)
	if errors.IsNotFound(err) {
		// If the relation is not found we represent it as dead,
		// but *only* if we are not currently migrating.
		// If we are migrating, we do not want to inform remote watchers that
		// the relation is dead before they have had a chance to be redirected
		// to the new controller.
		if migrating, mErr := st.IsMigrationActive(); mErr == nil && !migrating {
			return &params.RelationLifeSuspendedStatusChange{
				Key:  key,
				Life: life.Dead,
			}, nil
		} else if mErr != nil {
			err = mErr
		}
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &params.RelationLifeSuspendedStatusChange{
		Key:             key,
		Life:            life.Value(rel.Life().String()),
		Suspended:       rel.Suspended(),
		SuspendedReason: rel.SuspendedReason(),
	}, nil
}

type offerGetter interface {
	ApplicationOfferForUUID(string) (*crossmodel.ApplicationOffer, error)
	Application(string) (Application, error)
}

// GetOfferStatusChange returns a status change
// struct for a specified offer name.
func GetOfferStatusChange(st offerGetter, offerUUID string) (*params.OfferStatusChange, error) {
	offer, err := st.ApplicationOfferForUUID(offerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(wallyworld) - for now, offer status is just the application status
	app, err := st.Application(offer.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sts, err := app.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &params.OfferStatusChange{
		OfferName: offer.OfferName,
		Status: params.EntityStatus{
			Status: sts.Status,
			Info:   sts.Message,
			Data:   sts.Data,
			Since:  sts.Since,
		},
	}, nil
}
