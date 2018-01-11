// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fakeobserver

import (
	"net/http"
	"runtime"
	"strings"

	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/rpc"
)

// Instance is a fake Observer used for testing.
type Instance struct {
	testing.Stub
}

// Join implements Observer.
func (f *Instance) Join(req *http.Request, connectionID uint64) {
	f.AddCall(funcName(), req, connectionID)
}

// Leave implements Observer.
func (f *Instance) Leave() {
	f.AddCall(funcName())
}

// Login implements Observer.
func (f *Instance) Login(entity names.Tag, model names.ModelTag, fromController bool, userData string) {
	f.AddCall(funcName(), entity, model, fromController, userData)
}

// RPCObserver implements Observer.
func (f *Instance) RPCObserver() rpc.Observer {
	// Stash the instance away in the call so that we can check calls
	// on it later.
	result := &RPCInstance{}
	f.AddCall(funcName(), result)
	return result
}

// RPCInstance is a fake RPCObserver used for testing.
type RPCInstance struct {
	testing.Stub
}

// ServerReply implements Observer.
func (f *RPCInstance) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
	f.AddCall(funcName(), req, hdr, body)
}

// ServerRequest implements Observer.
func (f *RPCInstance) ServerRequest(hdr *rpc.Header, body interface{}) {
	f.AddCall(funcName(), hdr, body)
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
