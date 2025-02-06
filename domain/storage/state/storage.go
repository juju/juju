// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"errors"

	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/storage"
)

func (s State) GetModelDetails() (domainstorage.ModelDetails, error) {
	//TODO implement me
	return domainstorage.ModelDetails{}, errors.New("not implemented")
}

func (s State) ImportFilesystem(ctx context.Context, name storage.Name, filesystem domainstorage.FilesystemInfo) (storage.ID, error) {
	//TODO implement me
	return "", errors.New("not implemented")
}
