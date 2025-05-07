// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fakeobserver

import (
	"context"
	"net/http"
	"runtime"
	"strings"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc"
)

// Instance is a fake Observer used for testing.
type Instance struct {
	testhelpers.Stub
}

// Join implements Observer.
func (f *Instance) Join(ctx context.Context, req *http.Request, connectionID uint64) {
	f.AddCall(funcName(), req, connectionID)
}

// Leave implements Observer.
func (f *Instance) Leave(ctx context.Context) {
	f.AddCall(funcName())
}

// Login implements Observer.
func (f *Instance) Login(ctx context.Context, entity names.Tag, model names.ModelTag, modelUUID model.UUID, fromController bool, userData string) {
	f.AddCall(funcName(), entity, model, modelUUID, fromController, userData)
}

// RPCObserver implements Observer.
func (f *Instance) RPCObserver() rpc.Observer {
	// Stash the instance away in the call so that we can check calls
	// on it later.
	result := &RPCInstance{}
	f.AddCall(funcName(), result)
	return result
}

// NoRPCInstance is a fake Observer used for testing that does not
// implement RPCObserver.
type NoRPCInstance struct {
	Instance
}

// RPCObserver implements Observer.
func (f *NoRPCInstance) RPCObserver() rpc.Observer {
	return nil
}

// RPCInstance is a fake RPCObserver used for testing.
type RPCInstance struct {
	testhelpers.Stub
}

// ServerReply implements Observer.
func (f *RPCInstance) ServerReply(ctx context.Context, req rpc.Request, hdr *rpc.Header, body interface{}) {
	f.AddCall(funcName(), req, hdr, body)
}

// ServerRequest implements Observer.
func (f *RPCInstance) ServerRequest(ctx context.Context, hdr *rpc.Header, body interface{}) {
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
