// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/cloud"
	corecloud "github.com/juju/juju/core/cloud"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/version"
	usererrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/errors"
	jujutesting "github.com/juju/juju/internal/testing"
)

type dummyStateCloud struct {
	Credentials map[string]credential.Key
	Regions     []string
}

type dummyState struct {
	clouds              map[string]dummyStateCloud
	models              map[coremodel.UUID]coremodel.Model
	nonActivatedModels  map[coremodel.UUID]coremodel.Model
	users               map[user.UUID]user.Name
	secretBackends      []string
	controllerModelUUID coremodel.UUID
}

func (d *dummyState) CheckModelExists(ctx context.Context, uuid coremodel.UUID) (bool, error) {
	_, exists := d.models[uuid]
	return exists, nil
}

type dummyDeleter struct {
	deleted map[string]struct{}
}

func (d *dummyDeleter) DeleteDB(namespace string) error {
	d.deleted[namespace] = struct{}{}
	return nil
}

func (d *dummyState) CloudSupportsAuthType(
	_ context.Context,
	name string,
	authType cloud.AuthType,
) (bool, error) {
	return true, nil
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

func (d *dummyState) ListModelUUIDsForUser(_ context.Context, _ user.UUID) ([]coremodel.UUID, error) {
	return nil, nil
}

func (d *dummyState) Create(
	_ context.Context,
	modelID coremodel.UUID,
	modelType coremodel.ModelType,
	args model.GlobalModelCreationArgs,
) error {
	if _, exists := d.models[modelID]; exists {
		return errors.Errorf("%w %q", modelerrors.AlreadyExists, modelID)
	}

	for _, v := range d.models {
		if v.Name == args.Name && v.Qualifier == args.Qualifier {
			return errors.Errorf("%w for name %s/%s", modelerrors.AlreadyExists, v.Qualifier, v.Name)
		}
	}

	cloud, exists := d.clouds[args.Cloud]
	if !exists {
		return errors.Errorf("%w cloud %q", coreerrors.NotFound, args.Cloud)
	}

	for _, u := range args.AdminUsers {
		_, exists = d.users[u]
		if !exists {
			return errors.Errorf("%w for creator %q", usererrors.UserNotFound, u)
		}
	}

	hasRegion := false
	for _, region := range cloud.Regions {
		if region == args.CloudRegion {
			hasRegion = true
		}
	}
	if !hasRegion {
		return errors.Errorf("%w cloud %q region %q", coreerrors.NotFound, args.Cloud, args.CloudRegion)
	}

	if !args.Credential.IsZero() {
		if _, exists := cloud.Credentials[args.Credential.String()]; !exists {
			return errors.Errorf("%w credential %q", coreerrors.NotFound, args.Credential.String())
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
		Name:        args.Name,
		Qualifier:   args.Qualifier,
		UUID:        modelID,
		ModelType:   modelType,
		Cloud:       args.Cloud,
		CloudRegion: args.CloudRegion,
		Credential:  args.Credential,
		Life:        life.Alive,
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
		return coremodel.Model{}, errors.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	return info, nil
}

func (d *dummyState) GetControllerModel(
	_ context.Context,
) (coremodel.Model, error) {
	info, exists := d.models[d.controllerModelUUID]
	if !exists {
		return coremodel.Model{}, modelerrors.NotFound
	}
	return info, nil
}

func (d *dummyState) GetModelByName(
	_ context.Context,
	qualifier string,
	modelName string,
) (coremodel.Model, error) {
	for _, model := range d.models {
		if model.Qualifier == qualifier && model.Name == modelName {
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
		return "", errors.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	return info.ModelType, nil
}

func (d *dummyState) Delete(
	_ context.Context,
	uuid coremodel.UUID,
) error {
	if _, exists := d.models[uuid]; !exists {
		return errors.Errorf("%w %q", modelerrors.NotFound, uuid)
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

func (d *dummyState) ListModelsForUser(
	_ context.Context,
	userID user.UUID,
) ([]coremodel.Model, error) {
	rval := []coremodel.Model{}
	for _, m := range d.models {
		userName, ok := d.users[userID]
		if !ok {
			continue
		}
		if m.Qualifier == userName.String() {
			rval = append(rval, m)
		}
	}

	return rval, nil
}

func (d *dummyState) ListModelUUIDs(
	_ context.Context,
) ([]coremodel.UUID, error) {
	rval := make([]coremodel.UUID, 0, len(d.models))
	for k := range d.models {
		rval = append(rval, k)
	}

	return rval, nil
}

func (d *dummyState) GetControllerModelUUID(
	_ context.Context,
) (coremodel.UUID, error) {
	return coremodel.UUID(""), nil
}

func (d *dummyState) GetModelCloudInfo(
	_ context.Context,
	_ coremodel.UUID,
) (string, string, error) {
	return "", "", errors.Errorf("not implemented")
}

func (d *dummyState) GetModelCloudAndCredential(
	_ context.Context,
	_ coremodel.UUID,
) (corecloud.UUID, credential.UUID, error) {
	return "", "", errors.Errorf("not implemented")
}

func (d *dummyState) UpdateCredential(
	_ context.Context,
	uuid coremodel.UUID,
	credentialKey credential.Key,
) error {
	info, exists := d.models[uuid]
	if !exists {
		return errors.Errorf("%w %q", modelerrors.NotFound, uuid)
	}

	cloud, exists := d.clouds[credentialKey.Cloud]
	if !exists {
		return errors.Errorf("%w cloud %q", coreerrors.NotFound, credentialKey.Cloud)
	}

	if _, exists := cloud.Credentials[credentialKey.String()]; !exists {
		return errors.Errorf("%w credential %q", coreerrors.NotFound, credentialKey.String())
	}

	if info.Cloud != credentialKey.Cloud {
		return errors.Errorf("%w credential cloud is different to that of the model", coreerrors.NotValid)
	}

	return nil
}

func (d *dummyState) GetModelUsers(_ context.Context, _ coremodel.UUID) ([]coremodel.ModelUserInfo, error) {
	var rval []coremodel.ModelUserInfo
	for _, name := range d.users {
		rval = append(rval, coremodel.ModelUserInfo{
			Name:           name,
			DisplayName:    name.Name(),
			Access:         permission.AdminAccess,
			LastModelLogin: time.Time{},
		})
	}
	return rval, nil
}

func (d *dummyState) ListModelSummariesForUser(_ context.Context, userName user.Name) ([]coremodel.UserModelSummary, error) {
	var rval []coremodel.UserModelSummary
	for _, m := range d.models {
		if m.Qualifier == userName.String() {
			rval = append(rval, coremodel.UserModelSummary{
				UserAccess: permission.AdminAccess,
				ModelSummary: coremodel.ModelSummary{
					Name:           m.Name,
					UUID:           m.UUID,
					ModelType:      m.ModelType,
					CloudName:      m.Cloud,
					CloudType:      m.CloudType,
					CloudRegion:    m.CloudRegion,
					ControllerUUID: jujutesting.ControllerTag.Id(),
					IsController:   m.UUID == d.controllerModelUUID,
					Qualifier:      m.Qualifier,
					Life:           m.Life,
					AgentVersion:   version.Current,
				},
			})
		}
	}
	return rval, nil
}

func (d *dummyState) ListAllModelSummaries(_ context.Context) ([]coremodel.ModelSummary, error) {
	var rval []coremodel.ModelSummary
	for _, m := range d.models {
		rval = append(rval, coremodel.ModelSummary{
			Name:           m.Name,
			UUID:           m.UUID,
			ModelType:      m.ModelType,
			CloudName:      m.Cloud,
			CloudType:      m.CloudType,
			CloudRegion:    m.CloudRegion,
			ControllerUUID: jujutesting.ControllerTag.Id(),
			IsController:   m.UUID == d.controllerModelUUID,
			Qualifier:      m.Qualifier,
			Life:           m.Life,
			AgentVersion:   version.Current,
		})
	}
	return rval, nil
}

func (d *dummyState) InitialWatchActivatedModelsStatement() (string, string) {
	return "model", "SELECT activated from model"
}

func (d *dummyState) InitialWatchModelTableName() string {
	return "model"
}

func (d *dummyState) GetActivatedModelUUIDs(ctx context.Context, uuids []coremodel.UUID) ([]coremodel.UUID, error) {
	return nil, nil
}

func (d *dummyState) GetModelLife(ctx context.Context, uuid coremodel.UUID) (domainlife.Life, error) {
	switch d.models[uuid].Life {
	case life.Alive:
		return domainlife.Alive, nil
	case life.Dead:
		return domainlife.Dead, nil
	case life.Dying:
		return domainlife.Dying, nil
	default:
		return domainlife.Life(0), errors.Errorf("invalid life value %v", d.models[uuid].Life)
	}
}
