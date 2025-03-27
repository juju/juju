// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize

import (
	"os"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/shell"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/mongo"
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

func (c *configFromEnv) UpgradedToVersion() semversion.Number {
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

func (c *configFromEnv) JujuDBSnapChannel() string {
	panic("not implemented")
}

func (c *configFromEnv) AgentLogfileMaxSizeMB() int {
	panic("not implemented")
}

func (c *configFromEnv) AgentLogfileMaxBackups() int {
	panic("not implemented")
}

func (c *configFromEnv) QueryTracingEnabled() bool {
	panic("not implemented")
}

func (c *configFromEnv) QueryTracingThreshold() time.Duration {
	panic("not implemented")
}

func (c *configFromEnv) OpenTelemetryEnabled() bool {
	panic("not implemented")
}

func (c *configFromEnv) OpenTelemetryEndpoint() string {
	panic("not implemented")
}

func (c *configFromEnv) OpenTelemetryInsecure() bool {
	panic("not implemented")
}

func (c *configFromEnv) OpenTelemetryStackTraces() bool {
	panic("not implemented")
}

func (c *configFromEnv) OpenTelemetrySampleRatio() float64 {
	panic("not implemented")
}

func (c *configFromEnv) OpenTelemetryTailSamplingThreshold() time.Duration {
	panic("not implemented")
}

func (c *configFromEnv) ObjectStoreType() objectstore.BackendType {
	panic("not implemented")
}

func (c *configFromEnv) DqlitePort() (int, bool) {
	panic("not implemented")
}

type configFunc func() agent.Config

func defaultConfig() agent.Config {
	return &configFromEnv{}
}

type identityFunc func() (identity, error)

func identityFromK8sMetadata() (identity, error) {
	podName := os.Getenv(k8sconstants.EnvJujuK8sPodName)
	podUUID := os.Getenv(k8sconstants.EnvJujuK8sPodUUID)
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
