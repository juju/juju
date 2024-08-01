// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	coreuser "github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
)

type dummyStateCloud struct {
	Credentials map[string]credential.Key
	Regions     []string
}

type dummyState struct {
	clouds             map[string]dummyStateCloud
	models             map[coremodel.UUID]coremodel.Model
	nonActivatedModels map[coremodel.UUID]coremodel.Model
	users              map[user.UUID]string
	secretBackends     []string
	lastLogin          map[user.UUID]map[coremodel.UUID]time.Time
}

type dummyDeleter struct {
	deleted map[string]struct{}
}

func (d *dummyDeleter) DeleteDB(namespace string) error {
	d.deleted[namespace] = struct{}{}
	return nil
}

func (d *dummyState) CloudType(
	_ context.Context,
	name string,
) (string, error) {
	_, exists := d.clouds[name]
	if !exists {
		return "", clouderrors.NotFound
	}

	return "aws", nil
}

func (d *dummyState) Create(
	_ context.Context,
	modelID coremodel.UUID,
	modelType coremodel.ModelType,
	args model.ModelCreationArgs,
) error {
	if _, exists := d.models[modelID]; exists {
		return fmt.Errorf("%w %q", modelerrors.AlreadyExists, modelID)
	}

	for _, v := range d.models {
		if v.Name == args.Name && v.Owner == args.Owner {
			return fmt.Errorf("%w for name %q and owner %q", modelerrors.AlreadyExists, v.Name, v.Owner)
		}
	}

	cloud, exists := d.clouds[args.Cloud]
	if !exists {
		return fmt.Errorf("%w cloud %q", errors.NotFound, args.Cloud)
	}

	userName, exists := d.users[user.UUID(args.Owner.String())]
	if !exists {
		return fmt.Errorf("%w for owner %q", usererrors.UserNotFound, args.Owner)
	}

	hasRegion := false
	for _, region := range cloud.Regions {
		if region == args.CloudRegion {
			hasRegion = true
		}
	}
	if !hasRegion {
		return fmt.Errorf("%w cloud %q region %q", errors.NotFound, args.Cloud, args.CloudRegion)
	}

	if !args.Credential.IsZero() {
		if _, exists := cloud.Credentials[args.Credential.String()]; !exists {
			return fmt.Errorf("%w credential %q", errors.NotFound, args.Credential.String())
		}
	}

	secretBackendFound := false
	for _, backend := range d.secretBackends {
		if backend == args.SecretBackend {
			secretBackendFound = true
		}
	}

	if !secretBackendFound {
		return secretbackenderrors.NotFound
	}

	d.nonActivatedModels[modelID] = coremodel.Model{
		AgentVersion: args.AgentVersion,
		Name:         args.Name,
		UUID:         modelID,
		ModelType:    modelType,
		Cloud:        args.Cloud,
		CloudRegion:  args.CloudRegion,
		Credential:   args.Credential,
		Owner:        args.Owner,
		OwnerName:    userName,
		Life:         life.Alive,
	}
	return nil
}

func (d *dummyState) Activate(
	_ context.Context,
	uuid coremodel.UUID,
) error {
	if model, exists := d.nonActivatedModels[uuid]; exists {
		d.models[uuid] = model
		delete(d.nonActivatedModels, uuid)
		return nil
	}

	if _, exists := d.models[uuid]; exists {
		return modelerrors.AlreadyActivated
	}
	return modelerrors.NotFound
}

func (d *dummyState) GetModel(
	_ context.Context,
	uuid coremodel.UUID,
) (coremodel.Model, error) {
	info, exists := d.models[uuid]
	if !exists {
		return coremodel.Model{}, fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	return info, nil
}

func (d *dummyState) GetModelByName(
	_ context.Context,
	userName string,
	modelName string,
) (coremodel.Model, error) {
	for _, model := range d.models {
		if model.OwnerName == userName && model.Name == modelName {
			return model, nil
		}
	}
	return coremodel.Model{}, modelerrors.NotFound
}

func (d *dummyState) GetModelType(
	_ context.Context,
	uuid coremodel.UUID,
) (coremodel.ModelType, error) {
	info, exists := d.models[uuid]
	if !exists {
		return "", fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	return info.ModelType, nil
}

func (d *dummyState) Delete(
	_ context.Context,
	uuid coremodel.UUID,
) error {
	if _, exists := d.models[uuid]; !exists {
		return fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	delete(d.models, uuid)
	return nil
}

func (d *dummyState) ListAllModels(
	_ context.Context,
) ([]coremodel.Model, error) {
	rval := make([]coremodel.Model, 0, len(d.models))
	for _, m := range d.models {
		rval = append(rval, m)
	}

	return rval, nil
}

func (d *dummyState) ListHostedModels(_ context.Context, includeLifes []life.Value, excludeIDs []coremodel.UUID) ([]coremodel.HostedModel, error) {
	var hostedModels []coremodel.HostedModel

	for _, m := range d.models {
		if !slices.Contains(includeLifes, m.Life) {
			continue
		}
		if slices.Contains(excludeIDs, m.UUID) {
			continue
		}

		hostedModels = append(hostedModels, coremodel.HostedModel{
			Model: m,
			Cloud: cloud.Cloud{
				Name:            m.Cloud,
				Type:            m.CloudType,
				HostCloudRegion: m.CloudRegion,
			},
			Credential: cloud.Credential{},
		})
	}

	return hostedModels, nil
}

func (d *dummyState) ListModelsWithLastLogin(_ context.Context, userID coreuser.UUID, includeLifes []life.Value) ([]coremodel.ModelWithLogin, error) {
	var modelsWithLogin []coremodel.ModelWithLogin

	for _, m := range d.models {
		if !slices.Contains(includeLifes, m.Life) {
			continue
		}

		var lastLogin *time.Time
		if userLogins, ok := d.lastLogin[userID]; ok {
			if modelLogin, ok := userLogins[m.UUID]; ok {
				lastLogin = &modelLogin
			}
		}

		modelsWithLogin = append(modelsWithLogin, coremodel.ModelWithLogin{
			Model:     m,
			UserID:    userID,
			LastLogin: lastLogin,
		})
	}

	return modelsWithLogin, nil
}

func (d *dummyState) ListModelsForUser(
	_ context.Context,
	userID coreuser.UUID,
) ([]coremodel.Model, error) {
	rval := []coremodel.Model{}
	for _, m := range d.models {
		if m.Owner == userID {
			rval = append(rval, m)
		}
	}

	return rval, nil
}

func (d *dummyState) ListModelIDs(
	_ context.Context,
) ([]coremodel.UUID, error) {
	rval := make([]coremodel.UUID, 0, len(d.models))
	for k := range d.models {
		rval = append(rval, k)
	}

	return rval, nil
}

func (d *dummyState) ModelCloudNameAndCredential(
	_ context.Context,
	modelName string,
	ownerName string,
) (string, credential.Key, error) {
	var ownerUUID user.UUID
	for k, v := range d.users {
		if v == ownerName {
			ownerUUID = k
		}
	}

	for _, model := range d.models {
		if model.Owner == ownerUUID && model.Name == modelName {
			return model.Cloud, model.Credential, nil
		}
	}
	return "", credential.Key{}, modelerrors.NotFound
}

func (d *dummyState) UpdateCredential(
	_ context.Context,
	uuid coremodel.UUID,
	credentialKey credential.Key,
) error {
	info, exists := d.models[uuid]
	if !exists {
		return fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}

	cloud, exists := d.clouds[credentialKey.Cloud]
	if !exists {
		return fmt.Errorf("%w cloud %q", errors.NotFound, credentialKey.Cloud)
	}

	if _, exists := cloud.Credentials[credentialKey.String()]; !exists {
		return fmt.Errorf("%w credential %q", errors.NotFound, credentialKey.String())
	}

	if info.Cloud != credentialKey.Cloud {
		return fmt.Errorf("%w credential cloud is different to that of the model", errors.NotValid)
	}

	return nil
}

func (d *dummyState) setLife(_ context.Context, modelID coremodel.UUID, newLife life.Value) error {
	m, ok := d.models[modelID]
	if !ok {
		return modelerrors.NotFound
	}

	m.Life = newLife
	d.models[modelID] = m
	return nil
}

func (d *dummyState) updateModelLogin(userID user.UUID, modelID coremodel.UUID, timestamp time.Time) {
	if d.lastLogin[userID] == nil {
		d.lastLogin[userID] = map[coremodel.UUID]time.Time{}
	}
	d.lastLogin[userID][modelID] = timestamp
}
