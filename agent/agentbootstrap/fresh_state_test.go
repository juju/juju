// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	jujuversion "github.com/juju/juju/core/version"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	pkissh "github.com/juju/juju/internal/pki/ssh"
	_ "github.com/juju/juju/internal/provider/kubernetes"
	_ "github.com/juju/juju/internal/provider/lxd"
	jujutesting "github.com/juju/juju/internal/testing"
)

type freshStateSuite struct {
	schematesting.ControllerModelSuite
}

func TestFreshStateSuite(t *testing.T) {
	tc.Run(t, &freshStateSuite{})
}

func (s *freshStateSuite) TestPreparationDoesNotChangeConfigOrState(c *tc.C) {
	b, cfg := s.newBootstrap(c, "lxd")
	before, err := cfg.Render()
	c.Assert(err, tc.ErrorIsNil)
	info, ok := cfg.ControllerAgentInfo()
	c.Assert(ok, tc.IsTrue)

	modelUUID, _, err := b.prepareFreshState(info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelUUID.String(), tc.Equals, b.stateInitialisationParams.ControllerModelConfig.UUID())
	after, err := cfg.Render()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(after, tc.DeepEquals, before)

	var count int
	err = s.DB().QueryRow("SELECT COUNT(*) FROM controller").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
	err = s.DB().QueryRow("SELECT COUNT(*) FROM user").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *freshStateSuite) TestFreshIAASState(c *tc.C) {
	s.assertFreshState(c, "lxd", coremodel.IAAS)
}

func (s *freshStateSuite) TestFreshCAASState(c *tc.C) {
	s.assertFreshState(c, cloud.CloudTypeKubernetes, coremodel.CAAS)
}

func (s *freshStateSuite) assertFreshState(c *tc.C, cloudType string, modelType coremodel.ModelType) {
	b, cfg := s.newBootstrap(c, cloudType)
	info, ok := cfg.ControllerAgentInfo()
	c.Assert(ok, tc.IsTrue)
	b.bootstrapDqlite = func(ctx context.Context, _ database.BootstrapNodeManager, _ network.ProviderAddresses, modelUUID coremodel.UUID, _ corelogger.Logger, ops ...database.BootstrapOpt) error {
		// The local password is changed only once all seed operations succeed.
		apiInfo, ok := cfg.APIInfo()
		c.Assert(ok, tc.IsTrue)
		c.Check(apiInfo.Password, tc.Equals, "agent-before")
		modelDB := s.ModelTxnRunner(c, modelUUID.String())
		for _, op := range ops {
			if err := op(ctx, s.ControllerTxnRunner(), modelDB); err != nil {
				return err
			}
		}
		var gotModelUUID, gotControllerUUID, gotType string
		var isController bool
		err := modelDB.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			return tx.QueryRowContext(ctx, "SELECT uuid, controller_uuid, type, is_controller_model FROM model").Scan(
				&gotModelUUID, &gotControllerUUID, &gotType, &isController,
			)
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Check(gotModelUUID, tc.Equals, modelUUID.String())
		c.Check(gotControllerUUID, tc.Equals, jujutesting.ControllerTag.Id())
		c.Check(gotType, tc.Equals, modelType.String())
		c.Check(isController, tc.IsTrue)
		return nil
	}

	err := b.Initialise(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	var cert, privateKey, caPrivateKey, systemIdentity string
	err = s.DB().QueryRow("SELECT cert, private_key, ca_private_key, system_identity FROM controller").Scan(
		&cert, &privateKey, &caPrivateKey, &systemIdentity,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cert, tc.Equals, info.Cert)
	c.Check(privateKey, tc.Equals, info.PrivateKey)
	c.Check(caPrivateKey, tc.Equals, info.CAPrivateKey)
	c.Check(systemIdentity, tc.Equals, info.SystemIdentity)

	var hash string
	var salt []byte
	err = s.DB().QueryRow(`SELECT p.password_hash, p.password_salt
FROM user_password AS p JOIN user AS u ON u.uuid = p.user_uuid
WHERE u.name = 'admin'`).Scan(&hash, &salt)
	c.Assert(err, tc.ErrorIsNil)
	password := auth.NewPassword("bootstrap-secret")
	defer password.Destroy()
	expectedHash, err := auth.HashPassword(password, salt)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hash, tc.Equals, expectedHash)
	apiInfo, ok := cfg.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(apiInfo.Password, tc.Not(tc.Equals), "agent-before")
	c.Check(apiInfo.Password, tc.Not(tc.Equals), "")
}

func (s *freshStateSuite) TestDatabaseFailureDoesNotChangeAgentConfig(c *tc.C) {
	b, cfg := s.newBootstrap(c, "lxd")
	before, err := cfg.Render()
	c.Assert(err, tc.ErrorIsNil)
	expected := errors.New("database failed")
	b.bootstrapDqlite = func(context.Context, database.BootstrapNodeManager, network.ProviderAddresses, coremodel.UUID, corelogger.Logger, ...database.BootstrapOpt) error {
		return expected
	}
	err = b.Initialise(c.Context())
	c.Check(err, tc.ErrorIs, expected)
	after, err := cfg.Render()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(after, tc.DeepEquals, before)
}

func (s *freshStateSuite) TestUnsupportedVersionDoesNotOpenDatabases(c *tc.C) {
	b, cfg := s.newBootstrap(c, "lxd")
	before, err := cfg.Render()
	c.Assert(err, tc.ErrorIsNil)
	b.stateInitialisationParams.AgentVersion = jujuversion.Current
	b.stateInitialisationParams.AgentVersion.Major++
	err = b.Initialise(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.AgentVersionNotSupported)
	after, err := cfg.Render()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(after, tc.DeepEquals, before)
}

func (s *freshStateSuite) newBootstrap(c *tc.C, cloudType string) (*AgentBootstrap, agent.ConfigSetterWriter) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	tag := names.Tag(names.NewMachineTag(agent.BootstrapControllerId))
	if cloudType == cloud.CloudTypeKubernetes {
		tag = names.NewControllerAgentTag(agent.BootstrapControllerId)
	}
	cfg, err := agent.NewStateMachineConfig(agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: c.MkDir()},
		Tag:               tag,
		UpgradedToVersion: jujuversion.Current,
		Password:          "bootstrap-secret",
		Controller:        jujutesting.ControllerTag,
		Model:             names.NewModelTag(modelUUID.String()),
		APIAddresses:      []string{"127.0.0.1:17070"},
		CACert:            jujutesting.CACert,
	}, controller.ControllerAgentInfo{
		Cert: jujutesting.ServerCert, PrivateKey: jujutesting.ServerKey,
		CAPrivateKey: jujutesting.CAKey, APIPort: 17070, SystemIdentity: "system-identity",
	})
	c.Assert(err, tc.ErrorIsNil)
	cfg.SetPassword("agent-before")
	hostKey, err := pkissh.NewMarshalledED25519()
	c.Assert(err, tc.ErrorIsNil)
	b, err := NewAgentBootstrap(AgentBootstrapArgs{
		AdminUser: names.NewLocalUserTag("admin"), AgentConfig: cfg,
		BootstrapEnviron: stubBootstrapEnviron{},
		StateInitialisationParams: instancecfg.StateInitializationParams{
			ControllerConfig: jujutesting.FakeControllerConfig(),
			ControllerModelConfig: jujutesting.CustomModelConfig(c, jujutesting.Attrs{
				"name": "controller", "uuid": modelUUID.String(), "type": cloudType,
				"agent-version": jujuversion.Current.String(),
			}),
			ControllerCloud:  cloud.Cloud{Name: "test-cloud", Type: cloudType, AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType}},
			SSHServerHostKey: string(hostKey),
		},
		BootstrapDqlite: func(context.Context, database.BootstrapNodeManager, network.ProviderAddresses, coremodel.UUID, corelogger.Logger, ...database.BootstrapOpt) error {
			c.Fatal("unexpected database initialisation")
			return nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return b, cfg
}
