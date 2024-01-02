// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	common "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/core/crossmodel"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/state"
)

// CrossModelRelationsState provides the subset of global state required by the
// remote relations facade.
type CrossModelRelationsState interface {
	common.Backend

	// Model returns the model entity.
	Model() (Model, error)

	// AddOfferConnection creates a new offer connection record, which records details about a
	// relation made from a remote model to an offer in the local model.
	AddOfferConnection(state.AddOfferConnectionParams) (common.OfferConnection, error)

	// IsMigrationActive returns true if the current model is
	// in the process of being migrated to another controller.
	IsMigrationActive() (bool, error)

	// GetSecretConsumerInfo returns the remote app tag and offer uuid
	// for the specified consumer app and relation tokens.
	GetSecretConsumerInfo(string, string) (names.Tag, string, error)

	// GetSecret gets the secret metadata for the given secret URI.
	GetSecret(*coresecrets.URI) (*coresecrets.SecretMetadata, error)

	// WatchConsumedSecretsChanges returns a watcher for secrets
	// consumed by the specified remote consumer.
	WatchConsumedSecretsChanges(string) (state.StringsWatcher, error)
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	common.Backend
	st *state.State
}

func (st stateShim) ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error) {
	oa := state.NewApplicationOffers(st.st)
	return oa.ApplicationOfferForUUID(offerUUID)
}

func (st stateShim) AddOfferConnection(arg state.AddOfferConnectionParams) (common.OfferConnection, error) {
	return st.st.AddOfferConnection(arg)
}

func (st stateShim) OfferConnectionForRelation(relationKey string) (common.OfferConnection, error) {
	return st.st.OfferConnectionForRelation(relationKey)
}

// IsMigrationActive returns true if the current model is
// in the process of being migrated to another controller.
func (st stateShim) IsMigrationActive() (bool, error) {
	migrating, err := st.st.IsMigrationActive()
	return migrating, errors.Trace(err)
}

func (s stateShim) GetSecretConsumerInfo(appToken, relToken string) (names.Tag, string, error) {
	appTag, err := s.Backend.GetRemoteEntity(appToken)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	// TODO(juju4) - remove
	// For compatibility with older clients which do not
	// provide a relation tag.
	if relToken == "" {
		return appTag, "", nil
	}

	relTag, err := s.Backend.GetRemoteEntity(relToken)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	conn, err := s.Backend.OfferConnectionForRelation(relTag.Id())
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	return appTag, conn.OfferUUID(), nil
}

func (s stateShim) GetSecret(uri *coresecrets.URI) (*coresecrets.SecretMetadata, error) {
	store := state.NewSecrets(s.st)
	return store.GetSecret(uri)
}

func (s stateShim) WatchConsumedSecretsChanges(consumerApp string) (state.StringsWatcher, error) {
	return s.st.WatchRemoteConsumedSecretsChanges(consumerApp)
}

type Model interface {
	Name() string
	Owner() names.UserTag
}

func (st stateShim) Model() (Model, error) {
	return st.st.Model()
}
