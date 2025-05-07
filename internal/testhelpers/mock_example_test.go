package testhelpers_test

import (
	"fmt"
	"log"

	"github.com/juju/loggo/v2"

	testing "github.com/juju/juju/internal/testhelpers"
)

type ExampleInterfaceToMock interface {
	Add(a, b int) int
	Div(a, b int) (int, error)
}

type fakeType struct {
	ExampleInterfaceToMock
	*testing.CallMocker
}

func (f *fakeType) Add(a, b int) int {
	results := f.MethodCall(f, "Add", a, b)
	return results[0].(int)
}

func (f *fakeType) Div(a, b int) (int, error) {
	results := f.MethodCall(f, "Div", a, b)
	return results[0].(int), testing.TypeAssertError(results[1])
}

type ExampleTypeWhichUsesInterface struct {
	calculator ExampleInterfaceToMock
}

func (e *ExampleTypeWhichUsesInterface) Add(nums ...int) int {
	var tally int
	for n := range nums {
		tally = e.calculator.Add(tally, n)
	}
	return tally
}

func (e *ExampleTypeWhichUsesInterface) Div(nums ...int) (int, error) {
	var tally int
	var err error
	for n := range nums {
		tally, err = e.calculator.Div(tally, n)
		if err != nil {
			break
		}
	}
	return tally, err
}

func Example() {
	var logger loggo.Logger

	// Set a fake type which mocks out calls.
	mock := &fakeType{CallMocker: testing.NewCallMocker(logger)}
	mock.Call("Add", 1, 1).Returns(2)
	mock.Call("Div", 1, 1).Returns(1, nil)
	mock.Call("Div", 1, 0).Returns(0, fmt.Errorf("cannot divide by zero"))

	// Pass in the mock which satisifes a dependency of
	// ExampleTypeWhichUsesInterface. This allows us to inject mocked
	// calls.
	example := ExampleTypeWhichUsesInterface{calculator: mock}
	if example.Add(1, 1) != 2 {
		log.Fatal("unexpected result")
	}

	if result, err := example.Div(1, 1); err != nil {
		log.Fatalf("unexpected error: %v", err)
	} else if result != 1 {
		log.Fatal("unexpected result")
	}

	if _, err := example.Div(1, 0); err == nil {
		log.Fatal("did not receive expected divide by zero error")
	}
}
