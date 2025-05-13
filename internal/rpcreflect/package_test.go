// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpcreflect_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/juju/tc"
)

func Test(t *testing.T) {
	tc.TestingT(t)
}

func callName(narg, nret int, retErr bool) string {
	e := ""
	if retErr {
		e = "e"
	}
	return fmt.Sprintf("Call%dr%d%s", narg, nret, e)
}

type callInfo struct {
	rcvr   interface{}
	method string
	arg    interface{}
}

type callError callInfo

func (e *callError) Error() string {
	return fmt.Sprintf("error calling %s", e.method)
}

type stringVal struct {
	Val string
}

type Root struct {
	mu          sync.Mutex
	calls       []*callInfo
	returnErr   bool
	simple      map[string]*SimpleMethods
	delayed     map[string]*DelayedMethods
	errorInst   *ErrorMethods
	contextInst *ContextMethods
}

func (r *Root) callError(rcvr interface{}, name string, arg interface{}) error {
	if r.returnErr {
		return &callError{rcvr, name, arg}
	}
	return nil
}

func (r *Root) SimpleMethods(id string) (*SimpleMethods, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.simple[id]; a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("unknown SimpleMethods id")
}

func (r *Root) DelayedMethods(id string) (*DelayedMethods, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.delayed[id]; a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("unknown DelayedMethods id")
}

func (r *Root) ErrorMethods(id string) (*ErrorMethods, error) {
	if r.errorInst == nil {
		return nil, fmt.Errorf("no error methods")
	}
	return r.errorInst, nil
}

func (r *Root) ContextMethods(id string) (*ContextMethods, error) {
	if r.contextInst == nil {
		return nil, fmt.Errorf("no context methods")
	}
	return r.contextInst, nil
}

func (r *Root) Discard1() {}

func (r *Root) Discard2(id string) error { return nil }

func (r *Root) Discard3(id string) int { return 0 }

func (r *Root) CallbackMethods(string) (*CallbackMethods, error) {
	return &CallbackMethods{r}, nil
}

func (r *Root) InterfaceMethods(id string) (InterfaceMethods, error) {
	m, err := r.SimpleMethods(id)
	if err != nil {
		return nil, err
	}
	return m, nil
}

type InterfaceMethods interface {
	Call1r1e(s stringVal) (stringVal, error)
}

type ChangeAPIMethods struct {
	r *Root
}

func (r *Root) ChangeAPIMethods(string) (*ChangeAPIMethods, error) {
	return &ChangeAPIMethods{r}, nil
}

func (t *Root) called(rcvr interface{}, method string, arg interface{}) {
	t.mu.Lock()
	t.calls = append(t.calls, &callInfo{rcvr, method, arg})
	t.mu.Unlock()
}

type SimpleMethods struct {
	root *Root
	id   string
}

// Each Call method is named in this standard form:
//
//     Call<narg>r<nret><e>
//
// where narg is the number of arguments, nret is the number of returned
// values (not including the error) and e is the letter 'e' if the
// method returns an error.

func (a *SimpleMethods) Call0r0() {
	a.root.called(a, "Call0r0", nil)
}

func (a *SimpleMethods) Call0r1() stringVal {
	a.root.called(a, "Call0r1", nil)
	return stringVal{"Call0r1 ret"}
}

func (a *SimpleMethods) Call0r1e() (stringVal, error) {
	a.root.called(a, "Call0r1e", nil)
	return stringVal{"Call0r1e ret"}, a.root.callError(a, "Call0r1e", nil)
}

func (a *SimpleMethods) Call0r0e() error {
	a.root.called(a, "Call0r0e", nil)
	return a.root.callError(a, "Call0r0e", nil)
}

func (a *SimpleMethods) Call1r0(s stringVal) {
	a.root.called(a, "Call1r0", s)
}

func (a *SimpleMethods) Call1r1(s stringVal) stringVal {
	a.root.called(a, "Call1r1", s)
	return stringVal{"Call1r1 ret"}
}

func (a *SimpleMethods) Call1r1e(s stringVal) (stringVal, error) {
	a.root.called(a, "Call1r1e", s)
	return stringVal{"Call1r1e ret"}, a.root.callError(a, "Call1r1e", s)
}

func (a *SimpleMethods) Call1r0e(s stringVal) error {
	a.root.called(a, "Call1r0e", s)
	return a.root.callError(a, "Call1r0e", s)
}

func (a *SimpleMethods) SliceArg(struct{ X []string }) stringVal {
	return stringVal{"SliceArg ret"}
}

func (a *SimpleMethods) Discard1(int) {}

func (a *SimpleMethods) Discard2(struct{}, struct{}) {}

func (a *SimpleMethods) Discard3() int { return 0 }

func (a *SimpleMethods) Discard4() (_, _ struct{}) { return }

type ContextMethods struct {
	root *Root
}

func (c *ContextMethods) Call0(ctx context.Context) error {
	c.root.called(c, "Call0", nil)
	return nil
}

func (c *ContextMethods) Call1(ctx context.Context, s stringVal) error {
	c.root.called(c, "Call1", nil)
	return nil
}

type DelayedMethods struct {
	ready     chan struct{}
	done      chan string
	doneError chan error
}

func (a *DelayedMethods) Delay() (stringVal, error) {
	if a.ready != nil {
		a.ready <- struct{}{}
	}
	select {
	case s := <-a.done:
		return stringVal{s}, nil
	case err := <-a.doneError:
		return stringVal{}, err
	}
}

type ErrorMethods struct {
	err error
}

func (e *ErrorMethods) Call() error {
	return e.err
}

type CallbackMethods struct {
	root *Root
}
