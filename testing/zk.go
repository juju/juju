package testing

import (
	"io/ioutil"
	"launchpad.net/gozk/zookeeper"
	"os"
)

var ZkPort = 21812

type TestingT interface {
	Fatalf(format string, args ...interface{})
}

// StartZkServer starts a ZooKeeper server in a temporary directory.
func StartZkServer(t TestingT) *zookeeper.Server {
	// In normal use, dir will be deleted by srv.Destroy, and does not need to
	// be tracked separately.
	dir, err := ioutil.TempDir("", "test-zk")
	if err != nil {
		t.Fatalf("cannot create temporary directory: %v", err)
	}
	srv, err := zookeeper.CreateServer(ZkPort, dir, "")
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("cannot create ZooKeeper server: %v", err)
	}
	err = srv.Start()
	if err != nil {
		srv.Destroy()
		t.Fatalf("cannot start ZooKeeper server: %v", err)
	}
	return srv
}
