// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober

import (
	"fmt"
	"net/http"
	"strconv"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v3/catacomb"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/observability/probe"
)

type Mux interface {
	AddHandler(string, string, http.Handler) error
	RemoveHandler(string, string)
}

type Controller struct {
	catacomb catacomb.Catacomb
}

const (
	DetailedResponseQueryKey = "detailed"
	PathLivenessProbe        = "/liveness"
	PathReadinessProbe       = "/readiness"
	PathStartupProbe         = "/startup"
)

// NewController constructs a new caas prober Controller.
func NewController(probes *CAASProbes, mux Mux) (*Controller, error) {
	c := &Controller{}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &c.catacomb,
		Work: c.makeLoop(probes, mux),
	}); err != nil {
		return c, jujuerrors.Trace(err)
	}

	return c, nil
}

// Kill implements worker.Kill
func (c *Controller) Kill() {
	c.catacomb.Kill(nil)
}

// makeLoop is responsible for producing the loop needed to run as part of the
// controller worker.
func (c *Controller) makeLoop(
	probes *CAASProbes,
	mux Mux,
) func() error {
	return func() error {
		if err := mux.AddHandler(
			http.MethodGet,
			k8sconstants.AgentHTTPPathLiveness,
			ProbeHandler("liveness", probes.Liveness)); err != nil {
			return jujuerrors.Trace(err)
		}
		defer mux.RemoveHandler(http.MethodGet, PathLivenessProbe)

		if err := mux.AddHandler(
			http.MethodGet,
			k8sconstants.AgentHTTPPathReadiness,
			ProbeHandler("readiness", probes.Readiness)); err != nil {
			return jujuerrors.Trace(err)
		}
		defer mux.RemoveHandler(http.MethodGet, PathReadinessProbe)

		if err := mux.AddHandler(
			http.MethodGet,
			k8sconstants.AgentHTTPPathStartup,
			ProbeHandler("startup", probes.Startup)); err != nil {
			return jujuerrors.Trace(err)
		}
		defer mux.RemoveHandler(http.MethodGet, PathStartupProbe)

		select {
		case <-c.catacomb.Dying():
			return c.catacomb.ErrDying()
		}
	}
}

// ProbeHandler implements a http handler for the supplied probe and probe name.
func ProbeHandler(name string, aggProbe *probe.Aggregate) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		shouldDetailResponse := false
		detailedVals, exists := req.URL.Query()[DetailedResponseQueryKey]
		if exists && len(detailedVals) == 1 {
			val, err := strconv.ParseBool(detailedVals[0])
			if err != nil {
				http.Error(res, fmt.Sprintf("invalid detailed query value %s expected boolean", detailedVals[0]),
					http.StatusBadRequest)
				return
			}
			shouldDetailResponse = val
		}

		good, err := aggProbe.ProbeWithResultCallback(
			probe.ProbeResultCallback(func(probeKey string, val bool, err error) {
				if !shouldDetailResponse {
					return
				}

				// We are trying to output 1 line here per probe called.
				// The format should be:
				// + uniter # for success
				// - uniter: some error # for failure

				if val {
					// Print + on probe success
					fmt.Fprintf(res, "+ ")
				} else {
					// Print - on probe failure
					fmt.Fprintf(res, "- ")
				}

				// Print the probe name
				fmt.Fprint(res, probeKey)

				// Print the error if one exists
				if err != nil {
					fmt.Fprintf(res, ": %s", err)
				}

				// Finish the current line
				fmt.Fprintf(res, "\n")
			}),
		)

		if jujuerrors.IsNotImplemented(err) {
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

// Wait implements worker.Wait
func (c *Controller) Wait() error {
	return c.catacomb.Wait()
}
