// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logtailer

import (
	"fmt"
)

type loggerAdaptor struct{}

func (l loggerAdaptor) Fatal(args ...interface{}) {
	logger.Criticalf(fmt.Sprint(args...))
}

func (l loggerAdaptor) Fatalf(msg string, args ...interface{}) {
	logger.Criticalf(msg, args...)
}

func (l loggerAdaptor) Fatalln(args ...interface{}) {
	logger.Criticalf(fmt.Sprint(args...))
}

func (l loggerAdaptor) Panic(args ...interface{}) {
	logger.Criticalf(fmt.Sprint(args...))
}

func (l loggerAdaptor) Panicf(msg string, args ...interface{}) {
	logger.Criticalf(msg, args...)
}

func (l loggerAdaptor) Panicln(args ...interface{}) {
	logger.Criticalf(fmt.Sprint(args...))
}

func (l loggerAdaptor) Print(args ...interface{}) {
	logger.Infof(fmt.Sprint(args...))
}

func (l loggerAdaptor) Printf(msg string, args ...interface{}) {
	logger.Infof(msg, args...)
}

func (l loggerAdaptor) Println(args ...interface{}) {
	logger.Infof(fmt.Sprint(args...))
}
