// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// fakeconnection implements a fake Conn type utilized for testing.
package fakeconnection

import (
	"net/http"
	"runtime"
	"strings"

	"github.com/juju/testing"
)

type Instance struct {
	testing.Stub
}

func (f *Instance) Request() *http.Request {
	f.AddCall(funcName())
	return &http.Request{}
}

func (f *Instance) Send(data ...interface{}) error {
	f.AddCall(funcName(), data...)
	return nil
}

func (f *Instance) Close() error {
	f.AddCall(funcName())
	return nil
}

// funcName returns the name of the function/method that called
// funcName() It panics if this is not possible.
func funcName() string {
	if pc, _, _, ok := runtime.Caller(1); ok == false {
		panic("could not find function name")
	} else {
		parts := strings.Split(runtime.FuncForPC(pc).Name(), ".")
		return parts[len(parts)-1]
	}
}
