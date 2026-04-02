// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"fmt"
	"io"

	corelogger "github.com/juju/juju/core/logger"
)

// ProgressBar defines a progress bar type for giving feedback to the user about
// the state of the download.
type ProgressBar interface {
	io.Writer

	// Start progress with max "total" steps.
	Start(label string, total float64)
	// Finished the progress display
	Finished()
}

type LoggingProgressBar struct {
	logger  corelogger.Logger
	label   string
	total   float64
	written float64
}

var _ ProgressBar = (*LoggingProgressBar)(nil)

// NewLoggingProgressBar creates a progress bar that logs percentage updates.
func NewLoggingProgressBar(logger corelogger.Logger) ProgressBar {
	return &LoggingProgressBar{
		logger: logger,
	}
}

// Start progress with max "total" steps.
func (p *LoggingProgressBar) Start(label string, total float64) {
	p.label = label
	p.total = total
	p.written = 0
}

// Finished the progress display.
func (*LoggingProgressBar) Finished() {}

func (p *LoggingProgressBar) Write(bs []byte) (n int, err error) {
	n = len(bs)
	p.written += float64(n)

	if p.logger == nil {
		return n, nil
	}

	if p.label == "" {
		p.logger.Tracef(context.TODO(), "download progress %s complete", p.percent())
		return n, nil
	}

	p.logger.Tracef(context.TODO(), `download %q progress %s complete`, p.label, p.percent())
	return n, nil
}

func (p *LoggingProgressBar) percent() string {
	if p.total == 0 {
		return "100%"
	}
	q := p.written * 100 / p.total
	if q > 100 || q < 0 {
		return "???%"
	}
	return fmt.Sprintf("%3.0f%%", q)
}
