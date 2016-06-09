// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fakeobserver

import (
	"net/http"
	"runtime"

	"strings"

	"github.com/juju/juju/rpc"
	"github.com/juju/testing"
)

type Instance struct {
	testing.Stub
}

func (f *Instance) Join(req *http.Request) {
	f.AddCall(funcName(), req)
}

func (f *Instance) Leave() {
	f.AddCall(funcName())
}

// ServerReply implements Observer.
func (f *Instance) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
	f.AddCall(funcName(), req, hdr, body)
}

// ServerRequest implements Observer.
func (f *Instance) ServerRequest(hdr *rpc.Header, body interface{}) {
	f.AddCall(funcName(), hdr, body)
}

// Login implements Observer.
func (f *Instance) Login(entityName string) {
	f.AddCall(funcName(), entityName)
}

// ClientRequest implements Observer.
func (f *Instance) ClientRequest(hdr *rpc.Header, body interface{}) {
	f.AddCall(funcName(), hdr, body)
}

// ClientReply implements Observer.
func (f *Instance) ClientReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
	f.AddCall(funcName(), req, hdr, body)
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
