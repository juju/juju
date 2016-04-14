// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslogger

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/syslog"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/logreader"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
)

var (
	dialSyslog = func(network, raddr string, priority syslog.Priority, tag string, tlsCfg *tls.Config) (SysLogger, error) {
		return syslog.Dial(network, raddr, priority, tag, tlsCfg)
	}

	longWait = time.Second
)

type SysLogger interface {
	Crit(string) error
	Err(string) error
	Warning(string) error
	Notice(string) error
	Info(string) error
	Debug(string) error
	Write([]byte) (int, error)
}

type LogAPI interface {
	WatchRsyslogConfig(names.Tag) (watcher.NotifyWatcher, error)
	RsyslogConfig(names.Tag) (*logreader.RsyslogConfig, error)
	LogReader() (logreader.LogReader, error)
}

// NewRsysWorker returns a worker.Worker that uses the notify watcher returned
// from the setup.
func NewRsysWorker(api LogAPI, agentConfig agent.Config) (worker.Worker, error) {
	logger := &rsysLogger{
		api:         api,
		agentConfig: agentConfig,
	}
	return worker.NewSimpleWorker(logger.loop), nil
}

// RsysLogger uses the api/logreader to tail the log
// collection and forwards all log messages to a
// rsyslog server.
type rsysLogger struct {
	api         LogAPI
	agentConfig agent.Config
}

func (r *rsysLogger) loop(stop <-chan struct{}) error {
	configWatcher, err := r.api.WatchRsyslogConfig(r.agentConfig.Tag())
	if err != nil {
		return errors.Trace(err)
	}

	stopChannel := make(chan struct{})
	runningLogger := false
	for {
		select {
		case <-stop:
			select {
			case stopChannel <- struct{}{}:
			default:
			}
			return nil
		case <-configWatcher.Changes():
			// try to stop the running logger by
			// sending an empty struct to the
			// stopChannel
			if runningLogger {
				select {
				case stopChannel <- struct{}{}:
				case <-time.After(longWait):
				}
				runningLogger = false
			}
			// run a new logger
			writer, err := newSysLogger(r.api, r.agentConfig)
			if err == nil && writer != nil {
				go r.readLoop(writer, stopChannel)
				runningLogger = true
			}
		}
	}
}

func (r *rsysLogger) readLoop(writer SysLogger, stop <-chan struct{}) error {
	logReader, err := r.api.LogReader()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		logReader.Close()
	}()

	logChannel := logReader.ReadLogs()
	for {
		select {
		case <-stop:
			return nil
		case rec := <-logChannel:
			if rec.Error != nil {
				continue
			}
			// below we ignore the returned error because logging
			// an error would just mean generating more logs to
			// send - and fail
			switch rec.Level {
			case loggo.CRITICAL:
				writer.Crit(formatLogRecord(rec))
			case loggo.ERROR:
				writer.Err(formatLogRecord(rec))
			case loggo.WARNING:
				writer.Warning(formatLogRecord(rec))
			case loggo.INFO:
				writer.Info(formatLogRecord(rec))
			case loggo.DEBUG:
				writer.Debug(formatLogRecord(rec))
			}
		}
	}
}

func formatLogRecord(r params.LogRecordResult) string {
	return fmt.Sprintf("%s: %s %s %s %s %s\n",
		r.ModelUUID,
		formatTime(r.Time),
		r.Level.String(),
		r.Module,
		r.Location,
		r.Message,
	)
}

func formatTime(t time.Time) string {
	return t.In(time.UTC).Format(time.RFC3339)
}

func newSysLogger(api LogAPI, agentConfig agent.Config) (SysLogger, error) {
	config, err := api.RsyslogConfig(agentConfig.Tag())
	if err != nil {
		return nil, errors.Trace(err)
	}

	if config.Configured() {
		caCertPool := x509.NewCertPool()
		ok := caCertPool.AppendCertsFromPEM([]byte(config.CACert))
		if !ok {
			return nil, errors.Errorf("failed to parse the ca certificate")
		}

		clientCert, err := tls.X509KeyPair([]byte(config.ClientCert), []byte(config.ClientKey))
		if err != nil {
			return nil, errors.Annotate(err, "failed to parse the client certificate/key")
		}

		writer, err := dialSyslog(
			"tcp",
			config.URL,
			syslog.LOG_EMERG,
			"juju-syslog",
			&tls.Config{
				RootCAs:      caCertPool,
				Certificates: []tls.Certificate{clientCert},
				// TODO (ashipika) remove this
				InsecureSkipVerify: true,
			},
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return writer, nil
	}
	return nil,
		errors.Errorf("configured: url %v, ca cert %v, client cert %v, client key %v",
			config.URL != "",
			config.CACert != "",
			config.ClientCert != "",
			config.ClientKey != "",
		)
}
