// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/controller"
	coredatabase "github.com/juju/juju/core/database"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalrecovery "github.com/juju/juju/internal/recovery"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

const (
	// recoveryArchivedMachine is the logical machine identity the fixture
	// archive records. It is deliberately not the bootstrap machine: the
	// tests read the machine-7 configuration and the machine-7 controller
	// node row, so a hardcoded bootstrap identity cannot pass them.
	recoveryArchivedMachine = "7"

	// recoveryControllerUUID is the fixture archive's source controller.
	recoveryControllerUUID = "c0ffee00-1111-2222-3333-444455556666"

	// recoveryControllerModelUUID is the fixture archive's controller
	// model.
	recoveryControllerModelUUID = "c0ffee00-aaaa-2222-3333-444455556666"
)

// recoveryMachineSuite exercises the populated parameters path of
// recover-state for machine replacements.
type recoveryMachineSuite struct {
	testhelpers.IsolationSuite
}

func TestRecoveryMachineSuite(t *testing.T) {
	tc.Run(t, &recoveryMachineSuite{})
}

// TestRecoverMachineInitialisesReplacement checks the success path of a
// machine replacement with populated parameters: the archived identity
// and password are kept, and Dqlite initialisation runs for the archived
// logical machine.
func (s *recoveryMachineSuite) TestRecoverMachineInitialisesReplacement(c *tc.C) {
	fixture := s.provisionRecovery(c, nil)
	calls := &recoverCalls{}
	command := s.newRecoverCommand(c, fixture, nil, calls)

	err := command.Run(cmdtesting.Context(c))
	c.Assert(err, tc.ErrorIsNil)

	// The identity and password provisioned from the archive are kept:
	// recovery never regenerates controller material or rotates the
	// password, so surviving agents keep trusting the replacement.
	identity, err := os.ReadFile(fixture.agentConfig.SystemIdentityPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(identity), tc.Equals, "provided-system-identity")
	stored, err := agent.ReadConfig(agent.ConfigPath(
		fixture.dataDir, names.NewMachineTag(recoveryArchivedMachine),
	))
	c.Assert(err, tc.ErrorIsNil)
	info, ok := stored.ControllerAgentInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(info.SystemIdentity, tc.Equals, "provided-system-identity")
	apiInfo, ok := stored.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(apiInfo.Password, tc.Equals, "agent-before")

	// Dqlite initialisation runs for the archived machine with the
	// resolved bind address.
	c.Check(calls.dqliteRuns, tc.Equals, 1)
	c.Check(calls.bindAddress, tc.Equals, "192.168.1.10")
	c.Check(calls.machineName, tc.Equals, recoveryArchivedMachine)

	// The claim is retained after success too: completing the import is
	// a later stage, and until then a rerun is refused, not replayed.
	marker, err := os.ReadFile(filepath.Join(fixture.dataDir, internalrecovery.StartedFile))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(marker), tc.Equals, fixture.checksum+"\n")
}

// TestRecoverRejectsFreshEmbeddedParams ensures the provenance guard also
// applies to the node configuration embedded in populated recovery params.
func (s *recoveryMachineSuite) TestRecoverRejectsFreshEmbeddedParams(c *tc.C) {
	fixture := s.provisionRecovery(c, nil)
	params, err := internalrecovery.ReadParams(fixture.dataDir)
	c.Assert(err, tc.ErrorIsNil)
	params.Node.RecoveryMode = false
	data, err := params.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(fixture.dataDir, agent.RecoveryParamsFile), data, 0600), tc.ErrorIsNil)

	calls := &recoverCalls{}
	command := s.newRecoverCommand(c, fixture, nil, calls)
	err = command.Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorMatches, "recovery params are not for a recovery allocation")
	c.Check(calls.dqliteRuns, tc.Equals, 0)
}

// TestRecoverMachineDqliteFailureRefusesReplay checks the claim ordering
// and retention: the marker is already on disk when Dqlite work starts,
// a failing run leaves it behind, and a rerun of the same replacement is
// refused without reaching Dqlite again.
func (s *recoveryMachineSuite) TestRecoverMachineDqliteFailureRefusesReplay(c *tc.C) {
	fixture := s.provisionRecovery(c, nil)
	expected := errors.New("dqlite failed")
	calls := &recoverCalls{}
	command := s.newRecoverCommand(c, fixture, expected, calls)

	err := command.Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorIs, expected)
	c.Check(calls.dqliteRuns, tc.Equals, 1)
	marker, err := os.ReadFile(filepath.Join(fixture.dataDir, internalrecovery.StartedFile))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(marker), tc.Equals, fixture.checksum+"\n")

	// Rerunning the same failed replacement is refused: the retained
	// marker means Dqlite never runs a second time against the partially
	// initialised database.
	err = command.Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorMatches,
		"recovery initialisation was already started; provision a new replacement")
	c.Check(calls.dqliteRuns, tc.Equals, 1)
}

// TestRecoverMachineOtherArchiveMarker checks that a marker belonging to
// a different archive is never adopted: recovery must not mix partial
// state from another attempt.
func (s *recoveryMachineSuite) TestRecoverMachineOtherArchiveMarker(c *tc.C) {
	fixture := s.provisionRecovery(c, nil)
	s.writeStartedMarker(c, fixture.dataDir, "another-attempt-checksum")

	calls := &recoverCalls{}
	command := s.newRecoverCommand(c, fixture, nil, calls)
	err := command.Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorMatches, "recovery marker belongs to another archive")
	c.Check(calls.dqliteRuns, tc.Equals, 0)
}

// TestRecoverMachineChecksumMismatch checks that an archive whose
// checksum disagrees with the operator-validated parameters is rejected
// before the claim is written and before Dqlite runs.
func (s *recoveryMachineSuite) TestRecoverMachineChecksumMismatch(c *tc.C) {
	fixture := s.provisionRecovery(c, func(setup *recoverySetup) {
		// A valid checksum, but not this archive's.
		setup.paramsChecksum = strings.Repeat("0", 64)
	})
	calls := &recoverCalls{}
	command := s.newRecoverCommand(c, fixture, nil, calls)

	err := command.Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorMatches,
		`archive checksum mismatch: expected sha256 "0+", archive is "sha256-[0-9a-f]+"`)
	c.Check(calls.dqliteRuns, tc.Equals, 0)
	// Validation precedes the claim: no marker is left behind.
	_, statErr := os.Stat(filepath.Join(fixture.dataDir, internalrecovery.StartedFile))
	c.Check(errors.Is(statErr, os.ErrNotExist), tc.IsTrue)
}

// TestRecoverMachineRefusesCrossVersionArchive checks the exact-version
// gate: an archive from another agent version is never recovered, even
// when its checksum matches.
func (s *recoveryMachineSuite) TestRecoverMachineRefusesCrossVersionArchive(c *tc.C) {
	fixture := s.provisionRecovery(c, func(setup *recoverySetup) {
		setup.metadataVersion = "3.43.1"
	})
	calls := &recoverCalls{}
	command := s.newRecoverCommand(c, fixture, nil, calls)

	err := command.Run(cmdtesting.Context(c))
	c.Check(err, tc.ErrorMatches,
		"archive was created by agent version 3.43.1 but this binary is .*; "+
			"recovery requires the exact same version")
	c.Check(calls.dqliteRuns, tc.Equals, 0)
	_, statErr := os.Stat(filepath.Join(fixture.dataDir, internalrecovery.StartedFile))
	c.Check(errors.Is(statErr, os.ErrNotExist), tc.IsTrue)
}

// TestRecoverDqliteAddsNodeRowWithoutSeeding checks recoverDqlite against
// a real Dqlite node: the controller node row survives the initialisation
// session and is readable from a fresh one, and no fresh bootstrap
// seeding lands in the database.
func (s *recoveryMachineSuite) TestRecoverDqliteAddsNodeRowWithoutSeeding(c *tc.C) {
	// go-dqlite derives its abstract TLS socket name from the initial node
	// ID, which is fixed even when the TCP port and data dir are unique.
	// Isolate it from Dqlite sessions in concurrently running test binaries.
	c.Setenv("SNAP_INSTANCE_NAME", "juju-recovery-test-"+strconv.Itoa(os.Getpid()))
	dataDir := c.MkDir()
	mgr := newLoopbackNodeManager(c, dataDir)
	logger := loggertesting.WrapCheckLog(c)
	const bindAddress = "127.0.0.1"
	err := recoverDqlite(c.Context(), mgr, bindAddress, recoveryArchivedMachine, logger)
	c.Assert(err, tc.ErrorIsNil)

	// Read the node row back through a fresh initialisation session on
	// the same data directory: the row must survive the session that
	// wrote it. The transaction only reads; the assertions run after it
	// so a retried transaction cannot carry partial results.
	var (
		nodeID, controllerID, bind, wantNodeID string
		nodeRows                               int
		seededRows                             map[string]int
	)
	seededTables := []string{"controller", "model", "controller_config"}
	err = database.WithDqlite(c.Context(), mgr, bindAddress, logger,
		func(ctx context.Context, session *database.DqliteSession) error {
			controller, err := session.OpenDatabase(ctx, coredatabase.ControllerNS, schema.ControllerDDL())
			if err != nil {
				return errors.Capture(err)
			}
			wantNodeID = strconv.FormatUint(session.NodeID(), 10)
			return controller.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
				// The transaction closure can be retried: clear what a
				// previous attempt may have populated.
				nodeRows, seededRows = 0, make(map[string]int, len(seededTables))
				rows, err := tx.QueryContext(ctx, `
SELECT controller_id, dqlite_node_id, dqlite_bind_address
FROM controller_node`)
				if err != nil {
					return errors.Capture(err)
				}
				defer rows.Close()
				for rows.Next() {
					nodeRows++
					if err := rows.Scan(&controllerID, &nodeID, &bind); err != nil {
						return errors.Capture(err)
					}
				}
				if err := rows.Err(); err != nil {
					return errors.Capture(err)
				}

				// No fresh bootstrap seeding: recovery initialises the
				// controller database alone and adds only the node row.
				for _, table := range seededTables {
					var count int
					if err := tx.QueryRowContext(ctx,
						"SELECT COUNT(*) FROM "+table,
					).Scan(&count); err != nil {
						return errors.Capture(err)
					}
					seededRows[table] = count
				}
				return nil
			})
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nodeRows, tc.Equals, 1)
	c.Check(controllerID, tc.Equals, recoveryArchivedMachine)
	c.Check(bind, tc.Equals, bindAddress)
	// The stored node identity is the one this node still has after the
	// restart: rejoining must not renumber the node.
	c.Check(nodeID, tc.Equals, wantNodeID)
	for _, table := range seededTables {
		c.Check(seededRows[table], tc.Equals, 0, tc.Commentf("table %s was seeded", table))
	}
}

// recoverySetup customises the provisioned replacement before it runs.
type recoverySetup struct {
	// metadataVersion is the agent version the archive metadata records.
	// It defaults to the running version: recovery is exact-version only.
	metadataVersion string

	// paramsChecksum overrides the checksum recorded in the recovery
	// parameters, decoupling it from the archive's actual checksum.
	paramsChecksum string
}

// recoveryFixture is a provisioned machine replacement: a data directory
// carrying populated recovery parameters referencing a valid archive,
// and the archived machine-7 agent configuration.
type recoveryFixture struct {
	dataDir     string
	agentConfig agent.Config
	modelConfig *config.Config
	checksum    string
}

// recoverCalls records what the stubbed Dqlite initialiser observed.
type recoverCalls struct {
	dqliteRuns  int
	bindAddress string
	machineName string
}

// provisionRecovery provisions a machine replacement data directory:
// populated recovery parameters referencing a valid archive, the
// bootstrap parameters delivered with the instance, and the archived
// agent configuration of machine 7.
func (s *recoveryMachineSuite) provisionRecovery(
	c *tc.C, setup func(*recoverySetup),
) recoveryFixture {
	stored := recoverySetup{metadataVersion: jujuversion.Current.String()}
	if setup != nil {
		setup(&stored)
	}
	archivePath, checksum := s.writeRecoveryArchive(c, stored.metadataVersion)

	dataDir := c.MkDir()
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	modelConfig := jujutesting.CustomModelConfig(c, jujutesting.Attrs{
		"name": "controller", "uuid": modelUUID.String(), "type": "dummy",
		"agent-version": jujuversion.Current.String(),
	})
	node := instancecfg.StateInitializationParams{
		RecoveryMode:          true,
		AgentVersion:          jujuversion.Current,
		ControllerConfig:      jujutesting.FakeControllerConfig(),
		ControllerModelConfig: modelConfig,
		ControllerCloud: cloud.Cloud{
			Name: "test-cloud", Type: "dummy", AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
		},
	}
	bootstrapParamsData, err := node.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(dataDir, cloudconfig.FileNameBootstrapParams), bootstrapParamsData, 0600)
	c.Assert(err, tc.ErrorIsNil)

	params := internalrecovery.Params{
		ArchivePath:  archivePath,
		SHA256:       checksum,
		AgentVersion: jujuversion.Current,
		MachineName:  recoveryArchivedMachine,
		Node:         node,
	}
	if stored.paramsChecksum != "" {
		params.SHA256 = stored.paramsChecksum
	}
	paramsData, err := params.Marshal()
	c.Assert(err, tc.ErrorIsNil)
	paramsPath := filepath.Join(dataDir, internalrecovery.ParamsFile)
	c.Assert(os.MkdirAll(filepath.Dir(paramsPath), 0o700), tc.ErrorIsNil)
	c.Assert(os.WriteFile(paramsPath, paramsData, 0o600), tc.ErrorIsNil)

	cfg, err := agent.NewStateMachineConfig(agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: dataDir},
		Tag:               names.NewMachineTag(recoveryArchivedMachine),
		UpgradedToVersion: jujuversion.Current, Password: "bootstrap-secret",
		Controller: jujutesting.ControllerTag, Model: names.NewModelTag(modelUUID.String()),
		APIAddresses: []string{"127.0.0.1:17070"}, CACert: jujutesting.CACert,
	}, controller.ControllerAgentInfo{
		Cert: jujutesting.ServerCert, PrivateKey: jujutesting.ServerKey,
		CAPrivateKey: jujutesting.CAKey, APIPort: 17070, SystemIdentity: "provided-system-identity",
	})
	c.Assert(err, tc.ErrorIsNil)
	// The archived password the surviving agents still trust.
	cfg.SetPassword("agent-before")
	c.Assert(cfg.Write(), tc.ErrorIsNil)
	return recoveryFixture{
		dataDir:     dataDir,
		agentConfig: cfg,
		modelConfig: modelConfig,
		checksum:    checksum,
	}
}

// newRecoverCommand returns a RecoverCommand with the environment lookup
// and Dqlite initialiser stubbed for a machine replacement: the
// initialiser records the inputs recoverDqlite would receive and returns
// dqliteErr. The stub also asserts the initialisation claim is on disk
// by the time Dqlite work starts.
func (s *recoveryMachineSuite) newRecoverCommand(
	c *tc.C, fixture recoveryFixture, dqliteErr error, calls *recoverCalls,
) *RecoverCommand {
	addresses := network.NewMachineAddresses([]string{"10.0.0.1"}).AsProviderAddresses()
	s.PatchValue(&environsNewIAAS, func(
		_ context.Context, args environs.OpenParams, _ environs.CredentialInvalidator,
	) (environs.Environ, error) {
		c.Check(args.Cloud.IsControllerCloud, tc.IsTrue)
		c.Check(args.ControllerUUID, tc.Equals, jujutesting.ControllerTag.Id())
		return &freshBootstrapEnviron{cfg: fixture.modelConfig, addresses: addresses}, nil
	})
	command := NewRecoverCommand()
	command.AgentConf = agentconf.NewAgentConf(fixture.dataDir)
	const bindAddress = "192.168.1.10"
	command.DqliteBindAddressResolver = func(
		_ context.Context, gotAddresses network.ProviderAddresses,
	) (string, error) {
		c.Check(gotAddresses, tc.DeepEquals, addresses)
		return bindAddress, nil
	}
	command.RecoverDqlite = func(
		_ context.Context, _ database.BootstrapNodeManager,
		gotBindAddress, gotMachineName string, _ corelogger.Logger,
	) error {
		calls.dqliteRuns++
		calls.bindAddress = gotBindAddress
		calls.machineName = gotMachineName
		// The exclusive claim precedes the Dqlite work: the marker is
		// already on disk when initialisation starts.
		marker, err := os.ReadFile(filepath.Join(fixture.dataDir, internalrecovery.StartedFile))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(string(marker), tc.Equals, fixture.checksum+"\n")
		return dqliteErr
	}
	return command
}

// writeStartedMarker writes the initialisation marker as a previous
// attempt with the given checksum would have left it.
func (s *recoveryMachineSuite) writeStartedMarker(c *tc.C, dataDir, checksum string) {
	filename := filepath.Join(dataDir, internalrecovery.StartedFile)
	c.Assert(os.MkdirAll(filepath.Dir(filename), 0o700), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filename, []byte(checksum+"\n"), 0o600), tc.ErrorIsNil)
}

// writeRecoveryArchive writes a minimal valid recovery archive whose
// metadata records agentVersion, and returns its path and SHA-256
// checksum (hex).
func (s *recoveryMachineSuite) writeRecoveryArchive(c *tc.C, agentVersion string) (string, string) {
	files := map[string][]byte{
		"juju-backup/metadata.json":                                        []byte(recoveryMetadataJSON(agentVersion)),
		"juju-backup/dump/controller.yaml":                                 []byte(recoveryControllerDump()),
		"juju-backup/dump/models/" + recoveryControllerModelUUID + ".yaml": []byte("payload: {}\n"),
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := files[name]
		hdr := &tar.Header{
			Name: name, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg,
		}
		c.Assert(tw.WriteHeader(hdr), tc.ErrorIsNil)
		_, err := tw.Write(data)
		c.Assert(err, tc.ErrorIsNil)
	}
	c.Assert(tw.Close(), tc.ErrorIsNil)
	c.Assert(gz.Close(), tc.ErrorIsNil)

	archive := buf.Bytes()
	sum := sha256.Sum256(archive)
	path := filepath.Join(c.MkDir(), "archive.tar.gz")
	c.Assert(os.WriteFile(path, archive, 0o600), tc.ErrorIsNil)
	return path, hex.EncodeToString(sum[:])
}

// recoveryMetadataJSON renders the archive's provenance record for the
// given agent version.
func recoveryMetadataJSON(agentVersion string) string {
	return `{` +
		`"ID":"20260930-120000.aabb-ccdd-eeff",` +
		`"FormatVersion":2,` +
		`"Checksum":"abc",` +
		`"ChecksumFormat":"SHA-256",` +
		`"Size":10,` +
		`"Stored":"0001-01-01T00:00:00Z",` +
		`"Started":"2026-09-30T12:00:00Z",` +
		`"Finished":"2026-09-30T12:00:34Z",` +
		`"Notes":"",` +
		`"ModelUUID":"` + recoveryControllerModelUUID + `",` +
		`"Machine":"` + recoveryArchivedMachine + `",` +
		`"Base":"ubuntu@24.04/stable",` +
		`"Arch":"amd64",` +
		`"Hostname":"myhost",` +
		`"Version":"` + agentVersion + `",` +
		`"ControllerUUID":"` + recoveryControllerUUID + `",` +
		`"HANodes":1,` +
		`"ControllerMachineID":"` + recoveryArchivedMachine + `",` +
		`"ControllerMachineInstanceID":"inst-` + recoveryArchivedMachine + `"` +
		`}` + "\n"
}

// recoveryControllerDump renders a minimal controller database dump: one
// lxd cloud carrying the controller model.
func recoveryControllerDump() string {
	return "version: 4.1.0\n" +
		"payload:\n" +
		"  controller:\n" +
		"  - uuid: " + recoveryControllerUUID + "\n" +
		"    model_uuid: " + recoveryControllerModelUUID + "\n" +
		"    ca_cert: source-ca-cert\n" +
		"    ca_private_key: source-ca-key\n" +
		"    system_identity: source-system-identity\n" +
		"  cloud:\n" +
		"  - uuid: cloud-lxd\n    name: lxd\n    cloud_type_id: 1\n" +
		"  cloud_type:\n" +
		"  - id: 1\n    type: lxd\n" +
		"  model_type:\n" +
		"  - id: 0\n    type: iaas\n" +
		"  model:\n" +
		"  - uuid: " + recoveryControllerModelUUID + "\n" +
		"    name: controller\n" +
		"    cloud_uuid: cloud-lxd\n" +
		"    model_type_id: 0\n"
}

// newLoopbackNodeManager returns a real node manager that binds its
// Dqlite application to an ephemeral loopback port, keeping the node
// identity stable across the initialisation and readback sessions.
func newLoopbackNodeManager(c *tc.C, dataDir string) *database.NodeManager {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, tc.ErrorIsNil)
	port := listener.Addr().(*net.TCPAddr).Port
	c.Assert(listener.Close(), tc.ErrorIsNil)
	return database.NewNodeManager(database.NodeManagerConfig{
		DataDir:              dataDir,
		DqlitePort:           port,
		CACert:               jujutesting.CACert,
		ControllerCert:       jujutesting.ServerCert,
		ControllerPrivateKey: jujutesting.ServerKey,
	}, loggertesting.WrapCheckLog(c), coredatabase.NoopSlowQueryLogger{})
}
