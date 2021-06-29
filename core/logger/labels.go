// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import "github.com/juju/loggo/v2"

// Label type defines a label type used to categorize loggers.
type Label = string

const (
	HTTP Label = "http"
)

// GetAllContextLabels returns a list of labels associated with a
// loggo.Context. If no ctx is supplied, the default loggo context is used.
func GetAllContextLabels(ctx *loggo.Context) []string {
	if ctx == nil {
		ctx = loggo.DefaultContext()
	}
	return ctx.GetAllLoggerLabels()
}
