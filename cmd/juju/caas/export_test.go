// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func NewAddCAASCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	fileCredentialStore jujuclient.CredentialStore,
	clientStore jujuclient.ClientStore,
	apiRoot api.Connection,
	newCloudAPIFunc func(base.APICallCloser) CloudAPI,
	newClientConfigReaderFunc func(string) (clientconfig.ClientConfigFunc, error),
) cmd.Command {
	cmd := &AddCAASCommand{
		cloudMetadataStore:    cloudMetadataStore,
		fileCredentialStore:   fileCredentialStore,
		apiRoot:               apiRoot,
		newCloudAPI:           newCloudAPIFunc,
		newClientConfigReader: newClientConfigReaderFunc,
	}
	cmd.SetClientStore(clientStore)
	return modelcmd.WrapController(cmd)
}
