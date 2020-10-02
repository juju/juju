// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober

import (
	"errors"
	"fmt"
	"net/http"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v2/catacomb"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
)

type Mux interface {
	AddHandler(string, string, http.Handler) error
	RemoveHandler(string, string)
}

type Controller struct {
	catacomb catacomb.Catacomb
}

const (
	PathLivenessProbe  = "/liveness"
	PathReadinessProbe = "/readiness"
	PathStartupProbe   = "/startup"
)

func NewController(probes CAASProbes, mux Mux) (*Controller, error) {
	c := &Controller{}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &c.catacomb,
		Work: c.makeLoop(probes, mux),
	}); err != nil {
		return c, jujuerrors.Trace(err)
	}

	return c, nil
}

func (c *Controller) Kill() {
	c.catacomb.Kill(nil)
}

func (c *Controller) makeLoop(
	probes CAASProbes,
	mux Mux,
) func() error {
	return func() error {
		if err := mux.AddHandler(
			http.MethodGet,
			k8sconstants.AgentHTTPPathLiveness,
			ProbeHandler("liveness", probes.LivenessProbe)); err != nil {
			return jujuerrors.Trace(err)
		}
		defer mux.RemoveHandler(http.MethodGet, PathLivenessProbe)

		if err := mux.AddHandler(
			http.MethodGet,
			k8sconstants.AgentHTTPPathReadiness,
			ProbeHandler("readiness", probes.ReadinessProbe)); err != nil {
			return jujuerrors.Trace(err)
		}
		defer mux.RemoveHandler(http.MethodGet, PathReadinessProbe)

		if err := mux.AddHandler(
			http.MethodGet,
			k8sconstants.AgentHTTPPathStartup,
			ProbeHandler("startup", probes.StartupProbe)); err != nil {
			return jujuerrors.Trace(err)
		}
		defer mux.RemoveHandler(http.MethodGet, PathStartupProbe)

		select {
		case <-c.catacomb.Dying():
			return c.catacomb.ErrDying()
		}
	}
}

func ProbeHandler(name string, supplier func() Prober) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		good, err := supplier().Probe()
		if errors.Is(err, ErrorProbeNotImplemented) {
			http.Error(res, fmt.Sprintf("%s: probe %s",
				http.StatusText(http.StatusNotImplemented), name),
				http.StatusNotImplemented)
			return
		}
		if err != nil {
			http.Error(res, fmt.Sprintf("%s: probe %s",
				http.StatusText(http.StatusInternalServerError), name),
				http.StatusInternalServerError)
			return
		}

		if !good {
			http.Error(res, fmt.Sprintf("%s: probe %s",
				http.StatusText(http.StatusTeapot), name),
				http.StatusTeapot)
			return
		}

		res.Header().Set("Content-Type", "text/plain; charset=utf-8")
		res.WriteHeader(http.StatusOK)
		fmt.Fprintf(res, "%s: probe %s", http.StatusText(http.StatusOK), name)
	})
}

func (c *Controller) Wait() error {
	return c.catacomb.Wait()
}
