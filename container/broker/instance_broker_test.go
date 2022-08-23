package broker

import (
	"io"
	"os"
	"reflect"
	"testing"
)

func mockOpen(name string) (*os.File, error) {
	return os.Open(".")
}

func mockReaddirnamesInterfaces(f *os.File, n int) (names []string, err error) {
	return nil, io.EOF
}

func mockReaddirnamesNetplan(f *os.File, n int) (names []string, err error) {
	return nil, nil
}

func TestDefaultBridger(t *testing.T) {
	openFunc = mockOpen

	readDirFunc = mockReaddirnamesNetplan
	bridger, err := defaultBridger()
	if err != nil {
		t.Fail()
	}
	if reflect.TypeOf(bridger).Elem().Name() != "netplanBridger" {
		t.Fail()
	}

	readDirFunc = mockReaddirnamesInterfaces
	bridger, err = defaultBridger()
	if err != nil {
		t.Fail()
	}
	if reflect.TypeOf(bridger).Elem().Name() != "etcNetworkInterfacesBridger" {
		t.Fail()
	}
}

