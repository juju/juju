// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v2/shell"
	"github.com/juju/version"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/mongo"
)

type configFromEnv struct {
}

func (c *configFromEnv) DataDir() string {
	panic("not implemented")
}

func (c *configFromEnv) TransientDataDir() string {
	panic("not implemented")
}

func (c *configFromEnv) LogDir() string {
	panic("not implemented")
}

func (c *configFromEnv) SystemIdentityPath() string {
	panic("not implemented")
}

func (c *configFromEnv) Jobs() []model.MachineJob {
	panic("not implemented")
}

func (c *configFromEnv) Tag() names.Tag {
	return names.NewApplicationTag(os.Getenv("JUJU_K8S_APPLICATION"))
}

func (c *configFromEnv) Dir() string {
	panic("not implemented")
}

func (c *configFromEnv) Nonce() string {
	panic("not implemented")
}

func (c *configFromEnv) CACert() string {
	return os.Getenv("JUJU_K8S_CONTROLLER_CA_CERT")
}

func (c *configFromEnv) APIAddresses() ([]string, error) {
	return strings.Split(os.Getenv("JUJU_K8S_CONTROLLER_ADDRESSES"), ","), nil
}

func (c *configFromEnv) WriteCommands(renderer shell.Renderer) ([]string, error) {
	panic("not implemented")
}

func (c *configFromEnv) StateServingInfo() (controller.StateServingInfo, bool) {
	panic("not implemented")
}

func (c *configFromEnv) APIInfo() (*api.Info, bool) {
	addresses, _ := c.APIAddresses()
	return &api.Info{
		Addrs:    addresses,
		CACert:   c.CACert(),
		ModelTag: c.Model(),
		Tag:      c.Tag(),
		Password: c.OldPassword(),
	}, true
}

func (c *configFromEnv) MongoInfo() (*mongo.MongoInfo, bool) {
	panic("not implemented")
}

func (c *configFromEnv) OldPassword() string {
	return os.Getenv("JUJU_K8S_APPLICATION_PASSWORD")
}

func (c *configFromEnv) UpgradedToVersion() version.Number {
	panic("not implemented")
}

func (c *configFromEnv) LoggingConfig() string {
	panic("not implemented")
}

func (c *configFromEnv) Value(key string) string {
	panic("not implemented")
}

func (c *configFromEnv) Model() names.ModelTag {
	return names.NewModelTag(os.Getenv("JUJU_K8S_MODEL"))
}

func (c *configFromEnv) Controller() names.ControllerTag {
	panic("not implemented")
}

func (c *configFromEnv) MetricsSpoolDir() string {
	panic("not implemented")
}

func (c *configFromEnv) MongoMemoryProfile() mongo.MemoryProfile {
	panic("not implemented")
}

func (c *configFromEnv) JujuDBSnapChannel() string {
	panic("not implemented")
}

func (c *configFromEnv) NonSyncedWritesToRaftLog() bool {
	panic("not implemented")
}

type configFunc func() agent.Config

func defaultConfig() agent.Config {
	return &configFromEnv{}
}

type identityFunc func() (identity, error)

func defaultIdentity() (identity, error) {
	if ecsMetaEndpoint := os.Getenv("ECS_CONTAINER_METADATA_URI_V4"); ecsMetaEndpoint != "" {
		return identityFromECSMetadataService(ecsMetaEndpoint)
	}
	return identityFromK8sMetadata()
}

func identityFromECSMetadataService(ecsMetaEndpoint string) (identity, error) {
	metaURL, err := url.Parse(ecsMetaEndpoint)
	if err != nil {
		return identity{}, errors.Annotate(err, "unable to parse ECS metadata endpoint")
	}
	metaURL.Path = path.Join(metaURL.Path, "task")

	res, err := http.Get(metaURL.String())
	if err != nil {
		return identity{}, errors.Annotatef(err, "unable to query ECS metadata service")
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		_, _ = io.Copy(ioutil.Discard, res.Body)
		return identity{}, errors.Annotatef(err, "unexpected status code %d from ECS metadata service", res.StatusCode)
	}

	// For more information on the response payload, see: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-metadata-endpoint-v4.html
	taskMeta := struct {
		TaskARN string `json:"TaskARN"`
	}{}

	if err = json.NewDecoder(res.Body).Decode(&taskMeta); err != nil {
		return identity{}, errors.Annotate(err, "unable to decode JSON response from ECS metadata service")
	}

	idSeparatorIndex := strings.LastIndexByte(taskMeta.TaskARN, '/')
	if idSeparatorIndex == -1 {
		return identity{}, errors.Annotate(err, "unable to extract task ID from task ARN")
	}

	return identity{
		PodName: taskMeta.TaskARN,
		PodUUID: taskMeta.TaskARN[idSeparatorIndex+1:],
	}, nil
}

func identityFromK8sMetadata() (identity, error) {
	podName := os.Getenv("JUJU_K8S_POD_NAME")
	podUUID := os.Getenv("JUJU_K8S_POD_UUID")
	if podName == "" || podUUID == "" {
		return identity{}, errors.New("unable to extract pod name and UUID from environment")
	}

	return identity{
		PodName: podName,
		PodUUID: podUUID,
	}, nil
}

type identity struct {
	PodName string
	PodUUID string
}
