// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/state"
)

type stateShim struct {
	st *state.State
}

func (s stateShim) WatchApplicationsWithPendingCharms() state.StringsWatcher {
	return s.st.WatchApplicationsWithPendingCharms()
}

func (s stateShim) ControllerConfig() (controller.Config, error) {
	return s.st.ControllerConfig()
}

func (s stateShim) UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error) {
	return s.st.UpdateUploadedCharm(info)
}

func (s stateShim) PrepareCharmUpload(curl string) (services.UploadedCharm, error) {
	return s.st.PrepareCharmUpload(curl)
}

func (s stateShim) ModelUUID() string { return s.st.ModelUUID() }

func (s stateShim) Application(appName string) (Application, error) {
	app, err := s.st.Application(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return appShim{app}, nil
}

type appShim struct {
	app *state.Application
}

func (a appShim) CharmPendingToBeDownloaded() bool { return a.app.CharmPendingToBeDownloaded() }
func (a appShim) SetStatus(info status.StatusInfo, recorder status.StatusHistoryRecorder) error {
	return a.app.SetStatus(info, nil)
}
func (a appShim) SetDownloadedIDAndHash(id, hash string) error {
	return a.app.SetDownloadedIDAndHash(id, hash)
}

func (a appShim) CharmOrigin() *corecharm.Origin {
	if origin := a.app.CharmOrigin(); origin != nil {
		coreOrigin := origin.AsCoreCharmOrigin()
		return &coreOrigin
	}
	return nil
}

func (a appShim) Charm() (Charm, bool, error) {
	ch, force, err := a.app.Charm()
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	return ch, force, nil
}

type resourcesShim struct {
	facade.Resources
}

func (r resourcesShim) Register(res worker.Worker) string {
	return r.Resources.Register(res)
}
