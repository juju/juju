// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"testing"

	"github.com/juju/description/v10"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
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

type mockState struct {
	testhelpers.Stub

	environs.EnvironConfigGetter
	common.APIHostPortsForAgentsGetter

	controllerUUID  string
	cloudUsers      map[string]permission.Access
	model           *mockModel
	controllerModel *mockModel
	controllerNodes []commonmodel.ControllerNode
}

type fakeModelDescription struct {
	description.Model `yaml:"-"`

	ModelUUID string `yaml:"model-uuid"`
}

func (st *mockState) ControllerModelTag() names.ModelTag {
	st.MethodCall(st, "ControllerModelTag")
	return st.controllerModel.tag
}

func (st *mockState) Export(store objectstore.ObjectStore) (description.Model, error) {
	st.MethodCall(st, "Export")
	return &fakeModelDescription{ModelUUID: st.model.UUID()}, nil
}

func (st *mockState) AllModelUUIDs() ([]string, error) {
	st.MethodCall(st, "AllModelUUIDs")
	return []string{st.model.UUID()}, st.NextErr()
}

func (st *mockState) GetBackend(modelUUID string) (commonmodel.ModelManagerBackend, func() bool, error) {
	st.MethodCall(st, "GetBackend", modelUUID)
	err := st.NextErr()
	return st, func() bool { return true }, err
}

func (st *mockState) GetModel(modelUUID string) (commonmodel.Model, func() bool, error) {
	st.MethodCall(st, "GetModel", modelUUID)
	return st.model, func() bool { return true }, st.NextErr()
}

func (st *mockState) AllVolumes() ([]state.Volume, error) {
	st.MethodCall(st, "AllVolumes")
	return nil, st.NextErr()
}

func (st *mockState) AllFilesystems() ([]state.Filesystem, error) {
	st.MethodCall(st, "AllFilesystems")
	return nil, st.NextErr()
}

func (st *mockState) NewModel(args state.ModelArgs) (commonmodel.Model, commonmodel.ModelManagerBackend, error) {
	st.MethodCall(st, "NewModel", args)
	st.model.tag = names.NewModelTag(args.UUID.String())
	err := st.NextErr()
	return st.model, st, err
}

func (st *mockState) ControllerTag() names.ControllerTag {
	st.MethodCall(st, "ControllerTag")
	return names.NewControllerTag(st.controllerUUID)
}

func (st *mockState) IsController() bool {
	st.MethodCall(st, "IsController")
	return st.controllerUUID == st.model.UUID()
}

func (st *mockState) ControllerNodes() ([]commonmodel.ControllerNode, error) {
	st.MethodCall(st, "ControllerNodes")
	return st.controllerNodes, st.NextErr()
}

func (st *mockState) Model() (commonmodel.Model, error) {
	st.MethodCall(st, "Model")
	return st.model, st.NextErr()
}

func (st *mockState) ModelTag() names.ModelTag {
	st.MethodCall(st, "ModelTag")
	return st.model.ModelTag()
}

func (st *mockState) Close() error {
	st.MethodCall(st, "Close")
	return st.NextErr()
}

func (st *mockState) DumpAll() (map[string]interface{}, error) {
	st.MethodCall(st, "DumpAll")
	return map[string]interface{}{
		"models": "lots of data",
	}, st.NextErr()
}

func (st *mockState) HAPrimaryMachine() (names.MachineTag, error) {
	st.MethodCall(st, "HAPrimaryMachine")
	return names.MachineTag{}, nil
}

func (st *mockState) ConstraintsBySpaceName(spaceName string) ([]*state.Constraints, error) {
	st.MethodCall(st, "ConstraintsBySpaceName", spaceName)
	return nil, st.NextErr()
}

func (st *mockState) InvalidateModelCredential(reason string) error {
	st.MethodCall(st, "InvalidateModelCredential", reason)
	return nil
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
