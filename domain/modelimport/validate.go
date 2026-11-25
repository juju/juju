// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport

import (
	"strconv"
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/export/types/latest"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/sequence"
	"github.com/juju/juju/internal/errors"
)

// ValidatePayload validates target-version payload invariants that must hold
// before any target-side import writes proceed.
func ValidatePayload(payload latest.ModelExport) error {
	if len(payload.ModelAgent) != 1 {
		return errors.Errorf(
			"model export payload has %d model_agent rows, expected 1: %w",
			len(payload.ModelAgent), coreerrors.NotValid)
	}
	appNames := make(map[string]string, len(payload.Application))
	for _, app := range payload.Application {
		appNames[app.UUID] = app.Name
	}
	aliveUnits := make(map[string]int)
	for _, unit := range payload.Unit {
		if unit.LifeID == 0 {
			aliveUnits[unit.ApplicationUUID]++
		}
	}
	for _, scale := range payload.ApplicationScale {
		name, ok := appNames[scale.ApplicationUUID]
		if !ok {
			return errors.Errorf("imported scale has no application %q: %w", scale.ApplicationUUID, coreerrors.NotValid)
		}
		if scale.Scale != nil && *scale.Scale != int64(aliveUnits[scale.ApplicationUUID]) {
			return errors.Errorf("imported application %q has scale %d but %d Alive units: scale request is not materialised: %w",
				name, *scale.Scale, aliveUnits[scale.ApplicationUUID], coreerrors.NotValid)
		}
	}
	return validateK8sUnitRanges(payload)
}

func validateK8sUnitRanges(payload latest.ModelExport) error {
	appNames := k8sApplicationsForValidation(payload)
	alive, highWater, err := collectK8sUnitOrdinals(payload, appNames)
	if err != nil {
		return errors.Capture(err)
	}
	sequences := importedUnitSequences(payload)
	for id, name := range appNames {
		namespace := sequence.MakePrefixNamespace(application.ApplicationSequenceNamespace, name).String()
		last := max(sequences[namespace], highWater[id])
		if err := application.ValidateK8sUnitRange(name, alive[id], last); err != nil {
			return errors.Errorf("imported application %q has incompatible unit ordinals: %w", name, err).Add(coreerrors.NotValid)
		}
	}
	return nil
}

func k8sApplicationsForValidation(payload latest.ModelExport) map[string]string {
	// Model rows identify Kubernetes payloads even when no legacy scale rows or
	// registered pods exist. Legacy scale rows also identify partial payloads.
	k8sModel := len(payload.Model) > 0 && payload.Model[0].Type == coremodel.CAAS.String()
	legacyApps := make(map[string]bool, len(payload.ApplicationScale))
	for _, scale := range payload.ApplicationScale {
		legacyApps[scale.ApplicationUUID] = true
	}
	syntheticCharms := make(map[string]bool)
	for _, charm := range payload.Charm {
		syntheticCharms[charm.UUID] = charm.SourceID == 2
	}
	appNames := make(map[string]string)
	for _, app := range payload.Application {
		if app.LifeID != int64(domainlife.Alive) || syntheticCharms[app.CharmUUID] {
			continue
		}
		if !k8sModel && (len(payload.Model) > 0 || !legacyApps[app.UUID]) {
			continue
		}
		appNames[app.UUID] = app.Name
	}
	return appNames
}

// collectK8sUnitOrdinals returns Alive membership and ordinal high-water marks
// keyed by application UUID, including the history of departing units and pods.
func collectK8sUnitOrdinals(
	payload latest.ModelExport, appNames map[string]string,
) (map[string][]coreunit.Name, map[string]uint64, error) {
	podIDs := make(map[string]string, len(payload.K8sPod))
	for _, pod := range payload.K8sPod {
		podIDs[pod.UnitUUID] = pod.ProviderID
	}
	alive := make(map[string][]coreunit.Name)
	highWater := make(map[string]uint64)
	for _, unit := range payload.Unit {
		name, ok := appNames[unit.ApplicationUUID]
		if !ok {
			continue
		}
		parsed, err := coreunit.NewName(unit.Name)
		if err != nil || parsed.Application() != name {
			return nil, nil, errors.Errorf("imported application %q has invalid unit name %q: %w", name, unit.Name, coreerrors.NotValid)
		}
		if unit.LifeID == int64(domainlife.Alive) {
			alive[unit.ApplicationUUID] = append(alive[unit.ApplicationUUID], parsed)
		}
		highWater[unit.ApplicationUUID] = max(highWater[unit.ApplicationUUID], uint64(parsed.Number()))
		ordinalText, ok := strings.CutPrefix(podIDs[unit.UUID], name+"-")
		ordinal, err := strconv.Atoi(ordinalText)
		if ok && err == nil && ordinal >= 0 && uint64(ordinal) > highWater[unit.ApplicationUUID] {
			highWater[unit.ApplicationUUID] = uint64(ordinal)
		}
	}
	return alive, highWater, nil
}

func importedUnitSequences(payload latest.ModelExport) map[string]uint64 {
	sequences := make(map[string]uint64, len(payload.Sequence))
	for _, seq := range payload.Sequence {
		if seq.Value >= 0 {
			sequences[seq.Namespace] = uint64(seq.Value)
		}
	}
	return sequences
}
