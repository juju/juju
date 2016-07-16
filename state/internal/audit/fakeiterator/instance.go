// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fakeiterator

import (
	"runtime"
	"strings"
	"sync"

	"encoding/json"

	"github.com/juju/testing"
)

type Instance struct {
	testing.Stub

	TestRecordJson []string

	close  sync.WaitGroup
	closed struct {
		sync.RWMutex

		value bool
	}
}

func (i *Instance) Init() *Instance {
	i.close.Add(1) // Simulate wait until Close is called.
	return i
}

func (i *Instance) Next(result interface{}) bool {
	i.AddCall(funcName(), result)

	i.closed.RLock()
	closed := i.closed.value
	i.closed.RUnlock()

	if closed {
		return false
	} else if len(i.TestRecordJson) <= 0 {
		i.close.Wait() // Simulate wait until Close is called.
		return false
	}

	if err := json.Unmarshal([]byte(i.TestRecordJson[0]), result); err != nil {
		panic(err)
	}
	i.TestRecordJson = i.TestRecordJson[1:]

	return true
}

func (i *Instance) Close() error {
	i.AddCall(funcName())
	i.closed.Lock()
	defer i.closed.Unlock()

	if i.closed.value == false {
		i.close.Done()
		i.closed.value = true
	}
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
