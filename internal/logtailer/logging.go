// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logtailer

import (
	"context"
	"fmt"
)

type loggerAdaptor struct{}

func (l loggerAdaptor) Fatal(args ...any) {
	logger.Criticalf(context.Background(), fmt.Sprint(args...))
}

func (l loggerAdaptor) Fatalf(msg string, args ...any) {
	logger.Criticalf(context.Background(), msg, args...)
}

func (l loggerAdaptor) Fatalln(args ...any) {
	logger.Criticalf(context.Background(), fmt.Sprint(args...))
}

func (l loggerAdaptor) Panic(args ...any) {
	logger.Criticalf(context.Background(), fmt.Sprint(args...))
}

func (l loggerAdaptor) Panicf(msg string, args ...any) {
	logger.Criticalf(context.Background(), msg, args...)
}

func (l loggerAdaptor) Panicln(args ...any) {
	logger.Criticalf(context.Background(), fmt.Sprint(args...))
}

func (l loggerAdaptor) Print(args ...any) {
	logger.Infof(context.Background(), fmt.Sprint(args...))
}

func (l loggerAdaptor) Printf(msg string, args ...any) {
	logger.Infof(context.Background(), msg, args...)
}

func (l loggerAdaptor) Println(args ...any) {
	logger.Infof(context.Background(), fmt.Sprint(args...))
}
