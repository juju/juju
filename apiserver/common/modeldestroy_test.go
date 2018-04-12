// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/metricsender"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
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
			{tag: testing.ModelTag},
			{tag: otherModelTag},
		},
	}
	s.metricSender = &testMetricSender{}
	s.PatchValue(common.SendMetrics, s.metricSender.SendMetrics)
}

func (s *destroyModelSuite) TestDestroyModelSendsMetrics(c *gc.C) {
	err := common.DestroyModel(s.modelManager, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.metricSender.CheckCalls(c, []jtesting.StubCall{
		{"SendMetrics", []interface{}{s.modelManager}},
	})
}

func (s *destroyModelSuite) TestDestroyModel(c *gc.C) {
	true_ := true
	false_ := false
	s.testDestroyModel(c, nil)
	s.testDestroyModel(c, &true_)
	s.testDestroyModel(c, &false_)
}

func (s *destroyModelSuite) testDestroyModel(c *gc.C, destroyStorage *bool) {
	s.modelManager.ResetCalls()
	s.modelManager.models[0].ResetCalls()

	err := common.DestroyModel(s.modelManager, destroyStorage)
	c.Assert(err, jc.ErrorIsNil)

	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{"GetBlockForType", []interface{}{state.DestroyBlock}},
		{"GetBlockForType", []interface{}{state.RemoveBlock}},
		{"GetBlockForType", []interface{}{state.ChangeBlock}},
		{"Model", nil},
	})

	s.modelManager.models[0].CheckCalls(c, []jtesting.StubCall{
		{"Destroy", []interface{}{state.DestroyModelParams{
			DestroyStorage: destroyStorage,
		}}},
	})
}

func (s *destroyModelSuite) TestDestroyModelBlocked(c *gc.C) {
	s.modelManager.SetErrors(errors.New("nope"))

	err := common.DestroyModel(s.modelManager, nil)
	c.Assert(err, gc.ErrorMatches, "nope")

	s.modelManager.CheckCallNames(c, "GetBlockForType")
	s.modelManager.models[0].CheckNoCalls(c)
}

func (s *destroyModelSuite) TestDestroyControllerNonControllerModel(c *gc.C) {
	s.modelManager.models[0].tag = s.modelManager.models[1].tag
	err := common.DestroyController(s.modelManager, false, nil)
	c.Assert(err, gc.ErrorMatches, `expected state for controller model UUID deadbeef-0bad-400d-8000-4b1d0d06f33d, got deadbeef-0bad-400d-8000-4b1d0d06f00d`)
}

func (s *destroyModelSuite) TestDestroyController(c *gc.C) {
	err := common.DestroyController(s.modelManager, false, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.modelManager.CheckCalls(c, []jtesting.StubCall{
		{"ControllerModelTag", nil},
		{"GetBlockForType", []interface{}{state.DestroyBlock}},
		{"GetBlockForType", []interface{}{state.RemoveBlock}},
		{"GetBlockForType", []interface{}{state.ChangeBlock}},
		{"Model", nil},
	})
	s.modelManager.models[0].CheckCalls(c, []jtesting.StubCall{
		{"Destroy", []interface{}{state.DestroyModelParams{}}},
	})
}

func (s *destroyModelSuite) TestDestroyControllerReleaseStorage(c *gc.C) {
	destroyStorage := false
	err := common.DestroyController(s.modelManager, false, &destroyStorage)
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
			DestroyStorage: &destroyStorage,
		}}},
	})
}

func (s *destroyModelSuite) TestDestroyControllerDestroyHostedModels(c *gc.C) {
	err := common.DestroyController(s.modelManager, true, nil)
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
		{"Destroy", []interface{}{state.DestroyModelParams{
			DestroyHostedModels: true,
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
	err := common.DestroyController(s.modelManager, true, nil)
	// Processing continued despite one model erring out.
	c.Assert(err, jc.ErrorIsNil)

	s.modelManager.SetErrors(
		nil, // for GetBackend, 1st model
		nil, // for GetBlockForType, 1st model
		nil, // for GetBlockForType, 1st model
		nil, // for GetBlockForType, 1st model
		errors.New("I have a problem"), // for GetBackend, 2nd model
	)
	err = common.DestroyController(s.modelManager, true, nil)
	// Processing erred out since a model seriously failed.
	c.Assert(err, gc.ErrorMatches, "I have a problem")
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
	tag names.ModelTag
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
