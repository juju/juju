// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"bytes"
	"io"

	"github.com/juju/cmd"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
	jujucmdcloud "github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

func NewAddCAASCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	store jujuclient.ClientStore,
	addCloudAPIFunc func() (AddCloudAPI, error),
	brokerGetter BrokerGetter,
	k8sCluster k8sCluster,
	newClientConfigReaderFunc func(string) (clientconfig.ClientConfigFunc, error),
	getAllCloudDetails func() (map[string]*jujucmdcloud.CloudDetails, error),
) cmd.Command {
	cmd := &AddCAASCommand{
		cloudMetadataStore:    cloudMetadataStore,
		store:                 store,
		addCloudAPIFunc:       addCloudAPIFunc,
		brokerGetter:          brokerGetter,
		k8sCluster:            k8sCluster,
		newClientConfigReader: newClientConfigReaderFunc,
		getAllCloudDetails:    getAllCloudDetails,
	}
	return cmd
}

func NewRemoveCAASCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	store jujuclient.ClientStore,
	removeCloudAPIFunc func() (RemoveCloudAPI, error),
) cmd.Command {
	cmd := &RemoveCAASCommand{
		cloudMetadataStore: cloudMetadataStore,
		store:              store,
		apiFunc:            removeCloudAPIFunc,
	}
	return cmd
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

func FakeCluster(config string) k8sCluster {
	return &fakeCluster{config: config, cloudType: "gce"}
}
