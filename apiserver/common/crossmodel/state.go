// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/state"
)

// GetBackend wraps a State to provide a Backend interface implementation.
func GetBackend(st *state.State) stateShim {
	model, err := st.Model()
	if err != nil {
		logger.Errorf(context.TODO(), "called GetBackend on a State with no Model.")
		return stateShim{}
	}
	return stateShim{State: st, Model: model}
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}

func (st stateShim) Relation(id int) (Relation, error) {
	return relationShim{}, nil
}

func (st stateShim) KeyRelation(key string) (Relation, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) OfferConnectionForRelation(relationKey string) (OfferConnection, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

// ControllerTag returns the tag of the controller in which we are operating.
// This is a temporary transitional step. Eventually code using
// crossmodel.Backend will only need to be passed a state.Model.
func (st stateShim) ControllerTag() names.ControllerTag {
	return st.Model.ControllerTag()
}

type applicationShim struct {
	*state.Application
}

func (a applicationShim) EndpointBindings() (Bindings, error) {
	return a.Application.EndpointBindings()
}

func (st stateShim) Application(name string) (Application, error) {
	a, err := st.State.Application(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationShim{a}, nil
}

func (st stateShim) RemoteApplication(name string) (RemoteApplication, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) AddRelation(eps ...relation.Endpoint) (Relation, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) EndpointsRelation(eps ...relation.Endpoint) (Relation, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) AddRemoteApplication(args AddRemoteApplicationParams) (RemoteApplication, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) GetRemoteApplicationTag(s string) (names.Tag, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) OfferUUIDForRelation(key string) (string, error) {
	return "", errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) GetRemoteEntity(token string) (names.Tag, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) GetToken(entity names.Tag) (string, error) {
	return "", errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) GetMacaroon(entity names.Tag) (*macaroon.Macaroon, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) ExportLocalEntity(entity names.Tag) (string, error) {
	return "", errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) ImportRemoteEntity(entity names.Tag, token string) error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) RemoveRemoteEntity(entity names.Tag) error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (st stateShim) ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (s stateShim) SaveIngressNetworks(relationKey string, cidrs []string) (RelationNetworks, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (s stateShim) IngressNetworks(relationKey string) (RelationNetworks, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (s stateShim) AllRemoteApplications() ([]RemoteApplication, error) {
	// todo(gfouillet): cross model relations are disabled until backend
	//   functionality is moved to domain, so we just return an empty list until it is done
	return nil, nil
}

type relationShim struct {
}

func (r relationShim) Status() (status.StatusInfo, error) {
	return status.StatusInfo{}, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationShim) SetStatus(info status.StatusInfo) error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationShim) Destroy(store objectstore.ObjectStore) error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationShim) DestroyWithForce(force bool, maxWait time.Duration) ([]error, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationShim) Id() int {
	// todo(gfouillet): to be implemented when CMR will be in domain
	return 0
}

func (r relationShim) Life() state.Life {
	// todo(gfouillet): to be implemented when CMR will be in domain
	return state.Dead
}

func (r relationShim) Tag() names.Tag {
	// todo(gfouillet): to be implemented when CMR will be in domain
	return names.NewRelationTag("implement:me")
}

func (r relationShim) UnitCount() int {
	// todo(gfouillet): to be implemented when CMR will be in domain
	return 0
}

func (r relationShim) Endpoints() []relation.Endpoint {
	// todo(gfouillet): to be implemented when CMR will be in domain
	return nil
}

func (r relationShim) Endpoint(appName string) (relation.Endpoint, error) {
	return relation.Endpoint{}, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationShim) WatchUnits(applicationName string) (relation.RelationUnitsWatcher, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationShim) WatchLifeSuspendedStatus() state.StringsWatcher {
	// todo(gfouillet): to be implemented when CMR will be in domain
	return nil
}

func (r relationShim) Suspended() bool {
	// todo(gfouillet): to be implemented when CMR will be in domain
	return true
}

func (r relationShim) SuspendedReason() string {
	// todo(gfouillet): to be implemented when CMR will be in domain
	return "not implemented"
}

func (r relationShim) SetSuspended(b bool, s string) error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationShim) ApplicationSettings(appName string) (map[string]interface{}, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationShim) RelatedEndpoints(name string) ([]relation.Endpoint, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationShim) RemoteApplication() (RemoteApplication, bool, error) {
	// todo(gfouillet): cross model relations are disabled until backend
	//   functionality is moved to domain, so we just return that the relation
	//  is not CMR
	return nil, false, nil
}

func (r relationShim) RemoteUnit(unitId string) (RelationUnit, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationShim) AllRemoteUnits(appName string) ([]RelationUnit, error) {
	// todo(gfouillet): cross model relations are disabled until backend
	//   functionality is moved to domain, so we just return an empty list
	//   to avoid breaking too much things.
	return nil, nil
}

func (r relationShim) Unit(unitId string) (RelationUnit, error) {
	return relationUnitShim{}, nil
}

func (r relationShim) ReplaceApplicationSettings(appName string, values map[string]interface{}) error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

type relationUnitShim struct{}

func (r relationUnitShim) EnterScope(settings map[string]interface{}) error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationUnitShim) InScope() (bool, error) {
	return false, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationUnitShim) LeaveScope() error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationUnitShim) Settings() (map[string]interface{}, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (r relationUnitShim) ReplaceSettings(s map[string]interface{}) error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}
