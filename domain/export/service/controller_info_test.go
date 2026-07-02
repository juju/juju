// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"

	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/internal/errors"
)

type controllerInfoServiceSuite struct{}

func TestControllerInfoServiceSuite(t *testing.T) {
	tc.Run(t, &controllerInfoServiceSuite{})
}

func (s *controllerInfoServiceSuite) TestGetControllerModelInfo(c *tc.C) {
	offerUUIDs := []string{"offer-1", "offer-2"}
	offererModels := []coremodelmigration.OffererModel{{
		ControllerUUID: "ctrl-1",
		ModelUUID:      "remote-model-1",
	}}
	expected := coremodelmigration.ControllerModelInfo{
		ModelInfo: coremodelmigration.ModelIdentityInfo{UUID: "model-uuid"},
	}

	svc := NewService(nil, ControllerInfoState{
		Controller: stubControllerModelInfoState{
			expectedModelUUID:     "model-uuid",
			expectedOfferUUIDs:    offerUUIDs,
			expectedOffererModels: offererModels,
			info:                  expected,
		},
		Model: stubModelControllerInfoState{
			offerUUIDs:    offerUUIDs,
			offererModels: offererModels,
		},
		ModelUUID: "model-uuid",
	})

	info, err := svc.GetControllerModelInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info, tc.DeepEquals, expected)
}

func (s *controllerInfoServiceSuite) TestGetControllerModelInfoOfferUUIDsError(c *tc.C) {
	svc := NewService(nil, ControllerInfoState{
		Controller: stubControllerModelInfoState{},
		Model: stubModelControllerInfoState{
			offerUUIDsErr: errors.New("boom"),
		},
		ModelUUID: "model-uuid",
	})

	_, err := svc.GetControllerModelInfo(c.Context())
	c.Assert(err, tc.ErrorMatches, "reading model offer UUIDs: boom")
}

func (s *controllerInfoServiceSuite) TestGetControllerModelInfoMissingState(c *tc.C) {
	svc := NewService(nil, ControllerInfoState{})

	_, err := svc.GetControllerModelInfo(c.Context())
	c.Assert(err, tc.ErrorMatches, "missing controller model info state")
}

type stubControllerModelInfoState struct {
	expectedModelUUID     string
	expectedOfferUUIDs    []string
	expectedOffererModels []coremodelmigration.OffererModel
	info                  coremodelmigration.ControllerModelInfo
}

func (s stubControllerModelInfoState) GetControllerModelInfo(
	_ context.Context,
	modelUUID string,
	offerUUIDs []string,
	offererModels []coremodelmigration.OffererModel,
) (coremodelmigration.ControllerModelInfo, error) {
	if modelUUID != s.expectedModelUUID {
		return coremodelmigration.ControllerModelInfo{}, errors.Errorf("model uuid %q", modelUUID)
	}
	if len(offerUUIDs) != len(s.expectedOfferUUIDs) {
		return coremodelmigration.ControllerModelInfo{}, errors.Errorf("offer uuid count %d", len(offerUUIDs))
	}
	if len(offererModels) != len(s.expectedOffererModels) {
		return coremodelmigration.ControllerModelInfo{}, errors.Errorf("offerer model count %d", len(offererModels))
	}
	return s.info, nil
}

type stubModelControllerInfoState struct {
	offerUUIDs       []string
	offerUUIDsErr    error
	offererModels    []coremodelmigration.OffererModel
	offererModelsErr error
}

func (s stubModelControllerInfoState) GetOfferUUIDs(context.Context) ([]string, error) {
	return s.offerUUIDs, s.offerUUIDsErr
}

func (s stubModelControllerInfoState) GetThirdPartyOffererModels(context.Context) ([]coremodelmigration.OffererModel, error) {
	return s.offererModels, s.offererModelsErr
}
