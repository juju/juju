// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type destroyModelSuite struct {
	testhelpers.IsolationSuite

	//modelManager *mockModelManager
}

var _ = tc.Suite(&destroyModelSuite{})

/*
func (s *destroyModelSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	otherModelTag := names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f33d")
	s.modelManager = &mockModelManager{
		models: []*mockModel{
			{tag: testing.ModelTag, currentStatus: status.StatusInfo{Status: status.Available}},
			{tag: otherModelTag, currentStatus: status.StatusInfo{Status: status.Available}},
		},
	}
}
.

func (s *destroyModelSuite) TestDestroyModel(c *tc.C) {
	true_ := true
	false_ := false
	zero := time.Second * 0
	one := time.Second
	s.testDestroyModel(c, nil, nil, nil, nil)
	s.testDestroyModel(c, nil, &false_, nil, nil)
	s.testDestroyModel(c, nil, &false_, &zero, nil)
	s.testDestroyModel(c, nil, &true_, nil, nil)
	s.testDestroyModel(c, nil, &true_, &zero, nil)
	s.testDestroyModel(c, &true_, nil, nil, nil)
	s.testDestroyModel(c, &true_, &false_, nil, nil)
	s.testDestroyModel(c, &true_, &false_, &zero, nil)
	s.testDestroyModel(c, &true_, &true_, nil, nil)
	s.testDestroyModel(c, &true_, &true_, &zero, nil)
	s.testDestroyModel(c, &false_, nil, nil, nil)
	s.testDestroyModel(c, &false_, &false_, nil, nil)
	s.testDestroyModel(c, &false_, &false_, &zero, nil)
	s.testDestroyModel(c, &false_, &true_, nil, nil)
	s.testDestroyModel(c, &false_, &true_, &zero, nil)
	s.testDestroyModel(c, &false_, &true_, &zero, &one)
}

func (s *destroyModelSuite) testDestroyModel(c *tc.C, destroyStorage, force *bool, maxWait, timeout *time.Duration) {
	s.modelManager.ResetCalls()

	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{FuncName: "GetBlockForType", Args: []interface{}{state.DestroyBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.RemoveBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.ChangeBlock}},
		{FuncName: "Model", Args: nil},
	})

	expectedModelCalls := []jtesting.StubCall{{"Destroy", []interface{}{state.DestroyModelParams{
		DestroyStorage: destroyStorage,
		Force:          force,
		MaxWait:        common.MaxWait(maxWait),
		Timeout:        timeout,
	}}}}
	notForcing := force == nil || !*force
	if notForcing {
		// We expect to check model status.
		expectedModelCalls = append([]jtesting.StubCall{{"Status", nil}}, expectedModelCalls...)
	}
	s.modelManager.models[0].CheckCalls(c, expectedModelCalls)
}

func (s *destroyModelSuite) TestDestroyModelBlocked(c *tc.C) {

	s.modelManager.CheckCallNames(c, "GetBlockForType")
	s.modelManager.models[0].CheckNoCalls(c)
}

func (s *destroyModelSuite) TestDestroyModelIgnoresErrorsWithForce(c *tc.C) {
	s.modelManager.models[0].SetErrors(
		errors.New("nope"),
	)

	s.modelManager.CheckCallNames(c, "GetBlockForType", "GetBlockForType", "GetBlockForType", "Model")
	s.modelManager.models[0].CheckCallNames(c, "Destroy")
}

func (s *destroyModelSuite) TestDestroyModelNotIgnoreErrorsrWithForce(c *tc.C) {
	s.modelManager.models[0].SetErrors(
		stateerrors.PersistentStorageError,

	s.modelManager.CheckCallNames(c, "GetBlockForType", "GetBlockForType", "GetBlockForType", "Model")
	s.modelManager.models[0].CheckCallNames(c, "Destroy")
}



	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{FuncName: "ControllerModelTag", Args: nil},
		{FuncName: "GetBlockForType", Args: []interface{}{state.DestroyBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.RemoveBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.ChangeBlock}},
		{FuncName: "Model", Args: nil},
	})
	s.modelManager.models[0].CheckCalls(c, []jtesting.StubCall{
		{FuncName: "Status", Args: nil},
		{FuncName: "Destroy", Args: []interface{}{state.DestroyModelParams{
			MaxWait: common.MaxWait(nil),
		}}},
	})
}


	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{FuncName: "ControllerModelTag", Args: nil},
		{FuncName: "GetBlockForType", Args: []interface{}{state.DestroyBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.RemoveBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.ChangeBlock}},
		{FuncName: "Model", Args: nil},
	})
	s.modelManager.models[0].CheckCalls(c, []jtesting.StubCall{
		{FuncName: "Status", Args: nil},
		{FuncName: "Destroy", Args: []interface{}{state.DestroyModelParams{
			DestroyStorage: &destroyStorage,
			MaxWait:        common.MaxWait(nil),
		}}},
	})
}

func (s *destroyModelSuite) TestDestroyControllerForce(c *tc.C) {
	force := true

	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{FuncName: "ControllerModelTag", Args: nil},
		{FuncName: "GetBlockForType", Args: []interface{}{state.DestroyBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.RemoveBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.ChangeBlock}},
		{FuncName: "Model", Args: nil},
	})
	s.modelManager.models[0].CheckCalls(c, []jtesting.StubCall{
		{FuncName: "Destroy", Args: []interface{}{state.DestroyModelParams{
			Force:   &force,
			Timeout: &timeout,
			MaxWait: maxWait,
		}}},
	})
}

	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{FuncName: "ControllerModelTag", Args: nil},
		{FuncName: "AllModelUUIDs", Args: nil},

		{FuncName: "GetBackend", Args: []interface{}{s.modelManager.models[0].tag.Id()}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.DestroyBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.RemoveBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.ChangeBlock}},

		{FuncName: "GetBackend", Args: []interface{}{s.modelManager.models[1].tag.Id()}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.DestroyBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.RemoveBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.ChangeBlock}},

		{FuncName: "GetBlockForType", Args: []interface{}{state.DestroyBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.RemoveBlock}},
		{FuncName: "GetBlockForType", Args: []interface{}{state.ChangeBlock}},
		{FuncName: "Model", Args: nil},
	})
	s.modelManager.models[0].CheckCalls(c, []jtesting.StubCall{
		{FuncName: "Status", Args: nil},
		{FuncName: "Destroy", Args: []interface{}{state.DestroyModelParams{
			DestroyHostedModels: true,
			MaxWait:             common.MaxWait(nil),
		}}},
	})
}

func (s *destroyModelSuite) TestDestroyControllerModelErrs(c *tc.C) {
	// This is similar to what we'd see if a model was destroyed
	// but there are still some connections to it lingering.
	s.modelManager.SetErrors(
		nil, // for GetBackend, 1st model
		nil, // for GetBlockForType, 1st model
		nil, // for GetBlockForType, 1st model
		nil, // for GetBlockForType, 1st model
c.Context()

	s.modelManager.SetErrors(
		nil,                            // for GetBackend, 1st model
		nil,                            // for GetBlockForType, 1st model
		nil,                            // for GetBlockForType, 1st model
		nil,                            // for GetBlockForType, 1st model

}

func (s *destroyModelSuite) TestDestroyModelWithInvalidCredentialWithoutForce(c *tc.C) {


func (s *destroyModelSuite) TestDestroyModelWithInvalidCredentialWithForce(c *tc.C) {
	s.modelManager.models[0].currentStatus = status.StatusInfo{Status: status.Suspended}
	true_ := true
	s.testDestroyModel(c, nil, &true_, nil, nil)
}

type mockModelManager struct {
	common.ModelManagerBackend
	jtesting.Stub

	models []*mockModel
}

func (m *mockModelManager) ControllerModelUUID() string {
	m.MethodCall(m, "ControllerModelUUID")
	return m.models[0].UUID()
}

func (m *mockModelManager) ControllerModelTag() names.ModelTag {
	m.MethodCall(m, "ControllerModelTag")
	return m.models[0].ModelTag()
}

func (m *mockModelManager) ModelTag() names.ModelTag {
	return testing.ModelTag
}

func (m *mockModelManager) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	m.MethodCall(m, "GetBlockForType", t)
	return nil, false, m.NextErr()
}

func (m *mockModelManager) AllModelUUIDs() ([]string, error) {
	m.MethodCall(m, "AllModelUUIDs")
	var out []string
	for _, model := range m.models {
		out = append(out, model.UUID())
	}
	return out, nil
}

func (m *mockModelManager) Model() (common.Model, error) {
	m.MethodCall(m, "Model")
	return m.models[0], m.NextErr()
}

func (m *mockModelManager) GetBackend(uuid string) (common.ModelManagerBackend, func() bool, error) {
	m.MethodCall(m, "GetBackend", uuid)
	return m, func() bool { return true }, m.NextErr()
}

func (m *mockModelManager) Close() error {
	m.MethodCall(m, "Close")
	return m.NextErr()
}

type mockModel struct {
	common.Model
	jtesting.Stub
	tag           names.ModelTag
	currentStatus status.StatusInfo
}

func (m *mockModel) ModelTag() names.ModelTag {
	return m.tag
}

func (m *mockModel) UUID() string {
	return m.tag.Id()
}

func (m *mockModel) Destroy(args state.DestroyModelParams) error {
	m.MethodCall(m, "Destroy", args)
	return m.NextErr()
}

func (m *mockModel) Status() (status.StatusInfo, error) {
	m.MethodCall(m, "Status")
	return m.currentStatus, m.NextErr()
}
*/
