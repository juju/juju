// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package logreader implements the API for
// retrieving log messages from the API server.
package logreader

import (
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
)

// LogReader is the interface that allows reading
// log messages transmitted by the server.
type LogReader interface {
	// ReadLogs returns a channel that can be used to receive log
	// messages.
	ReadLogs() chan params.LogRecordResult
	worker.Worker //TODO maybe not?
}

// API provides access to the LogReader API.
type API struct {
	facade    base.FacadeCaller
	connector base.StreamConnector
}

// NewAPI creates a new client-side logsender API.
func NewAPI(api base.APICaller) *API {
	facade := base.NewFacadeCaller(api, "RsyslogConfig")
	return &API{facade: facade, connector: api}
}

// WatchRsyslogConfig returns a watcher that notifies interested
// parties when the rsyslog config values change.
func (api *API) WatchRsyslogConfig(agentTag names.Tag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag.String()}},
	}
	err := api.facade.FacadeCall("WatchRsyslogConfig", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(api.facade.RawAPICaller(), result)
	return w, nil
}

// RsyslogConfig holds config values for rsyslog forwarding.
type RsyslogConfig struct {
	URL        string
	CACert     string
	ClientCert string
	ClientKey  string
}

// Configured returns whether or not the config has been
// configured or not.
func (c *RsyslogConfig) Configured() bool {
	return c.URL != "" && c.CACert != "" && c.ClientCert != "" && c.ClientKey != ""
}

// RsyslogURLConfig returns the url of the rsyslog server.
func (api *API) RsyslogConfig(agentTag names.Tag) (*RsyslogConfig, error) {
	var results params.RsyslogConfigResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag.String()}},
	}
	err := api.facade.FacadeCall("RsyslogConfig", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, err
	}
	return &RsyslogConfig{
		URL:        result.URL,
		CACert:     result.CACert,
		ClientCert: result.ClientCert,
		ClientKey:  result.ClientKey,
	}, nil
}

// LogReader returns a  structure that implements
// the LogReader interface,
// which must be closed when finished with.
func (api *API) LogReader() (LogReader, error) {
	conn, err := api.connector.ConnectStream(
		"/log",
		url.Values{
			"format":  []string{"json"},
			"all":     []string{"true"},
			"backlog": []string{"10"},
		},
	)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot connect to /log")
	}
	return newLogsWatcher(conn), nil
}

type reader struct {
	conn base.Stream
	tomb tomb.Tomb
	out  chan params.LogRecordResult
}

func newLogsWatcher(conn base.Stream) *reader {
	out := make(chan params.LogRecordResult, 1)
	w := &reader{
		conn: conn,
		out:  out,
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		defer w.conn.Close()
		w.tomb.Kill(w.loop())
	}()
	return w
}

// ReadLogs implements the LogReader interface.
func (r *reader) ReadLogs() chan params.LogRecordResult {
	return r.out
}

func (r *reader) loop() error {
	for {
		var record params.LogRecordResult
		var logRecord params.LogRecord
		err := r.conn.ReadJSON(&logRecord)
		if err != nil {
			return err
		} else {
			record.LogRecord = logRecord
		}
		select {
		case <-r.tomb.Dying():
			return tomb.ErrDying
		case r.out <- record:
		}
	}
}

// Kill implements Worker.Kill()
func (r *reader) Kill() {
	r.tomb.Kill(nil)
}

// Wait implements Worker.Wait()
func (r *reader) Wait() error {
	return r.tomb.Wait()
}
