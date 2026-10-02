// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controllerlokiupdater keeps the controller runtime config file
// in sync with the controller-wide Loki endpoint configuration stored in
// the logging domain.
//
// The standalone controller binary (jujud) reads Loki endpoint settings
// from runtime.conf through its startup-value provider when the logrouter
// worker starts or bounces. The control socket worker writes Loki config
// changes to the controller database, but nothing in the original design
// propagated those changes back to runtime.conf. Without this worker,
// changing the Loki endpoint on a running standalone controller leaves
// runtime.conf stale, so the logrouter re-reads old values and never
// switches backends.
//
// The jujuagentd machine-agent path does not need this worker because the
// lokiendpointupdater worker watches the database via the API facade and
// writes changes to agent.conf through Agent.ChangeConfig. The standalone
// controller has no agent.conf and cannot use that path.
//
// # Loki config update pipeline
//
// The full propagation pipeline for the standalone controller is:
//
//	control socket (handleSetLokiEndpoint)
//	  -> logging domain service (SetLokiConfig)
//	    -> controller database (logging_loki_config table)
//	      -> controllerlokiupdater (WatchLokiConfig notification)
//	        -> runtime.conf (LokiEndpoint, LokiCACert, etc.)
//	          -> RuntimeConfigChanged voyeur.Value set
//	            -> controller-log-router (re-reads CurrentLokiConfig)
//
// When no Loki config exists in the database (Loki removed), the worker
// writes empty values to runtime.conf so the logrouter defaults to
// logsink mode.
//
// See github.com/juju/juju/internal/worker/lokiendpointupdater for the
// equivalent jujuagentd machine-agent path that writes to agent.conf.
// See github.com/juju/juju/internal/worker/controllerlogger for the
// controller-only logging configuration worker. See
// github.com/juju/juju/internal/worker/domainservices for the
// controller-local service source that supplies the logging domain
// service used here.
package controllerlokiupdater
