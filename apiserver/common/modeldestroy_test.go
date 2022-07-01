// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/apiserver/common"
	"github.com/juju/juju/v3/apiserver/facades/agent/metricsender"
	"github.com/juju/juju/v3/core/status"
	"github.com/juju/juju/v3/state"
	stateerrors "github.com/juju/juju/v3/state/errors"
	"github.com/juju/juju/v3/testing"
)

type destroyModelSuite struct {
	jtesting.IsolationSuite

	modelManager *mockModelManager
	metricSender *testMetricSender
}

var _ = gc.Suite(&destroyModelSuite{})

func (s *destroyModelSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	otherModelTag := names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f33d")
	s.modelManager = &mockModelManager{
		models: []*mockModel{
			{tag: testing.ModelTag, currentStatus: status.StatusInfo{Status: status.Available}},
			{tag: otherModelTag, currentStatus: status.StatusInfo{Status: status.Available}},
		},
	}
	s.metricSender = &testMetricSender{}
	s.PatchValue(common.SendMetrics, s.metricSender.SendMetrics)
}

func (s *destroyModelSuite) TestDestroyModelSendsMetrics(c *gc.C) {
	err := common.DestroyModel(s.modelManager, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.metricSender.CheckCalls(c, []jtesting.StubCall{
		{"SendMetrics", []interface{}{s.modelManager}},
	})
}

func (s *destroyModelSuite) TestDestroyModel(c *gc.C) {
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

func (s *destroyModelSuite) testDestroyModel(c *gc.C, destroyStorage, force *bool, maxWait, timeout *time.Duration) {
	s.modelManager.ResetCalls()
	s.modelManager.models[0].ResetCalls()

	err := common.DestroyModel(s.modelManager, destroyStorage, force, maxWait, timeout)
	c.Assert(err, jc.ErrorIsNil)

	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{"GetBlockForType", []interface{}{state.DestroyBlock}},
		{"GetBlockForType", []interface{}{state.RemoveBlock}},
		{"GetBlockForType", []interface{}{state.ChangeBlock}},
		{"Model", nil},
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

func (s *destroyModelSuite) TestDestroyModelBlocked(c *gc.C) {
	s.modelManager.SetErrors(errors.New("nope"))

	err := common.DestroyModel(s.modelManager, nil, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, "nope")

	s.modelManager.CheckCallNames(c, "GetBlockForType")
	s.modelManager.models[0].CheckNoCalls(c)
}

func (s *destroyModelSuite) TestDestroyModelIgnoresErrorsWithForce(c *gc.C) {
	s.modelManager.models[0].SetErrors(
		errors.New("nope"),
	)

	true_ := true
	err := common.DestroyModel(s.modelManager, nil, &true_, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.modelManager.CheckCallNames(c, "GetBlockForType", "GetBlockForType", "GetBlockForType", "Model")
	s.modelManager.models[0].CheckCallNames(c, "Destroy")
}

func (s *destroyModelSuite) TestDestroyModelNotIgnoreErrorsrWithForce(c *gc.C) {
	s.modelManager.models[0].SetErrors(
		stateerrors.NewHasPersistentStorageError(),
	)
	true_ := true
	err := common.DestroyModel(s.modelManager, nil, &true_, nil, nil)
	c.Assert(err, jc.Satisfies, state.IsHasPersistentStorageError)

	s.modelManager.CheckCallNames(c, "GetBlockForType", "GetBlockForType", "GetBlockForType", "Model")
	s.modelManager.models[0].CheckCallNames(c, "Destroy")
}

func (s *destroyModelSuite) TestDestroyControllerNonControllerModel(c *gc.C) {
	s.modelManager.models[0].tag = s.modelManager.models[1].tag
	err := common.DestroyController(s.modelManager, false, nil, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, `expected state for controller model UUID deadbeef-0bad-400d-8000-4b1d0d06f33d, got deadbeef-0bad-400d-8000-4b1d0d06f00d`)
}

func (s *destroyModelSuite) TestDestroyController(c *gc.C) {
	err := common.DestroyController(s.modelManager, false, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{"ControllerModelTag", nil},
		{"GetBlockForType", []interface{}{state.DestroyBlock}},
		{"GetBlockForType", []interface{}{state.RemoveBlock}},
		{"GetBlockForType", []interface{}{state.ChangeBlock}},
		{"Model", nil},
	})
	s.modelManager.models[0].CheckCalls(c, []jtesting.StubCall{
		{"Status", nil},
		{"Destroy", []interface{}{state.DestroyModelParams{
			MaxWait: common.MaxWait(nil),
		}}},
	})
}

func (s *destroyModelSuite) TestDestroyControllerReleaseStorage(c *gc.C) {
	destroyStorage := false
	err := common.DestroyController(s.modelManager, false, &destroyStorage, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{"ControllerModelTag", nil},
		{"GetBlockForType", []interface{}{state.DestroyBlock}},
		{"GetBlockForType", []interface{}{state.RemoveBlock}},
		{"GetBlockForType", []interface{}{state.ChangeBlock}},
		{"Model", nil},
	})
	s.modelManager.models[0].CheckCalls(c, []jtesting.StubCall{
		{"Status", nil},
		{"Destroy", []interface{}{state.DestroyModelParams{
			DestroyStorage: &destroyStorage,
			MaxWait:        common.MaxWait(nil),
		}}},
	})
}

func (s *destroyModelSuite) TestDestroyControllerForce(c *gc.C) {
	force := true
	timeout := time.Hour
	maxWait := time.Second
	err := common.DestroyController(s.modelManager, false, nil, &force, &maxWait, &timeout)
	c.Assert(err, jc.ErrorIsNil)

	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{"ControllerModelTag", nil},
		{"GetBlockForType", []interface{}{state.DestroyBlock}},
		{"GetBlockForType", []interface{}{state.RemoveBlock}},
		{"GetBlockForType", []interface{}{state.ChangeBlock}},
		{"Model", nil},
	})
	s.modelManager.models[0].CheckCalls(c, []jtesting.StubCall{
		{"Destroy", []interface{}{state.DestroyModelParams{
			Force:   &force,
			Timeout: &timeout,
			MaxWait: maxWait,
		}}},
	})
}

func (s *destroyModelSuite) TestDestroyControllerDestroyHostedModels(c *gc.C) {
	err := common.DestroyController(s.modelManager, true, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{"ControllerModelTag", nil},
		{"AllModelUUIDs", nil},

		{"GetBackend", []interface{}{s.modelManager.models[0].tag.Id()}},
		{"GetBlockForType", []interface{}{state.DestroyBlock}},
		{"GetBlockForType", []interface{}{state.RemoveBlock}},
		{"GetBlockForType", []interface{}{state.ChangeBlock}},

		{"GetBackend", []interface{}{s.modelManager.models[1].tag.Id()}},
		{"GetBlockForType", []interface{}{state.DestroyBlock}},
		{"GetBlockForType", []interface{}{state.RemoveBlock}},
		{"GetBlockForType", []interface{}{state.ChangeBlock}},

		{"GetBlockForType", []interface{}{state.DestroyBlock}},
		{"GetBlockForType", []interface{}{state.RemoveBlock}},
		{"GetBlockForType", []interface{}{state.ChangeBlock}},
		{"Model", nil},
	})
	s.modelManager.models[0].CheckCalls(c, []jtesting.StubCall{
		{"Status", nil},
		{"Destroy", []interface{}{state.DestroyModelParams{
			DestroyHostedModels: true,
			MaxWait:             common.MaxWait(nil),
		}}},
	})
	s.metricSender.CheckCalls(c, []jtesting.StubCall{
		// One call per hosted model, and one for the controller model.
		{"SendMetrics", []interface{}{s.modelManager}},
		{"SendMetrics", []interface{}{s.modelManager}},
		{"SendMetrics", []interface{}{s.modelManager}},
	})
}

func (s *destroyModelSuite) TestDestroyControllerModelErrs(c *gc.C) {
	// This is similar to what we'd see if a model was destroyed
	// but there are still some connections to it lingering.
	s.modelManager.SetErrors(
		nil, // for GetBackend, 1st model
		nil, // for GetBlockForType, 1st model
		nil, // for GetBlockForType, 1st model
		nil, // for GetBlockForType, 1st model
		errors.NotFoundf("pretend I am not here"), // for GetBackend, 2nd model
	)
	err := common.DestroyController(s.modelManager, true, nil, nil, nil, nil)
	// Processing continued despite one model erring out.
	c.Assert(err, jc.ErrorIsNil)

	s.modelManager.SetErrors(
		nil,                            // for GetBackend, 1st model
		nil,                            // for GetBlockForType, 1st model
		nil,                            // for GetBlockForType, 1st model
		nil,                            // for GetBlockForType, 1st model
		errors.New("I have a problem"), // for GetBackend, 2nd model
	)
	err = common.DestroyController(s.modelManager, true, nil, nil, nil, nil)
	// Processing erred out since a model seriously failed.
	c.Assert(err, gc.ErrorMatches, "I have a problem")
}

func (s *destroyModelSuite) TestDestroyModelWithInvalidCredentialWithoutForce(c *gc.C) {
	s.modelManager.models[0].currentStatus = status.StatusInfo{Status: status.Suspended}

	err := common.DestroyModel(s.modelManager, nil, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, "invalid cloud credential, use --force")
}

func (s *destroyModelSuite) TestDestroyModelWithInvalidCredentialWithForce(c *gc.C) {
	s.modelManager.models[0].currentStatus = status.StatusInfo{Status: status.Suspended}
	true_ := true
	s.testDestroyModel(c, nil, &true_, nil, nil)
}

type testMetricSender struct {
	jtesting.Stub
}

func (t *testMetricSender) SendMetrics(st metricsender.ModelBackend) error {
	t.MethodCall(t, "SendMetrics", st)
	return t.NextErr()
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
