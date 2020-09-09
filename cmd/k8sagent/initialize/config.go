// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"os"
	"strings"

	"github.com/juju/names/v4"
	"github.com/juju/utils/shell"
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

func (c *configFromEnv) MongoVersion() mongo.Version {
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

type identityFunc func() identity

func defaultIdentity() identity {
	return identity{
		PodName: os.Getenv("JUJU_K8S_POD_NAME"),
		PodUUID: os.Getenv("JUJU_K8S_POD_UUID"),
	}
}

type identity struct {
	PodName string
	PodUUID string
}
