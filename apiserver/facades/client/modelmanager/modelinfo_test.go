// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"testing"

	"github.com/juju/description/v10"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	commonmodel "github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
)

type modelInfoSuite struct{}

func TestModelInfoSuite(t *testing.T) {
	tc.Run(t, &modelInfoSuite{})
}

func (s *modelInfoSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Test ModelInfo() with readAccess;
- Test ModelInfo() ErrorInvalidTag;
- Test ModelInfo() ErrorGetModelNotFound;
- Test ModelInfo() ErrorNoModelUsers;
- Test ModelInfo() ErrorNoAccess;
- Test ModelInfo() - running migration status;
`)
}

type fakeModelDescription struct {
	description.Model `yaml:"-"`

	ModelUUID string `yaml:"model-uuid"`
}

type mockModel struct {
	testhelpers.Stub
	owner  names.UserTag
	life   state.Life
	tag    names.ModelTag
	status status.StatusInfo
	cfg    *config.Config
}

func (m *mockModel) ModelTag() names.ModelTag {
	m.MethodCall(m, "ModelTag")
	return m.tag
}

func (m *mockModel) Type() state.ModelType {
	m.MethodCall(m, "Type")
	return state.ModelTypeIAAS
}

func (m *mockModel) Life() state.Life {
	m.MethodCall(m, "Life")
	return m.life
}

func (m *mockModel) Status() (status.StatusInfo, error) {
	m.MethodCall(m, "Status")
	return m.status, m.NextErr()
}

func (m *mockModel) Destroy(args state.DestroyModelParams) error {
	m.MethodCall(m, "Destroy", args)
	return m.NextErr()
}

func (m *mockModel) UUID() string {
	m.MethodCall(m, "UUID")
	return m.cfg.UUID()
}

type mockCredentialShim struct {
	commonmodel.ModelManagerBackend
}

func (s mockCredentialShim) InvalidateModelCredential(reason string) error {
	return nil
}

type mockObjectStore struct {
	objectstore.ObjectStore
}
