// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
	jujucmdcloud "github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func NewAddCAASCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	fileCredentialStore jujuclient.CredentialStore,
	clientStore jujuclient.ClientStore,
	addCloudAPIFunc func() (AddCloudAPI, error),
	brokerGetter BrokerGetter,
	newClientConfigReaderFunc func(string) (clientconfig.ClientConfigFunc, error),
	getAllCloudDetails func() (map[string]*jujucmdcloud.CloudDetails, error),
) cmd.Command {
	cmd := &AddCAASCommand{
		cloudMetadataStore:    cloudMetadataStore,
		fileCredentialStore:   fileCredentialStore,
		addCloudAPIFunc:       addCloudAPIFunc,
		brokerGetter:          brokerGetter,
		newClientConfigReader: newClientConfigReaderFunc,
		getAllCloudDetails:    getAllCloudDetails,
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

type K8sBrokerRegionLister = k8sBrokerRegionLister
