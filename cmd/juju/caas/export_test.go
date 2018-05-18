// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func NewAddCAASCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	fileCredentialStore jujuclient.CredentialStore,
	clientStore jujuclient.ClientStore,
	addCloudAPIFunc func() (AddCloudAPI, error),
	newClientConfigReaderFunc func(string) (clientconfig.ClientConfigFunc, error),
) cmd.Command {
	cmd := &AddCAASCommand{
		cloudMetadataStore:    cloudMetadataStore,
		fileCredentialStore:   fileCredentialStore,
		apiFunc:               addCloudAPIFunc,
		newClientConfigReader: newClientConfigReaderFunc,
	}
	cmd.SetClientStore(clientStore)
	return modelcmd.WrapController(cmd)
}

func NewRemoveCAASCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	fileCredentialStore jujuclient.CredentialStore,
	clientStore jujuclient.ClientStore,
	removeCloudAPIFunc func() (RemoveCloudAPI, error),
) cmd.Command {
	cmd := &RemoveCAASCommand{
		cloudMetadataStore:  cloudMetadataStore,
		fileCredentialStore: fileCredentialStore,
		apiFunc:             removeCloudAPIFunc,
	}
	cmd.SetClientStore(clientStore)
	return modelcmd.WrapController(cmd)
}
