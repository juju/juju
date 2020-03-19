// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"bytes"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	jujucmdcloud "github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func NewAddCAASCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	credentialStoreAPI CredentialStoreAPI,
	store jujuclient.ClientStore,
	addCloudAPIFunc func() (AddCloudAPI, error),
	brokerGetter BrokerGetter,
	k8sCluster k8sCluster,
	newClientConfigReaderFunc func(string) (clientconfig.ClientConfigFunc, error),
	getAllCloudDetails func() (map[string]*jujucmdcloud.CloudDetails, error),
) cmd.Command {
	command := &AddCAASCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		cloudMetadataStore:        cloudMetadataStore,
		credentialStoreAPI:        credentialStoreAPI,
		addCloudAPIFunc:           addCloudAPIFunc,
		brokerGetter:              brokerGetter,
		k8sCluster:                k8sCluster,
		newClientConfigReader:     newClientConfigReaderFunc,
		credentialUIDGetter:       func(credentialGetter, string, string) (string, error) { return "9baa5e46", nil },
		getAllCloudDetails: func(jujuclient.CredentialGetter) (map[string]*jujucmdcloud.CloudDetails, error) {
			return getAllCloudDetails()
		},
	}
	return command
}

func NewUpdateCAASCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	store jujuclient.ClientStore,
	updateCloudAPIFunc func() (UpdateCloudAPI, error),
	brokerGetter BrokerGetter,
) cmd.Command {
	command := &UpdateCAASCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		cloudMetadataStore:        cloudMetadataStore,
		brokerGetter:              brokerGetter,
		updateCloudAPIFunc:        updateCloudAPIFunc,
		builtInCloudsFunc: func(cloudName string) (cloud.Cloud, *cloud.Credential, string, error) {
			if cloudName != "microk8s" {
				return cloud.Cloud{}, nil, "", errors.NotFoundf("cloud %q", cloudName)
			}
			return cloud.Cloud{
					Name:      cloudName,
					Type:      "kubernetes",
					AuthTypes: cloud.AuthTypes{"certificate"},
				},
				&cloud.Credential{
					Label: "test",
				},
				"default",
				nil
		},
	}
	return command
}

func NewRemoveCAASCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	credentialStoreAPI credentialGetter,
	store jujuclient.ClientStore,
	removeCloudAPIFunc func() (RemoveCloudAPI, error),
) cmd.Command {
	command := &RemoveCAASCommand{
		credentialStoreAPI:        credentialStoreAPI,
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		cloudMetadataStore:        cloudMetadataStore,
		apiFunc:                   removeCloudAPIFunc,
	}
	return command
}

type fakeCluster struct {
	CommandRunner

	cloudType string
	config    string
}

type readerCloser struct {
	io.Reader
}

func (*readerCloser) Close() error {
	return nil
}

func (f *fakeCluster) getKubeConfig(p *clusterParams) (io.ReadCloser, string, error) {
	return &readerCloser{bytes.NewBuffer([]byte(f.config))}, "the-cluster", nil
}

func (*fakeCluster) interactiveParams(ctx *cmd.Context, p *clusterParams) (*clusterParams, error) {
	return p, nil
}

func (f *fakeCluster) cloud() string {
	return f.cloudType
}

func (f *fakeCluster) ensureExecutable() error {
	return nil
}

// TODO exported function with unexported type :(
func FakeCluster(config string) k8sCluster {
	return &fakeCluster{config: config, cloudType: "gce"}
}

var (
	CheckCloudRegion = checkCloudRegion
)
