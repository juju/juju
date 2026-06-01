// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"reflect"
	"slices"
	"sort"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agenttest"
	agentcontroller "github.com/juju/juju/cmd/jujud/agent/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agentconfigupdater"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/bootstrap"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/gate"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

func TestManifoldsSuite(t *stdtesting.T) {
	tc.Run(t, &ManifoldsSuite{})
}

func (s *ManifoldsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *ManifoldsSuite) TestStartFuncs(c *tc.C) {
	manifolds := agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}

	manifolds = agentcontroller.CAASManifolds(agentcontroller.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})
	for name, manifold := range manifolds {
		c.Logf("checking CAAS %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNames(c *tc.C) {
	manifolds := agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	c.Assert(keys, tc.SameContents, []string{
		"agent",
		"agent-config-updater",
		"api-address-setter",
		"api-caller",
		"api-config-watcher",
		"api-remote-caller",
		"api-remote-relation-caller",
		"api-server",
		"audit-config-updater",
		"bootstrap",
		"certificate-updater",
		"certificate-watcher",
		"change-stream",
		"change-stream-pruner",
		"clock",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"control-socket",
		"controller-agent-config",
		"controller-presence",
		"db-accessor",
		"domain-services",
		"external-controller-updater",
		"file-notify-watcher",
		"flight-recorder",
		"http-client",
		"http-server",
		"http-server-args",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-primary-controller-flag",
		"jwt-parser",
		"lease-expiry",
		"lease-manager",
		"log-sink",
		"logging-controller-config-updater",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"model-worker-manager",
		"object-store",
		"object-store-drainer",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"secret-backend-rotate",
		"ssh-identity-writer",
		"ssh-server",
		"storage-registry",
		"termination-signal-handler",
		"trace",
		"undertaker",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-controller-steps-runner",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-database-runner",
		"upgrade-services",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrader",
		"watcher-registry",
	})
}

func (*ManifoldsSuite) TestMigrationInfrastructureStaysActive(c *tc.C) {
	manifolds := agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})
	manifold, ok := manifolds["migration-fortress"]
	c.Assert(ok, tc.IsTrue)

	checkNotContains(c, manifold.Inputs, "controller-upgrade-flag")
	checkNotContains(c, manifold.Inputs, "upgrade-check-flag")
	checkNotContains(c, manifold.Inputs, "upgrade-steps-flag")
}

func (s *ManifoldsSuite) TestMigrationGuardsUsed(c *tc.C) {
	// All manifolds that do NOT require both migration-fortress and
	// migration-inactive-flag as direct Inputs are listed here.
	exempt := set.NewStrings(
		"agent",
		"api-address-setter",
		"api-caller",
		"api-config-watcher",
		"api-remote-caller",
		"api-remote-relation-caller",
		"api-server",
		"audit-config-updater",
		"bootstrap",
		"certificate-updater",
		"certificate-watcher",
		"change-stream",
		"change-stream-pruner",
		"clock",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"control-socket",
		"controller-agent-config",
		"controller-presence",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"flight-recorder",
		"http-client",
		"http-server",
		"http-server-args",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-primary-controller-flag",
		"jwt-parser",
		"lease-expiry",
		"lease-manager",
		"log-sink",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"model-worker-manager",
		"object-store",
		"object-store-drainer",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"ssh-server",
		"storage-registry",
		"termination-signal-handler",
		"trace",
		"undertaker",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-controller-steps-runner",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-database-runner",
		"upgrade-services",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrader",
		"watcher-registry",
	)
	manifolds := agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})
	for name, manifold := range manifolds {
		c.Logf("%s", name)
		if !exempt.Contains(name) {
			checkContains(c, manifold.Inputs, "migration-fortress")
			checkContains(c, manifold.Inputs, "migration-inactive-flag")
		}
	}
}

func (*ManifoldsSuite) TestObjectStoreDoesNotUseDomainServices(c *tc.C) {
	// The object-store is a dependency of domain-services; no circular
	// dependency is permitted.
	manifolds := agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})

	manifold := manifolds["object-store"]
	checkNotContains(c, manifold.Inputs, "domain-services")

	dependencies := agenttest.ManifoldDependencies(manifolds, manifold)
	c.Check(dependencies.Contains("domain-services"), tc.IsFalse)
}

func (*ManifoldsSuite) TestProviderTrackerDoesNotUseDomainServices(c *tc.C) {
	// The provider-tracker is a dependency of domain-services; no circular
	// dependency is permitted.
	manifolds := agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})

	manifold := manifolds["provider-tracker"]
	checkNotContains(c, manifold.Inputs, "domain-services")

	dependencies := agenttest.ManifoldDependencies(manifolds, manifold)
	c.Check(dependencies.Contains("domain-services"), tc.IsFalse)
}

func (*ManifoldsSuite) TestAPICallerNonRecoverableErrorHandling(c *tc.C) {
	ag := &mockAgent{
		conf: mockConfig{
			dataPath: c.MkDir(),
		},
	}
	manifolds := agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
		Agent:           ag,
		PreUpgradeSteps: preUpgradeSteps,
	})

	c.Assert(manifolds["api-caller"], tc.Not(tc.IsNil))
	apiCallerManifold := manifolds["api-caller"]

	err := apiCallerManifold.Filter(apicaller.ErrConnectImpossible)
	c.Assert(err, tc.Equals, jworker.ErrTerminateAgent)
}

func checkContains(c *tc.C, names []string, seek string) {
	if slices.Contains(names, seek) {
		return
	}
	c.Errorf("%q not found in %v", seek, names)
}

func checkNotContains(c *tc.C, names []string, seek string) {
	if slices.Contains(names, seek) {
		c.Errorf("%q found in %v", seek, names)
		return
	}
}

func (*ManifoldsSuite) TestControllerUpgradeGate(c *tc.C) {
	controllerUpgradeLock := gate.NewLock()
	manifolds := agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
		Agent:                 &mockAgent{},
		PreUpgradeSteps:       preUpgradeSteps,
		ControllerUpgradeLock: controllerUpgradeLock,
	})
	assertGate(c, manifolds["controller-upgrade-gate"], controllerUpgradeLock)
}

func (*ManifoldsSuite) TestUpgradeGates(c *tc.C) {
	upgradeStepsLock := gate.NewLock()
	upgradeCheckLock := gate.NewLock()
	manifolds := agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
		Agent:            &mockAgent{},
		PreUpgradeSteps:  preUpgradeSteps,
		UpgradeStepsLock: upgradeStepsLock,
		UpgradeCheckLock: upgradeCheckLock,
	})
	checkContains(c, manifolds["upgrade-steps-gate"].Inputs, "controller-upgrade-flag")
	checkContains(c, manifolds["upgrade-check-gate"].Inputs, "controller-upgrade-flag")
}

func (*ManifoldsSuite) TestChangeStreamDirectInputs(c *tc.C) {
	// change-stream no longer depends on the agent manifold directly.
	// It receives the controller ID as a static config value, so only
	// db-accessor and file-notify-watcher appear in its direct Inputs.
	for _, manifolds := range []dependency.Manifolds{
		agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		agentcontroller.CAASManifolds(agentcontroller.ManifoldsConfig{
			Agent:           &mockAgent{conf: mockConfig{tag: names.NewControllerAgentTag("0")}},
			PreUpgradeSteps: preUpgradeSteps,
		}),
	} {
		manifold, ok := manifolds["change-stream"]
		c.Assert(ok, tc.IsTrue)
		c.Check(manifold.Inputs, tc.SameContents, []string{
			"db-accessor",
			"file-notify-watcher",
		})
		checkNotContains(c, manifold.Inputs, "agent")
	}
}

func (*ManifoldsSuite) TestControllerOnlyWorkerDirectInputs(c *tc.C) {
	for _, manifolds := range []dependency.Manifolds{
		agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		agentcontroller.CAASManifolds(agentcontroller.ManifoldsConfig{
			Agent:           &mockAgent{conf: mockConfig{tag: names.NewControllerAgentTag("0")}},
			PreUpgradeSteps: preUpgradeSteps,
		}),
	} {
		loggingManifold, ok := manifolds["logging-controller-config-updater"]
		c.Assert(ok, tc.IsTrue)
		c.Check(loggingManifold.Inputs, tc.SameContents, []string{
			"domain-services",
			"migration-fortress",
			"migration-inactive-flag",
		})
		checkNotContains(c, loggingManifold.Inputs, "agent")
		checkNotContains(c, loggingManifold.Inputs, "api-caller")

		secretBackendManifold, ok := manifolds["secret-backend-rotate"]
		c.Assert(ok, tc.IsTrue)
		checkContains(c, secretBackendManifold.Inputs, "domain-services")
		checkContains(c, secretBackendManifold.Inputs, "is-primary-controller-flag")
		checkNotContains(c, secretBackendManifold.Inputs, "api-caller")

		identityManifold, ok := manifolds["ssh-identity-writer"]
		c.Assert(ok, tc.IsTrue)
		c.Check(identityManifold.Inputs, tc.SameContents, []string{
			"migration-fortress",
			"migration-inactive-flag",
		})
		checkNotContains(c, identityManifold.Inputs, "agent")
		checkNotContains(c, identityManifold.Inputs, "api-caller")
	}
}

func (*ManifoldsSuite) TestOutOfScopeWorkersUseControllerUpgradeGate(c *tc.C) {
	for _, manifolds := range []dependency.Manifolds{
		agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		agentcontroller.CAASManifolds(agentcontroller.ManifoldsConfig{
			Agent:           &mockAgent{conf: mockConfig{tag: names.NewControllerAgentTag("0")}},
			PreUpgradeSteps: preUpgradeSteps,
		}),
	} {
		for _, name := range []string{
			"upgrade-services",
			"upgrade-steps-gate",
			"upgrade-steps-flag",
			"upgrade-check-gate",
			"upgrade-check-flag",
			"upgrader",
			"upgrade-controller-steps-runner",
			"api-remote-relation-caller",
			"migration-minion",
		} {
			checkContains(c, manifolds[name].Inputs, "controller-upgrade-flag")
		}

		for _, name := range []string{
			"api-remote-caller",
			"upgrade-database-gate",
			"upgrade-database-flag",
			"upgrade-database-runner",
			"migration-fortress",
			"migration-inactive-flag",
		} {
			checkNotContains(c, manifolds[name].Inputs, "controller-upgrade-flag")
		}
	}
}

func (*ManifoldsSuite) TestAgentConfigUpdaterUsesMigrationHousingInBothVariants(c *tc.C) {
	for _, manifolds := range []dependency.Manifolds{
		agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		agentcontroller.CAASManifolds(agentcontroller.ManifoldsConfig{
			Agent:           &mockAgent{conf: mockConfig{tag: names.NewControllerAgentTag("0")}},
			PreUpgradeSteps: preUpgradeSteps,
		}),
	} {
		manifold := manifolds["agent-config-updater"]
		checkContains(c, manifold.Inputs, "migration-fortress")
		checkContains(c, manifold.Inputs, "migration-inactive-flag")
	}
}

func (*ManifoldsSuite) TestBootstrapManifoldConfigUsesProviderSpecificHelpers(c *tc.C) {
	manifoldsCfg := agentcontroller.ManifoldsConfig{
		Agent: &mockAgent{},
	}
	iaasCfg := agentcontroller.NewIAASBootstrapManifoldConfig(manifoldsCfg)
	caasCfg := agentcontroller.NewCAASBootstrapManifoldConfig(manifoldsCfg)

	c.Check(reflect.ValueOf(iaasCfg.PopulateControllerCharm).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.PopulateIAASControllerCharm).Pointer())
	c.Check(reflect.ValueOf(caasCfg.PopulateControllerCharm).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.PopulateCAASControllerCharm).Pointer())
	c.Check(reflect.ValueOf(iaasCfg.AgentBinaryUploader).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.IAASAgentBinaryUploader).Pointer())
	c.Check(reflect.ValueOf(caasCfg.AgentBinaryUploader).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.CAASAgentBinaryUploader).Pointer())
	c.Check(reflect.ValueOf(iaasCfg.ControllerCharmDeployer).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.IAASControllerCharmUploader).Pointer())
	c.Check(reflect.ValueOf(caasCfg.ControllerCharmDeployer).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.CAASControllerCharmUploader).Pointer())
	c.Check(reflect.ValueOf(iaasCfg.ControllerUnitPassword).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.IAASControllerUnitPassword).Pointer())
	c.Check(reflect.ValueOf(caasCfg.ControllerUnitPassword).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.CAASControllerUnitPassword).Pointer())
	c.Check(reflect.ValueOf(iaasCfg.BootstrapAddressFinderGetter).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.IAASAddressFinder).Pointer())
	c.Check(reflect.ValueOf(caasCfg.BootstrapAddressFinderGetter).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.CAASAddressFinder).Pointer())
	c.Check(reflect.ValueOf(iaasCfg.AgentFinalizer).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.IAASAgentFinalizer).Pointer())
	c.Check(reflect.ValueOf(caasCfg.AgentFinalizer).Pointer(), tc.Equals, reflect.ValueOf(bootstrap.CAASAgentFinalizer).Pointer())
}

func (*ManifoldsSuite) TestAgentConfigUpdaterManifoldConfigUsesProviderSpecificControllerChecks(c *tc.C) {
	iaasCfg := agentcontroller.NewIAASAgentConfigUpdaterManifoldConfig()
	caasCfg := agentcontroller.NewCAASAgentConfigUpdaterManifoldConfig()

	c.Check(reflect.ValueOf(iaasCfg.IsControllerAgentFn).Pointer(), tc.Equals, reflect.ValueOf(agentconfigupdater.IAASIsControllerAgent).Pointer())
	c.Check(reflect.ValueOf(caasCfg.IsControllerAgentFn).Pointer(), tc.Equals, reflect.ValueOf(agentconfigupdater.CAASIsControllerAgent).Pointer())
}

func (*ManifoldsSuite) TestDBAccessorManifoldConfigUsesProviderSpecificNodeManagers(c *tc.C) {
	manifoldsCfg := agentcontroller.ManifoldsConfig{
		Agent: &mockAgent{},
	}
	iaasCfg := agentcontroller.NewIAASDBAccessorManifoldConfig(manifoldsCfg)
	caasCfg := agentcontroller.NewCAASDBAccessorManifoldConfig(manifoldsCfg)

	c.Check(reflect.ValueOf(iaasCfg.NewNodeManager).Pointer(), tc.Equals, reflect.ValueOf(dbaccessor.IAASNodeManager).Pointer())
	c.Check(reflect.ValueOf(caasCfg.NewNodeManager).Pointer(), tc.Equals, reflect.ValueOf(dbaccessor.CAASNodeManager).Pointer())
}

func assertGate(c *tc.C, manifold dependency.Manifold, unlocker gate.Unlocker) {
	w, err := manifold.Start(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	var waiter gate.Waiter
	err = manifold.Output(w, &waiter)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-waiter.Unlocked():
		c.Fatalf("expected gate to be locked")
	default:
	}

	unlocker.Unlock()

	select {
	case <-waiter.Unlocked():
	default:
		c.Fatalf("expected gate to be unlocked")
	}
}

func (s *ManifoldsSuite) TestManifoldsDependencies(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		agentcontroller.IAASManifolds(agentcontroller.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		expectedControllerManifoldsWithDependencies,
	)
}

var expectedControllerManifoldsWithDependencies = map[string][]string{

	"agent": {},

	"agent-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"migration-fortress",
		"migration-inactive-flag",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"api-address-setter": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-primary-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"api-caller": {"agent", "api-config-watcher"},

	"api-config-watcher": {"agent"},

	"api-remote-caller": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"object-store-services",
		"query-logger",
	},

	"api-remote-relation-caller": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"api-server": {
		"agent",
		"api-remote-caller",
		"audit-config-updater",
		"change-stream",
		"clock",
		"controller-agent-config",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"flight-recorder",
		"http-client",
		"http-server-args",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"jwt-parser",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
		"watcher-registry",
	},

	"audit-config-updater": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"bootstrap": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-bootstrap-gate",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"certificate-updater": {
		"agent",
		"api-remote-caller",
		"certificate-watcher",
		"change-stream",
		"clock",
		"controller-agent-config",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"certificate-watcher": {"agent"},

	"change-stream": {
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"query-logger",
	},

	"change-stream-pruner": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-primary-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"clock": {},

	"controller-upgrade-flag": {"controller-upgrade-gate"},

	"controller-upgrade-gate": {},

	"control-socket": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"controller-agent-config": {},

	"controller-presence": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"db-accessor": {
		"controller-agent-config",
		"query-logger",
	},

	"domain-services": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"external-controller-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-primary-controller-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"query-logger",
		"trace",
	},

	"file-notify-watcher": {},

	"flight-recorder": {},

	"http-client": {},

	"http-server": {
		"agent",
		"api-remote-caller",
		"api-server",
		"audit-config-updater",
		"certificate-watcher",
		"change-stream",
		"clock",
		"controller-agent-config",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"flight-recorder",
		"http-client",
		"http-server-args",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"jwt-parser",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
		"watcher-registry",
	},

	"http-server-args": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"is-bootstrap-flag": {"is-bootstrap-gate"},

	"is-bootstrap-gate": {},

	"is-primary-controller-flag": {
		"agent",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"lease-manager",
		"query-logger",
		"trace",
	},

	"jwt-parser": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"lease-expiry": {
		"agent",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-primary-controller-flag",
		"lease-manager",
		"query-logger",
		"trace",
	},

	"lease-manager": {
		"agent",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"query-logger",
		"trace",
	},

	"log-sink": {},

	"logging-controller-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"migration-fortress",
		"migration-inactive-flag",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"migration-fortress": {},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
	},

	"migration-minion": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"migration-fortress",
	},

	"model-worker-manager": {
		"agent",
		"api-remote-caller",
		"api-remote-relation-caller",
		"certificate-watcher",
		"change-stream",
		"clock",
		"controller-agent-config",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"object-store-fortress": {},

	"object-store-facade": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"object-store",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"query-logger",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"object-store-drainer": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"object-store",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"query-logger",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"object-store": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"object-store-s3-caller",
		"object-store-services",
		"query-logger",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"object-store-services": {
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"query-logger",
	},

	"object-store-s3-caller": {
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"object-store-services",
		"query-logger",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"provider-services": {
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"query-logger",
	},

	"provider-tracker": {
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"log-sink",
		"provider-services",
		"query-logger",
	},

	"query-logger": {},

	"secret-backend-rotate": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-primary-controller-flag",
		"lease-manager",
		"log-sink",
		"migration-fortress",
		"migration-inactive-flag",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"ssh-identity-writer": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
	},

	"ssh-server": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"storage-registry": {
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"log-sink",
		"provider-services",
		"provider-tracker",
		"query-logger",
	},

	"termination-signal-handler": {},

	"trace": {"agent"},

	"undertaker": {
		"agent",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"upgrade-check-flag": {"controller-upgrade-flag", "controller-upgrade-gate", "upgrade-check-gate"},

	"upgrade-check-gate": {"controller-upgrade-flag", "controller-upgrade-gate"},

	"upgrade-database-flag": {"upgrade-database-gate"},

	"upgrade-database-gate": {},

	"upgrade-database-runner": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"db-accessor",
		"file-notify-watcher",
		"query-logger",
		"upgrade-database-gate",
		"upgrade-services",
	},

	"upgrade-services": {
		"change-stream",
		"controller-agent-config",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"db-accessor",
		"file-notify-watcher",
		"query-logger",
	},

	"upgrade-steps-flag": {"controller-upgrade-flag", "controller-upgrade-gate", "upgrade-steps-gate"},

	"upgrade-steps-gate": {"controller-upgrade-flag", "controller-upgrade-gate"},

	"upgrade-controller-steps-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"api-remote-caller",
		"change-stream",
		"clock",
		"controller-agent-config",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
	},

	"upgrader": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"controller-upgrade-flag",
		"controller-upgrade-gate",
		"upgrade-check-gate",
		"upgrade-steps-gate",
	},

	"watcher-registry": {},
}

type mockAgent struct {
	agent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

func (ma *mockAgent) ChangeConfig(f agent.ConfigMutator) error {
	return f(&ma.conf)
}

type mockConfig struct {
	agent.ConfigSetter
	tag      names.Tag
	dataPath string
}

func (mc *mockConfig) Tag() names.Tag {
	if mc.tag == nil {
		return names.NewMachineTag("99")
	}
	return mc.tag
}

func (mc *mockConfig) Controller() names.ControllerTag {
	return testing.ControllerTag
}

func (mc *mockConfig) LogDir() string {
	return "log-dir"
}

func (mc *mockConfig) DataDir() string {
	if mc.dataPath != "" {
		return mc.dataPath
	}
	return "data-dir"
}

func (mc *mockConfig) LoggingConfig() string {
	return ""
}

func preUpgradeSteps(model.ModelType) upgrades.PreUpgradeStepsFunc { return nil }
