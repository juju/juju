// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

var logger = loggo.GetLogger("juju.apiserver.common.crossmodel")

// PublishRelationChange applies the relation change event to the specified backend.
func PublishRelationChange(backend Backend, relationTag names.Tag, change params.RemoteRelationChangeEvent) error {
	logger.Debugf("publish into model %v change for %v: %+v", backend.ModelUUID(), relationTag, change)

	dyingOrDead := change.Life != "" && change.Life != params.Alive
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

	// Update the relation status if necessary.
	relStatus := status.Status(change.Status)
	if relStatus != "" {
		currentStatus, err := rel.Status()
		if err != nil {
			return errors.Trace(err)
		}
		if currentStatus.Status != relStatus || currentStatus.Message != change.StatusMessage {
			if err := rel.SetStatus(status.StatusInfo{
				Status:  relStatus,
				Message: change.StatusMessage,
			}); err != nil {
				return errors.Trace(err)
			}
		}
	}

	// Look up the application on the remote side of this relation
	// ie from the model which published this change.
	applicationTag, err := backend.GetRemoteEntity(change.ApplicationToken)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("application tag for token %+v is %v", change.ApplicationToken, applicationTag)

	// If the remote model has destroyed the relation, do it here also.
	if dyingOrDead {
		logger.Debugf("remote side of %v died", relationTag)
		if err := rel.Destroy(); err != nil {
			return errors.Trace(err)
		}
		// See if we need to remove the remote application proxy - we do this
		// on the offering side as there is 1:1 between proxy and consuming app.
		if applicationTag != nil {
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
		}
	}

	// TODO(wallyworld) - deal with remote application being removed
	if applicationTag == nil {
		logger.Infof("no remote application found for %v", relationTag.Id())
		return nil
	}
	logger.Debugf("remote application for changed relation %v is %v", relationTag.Id(), applicationTag.Id())

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

// WatchRelationUnits returns a watcher for changes to the units on the specified relation.
func WatchRelationUnits(backend Backend, tag names.RelationTag) (state.RelationUnitsWatcher, error) {
	relation, err := backend.KeyRelation(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, ep := range relation.Endpoints() {
		_, err := backend.Application(ep.ApplicationName)
		if errors.IsNotFound(err) {
			// Not found, so it's the remote application. Try the next endpoint.
			continue
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		w, err := relation.WatchUnits(ep.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return w, nil
	}
	return nil, errors.NotFoundf("local application for %s", names.ReadableString(tag))
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
	rule, err := backend.FirewallRule(state.JujuApplicationOfferRule)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if errors.IsNotFound(err) {
		return nil
	}
	var whitelistCIDRs, blacklistCIDRs, requestedCIDRs []*net.IPNet
	if err := parseCIDRs(&whitelistCIDRs, rule.WhitelistCIDRs); err != nil {
		return errors.Trace(err)
	}
	if err := parseCIDRs(&blacklistCIDRs, rule.BlacklistCIDRs); err != nil {
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
	if len(blacklistCIDRs) > 0 {
		for _, n := range requestedCIDRs {
			if network.SubnetInAnyRange(whitelistCIDRs, n) {
				return &params.Error{
					Code:    params.CodeForbidden,
					Message: fmt.Sprintf("subnet %v in firewall blacklist", n),
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
	KeyRelation(string) (Relation, error)
}

// GetRelationStatusChange returns a status change struct for a specified relation key.
func GetRelationStatusChange(st relationGetter, key string) (*params.RelationStatusChange, error) {
	rel, err := st.KeyRelation(key)
	if errors.IsNotFound(err) {
		return &params.RelationStatusChange{
			Key:    key,
			Life:   params.Dead,
			Status: params.Broken,
		}, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	relStatus, err := rel.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &params.RelationStatusChange{
		Key:           key,
		Life:          params.Life(rel.Life().String()),
		Status:        params.RelationStatusValue(relStatus.Status),
		StatusMessage: relStatus.Message,
	}, nil
}
