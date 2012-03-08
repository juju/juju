package testutil

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"os"
	"testing"
)

func ZkTestingT(t *testing.T, zkAddr *string) {
	srv, dir, addr := ZkSetUpEnvironment(t)
	defer ZkTearDownEnvironment(t, srv, dir)
	*zkAddr = addr
	gocheck.TestingT(t)
}

// ZkSetUpEnvironment initializes the ZooKeeper test environment.
func ZkSetUpEnvironment(t *testing.T) (*zookeeper.Server, string, string) {
	dir, err := ioutil.TempDir("", "statetest")
	if err != nil {
		t.Fatalf("cannot create temporary directory: %v", err)
	}
	testRoot := dir + "/zookeeper"
	testPort := 21812
	srv, err := zookeeper.CreateServer(testPort, testRoot, "")
	if err != nil {
		t.Fatalf("cannot create ZooKeeper server: %v", err)
	}
	err = srv.Start()
	if err != nil {
		t.Fatalf("cannot start ZooKeeper server: %v", err)
	}
	return srv, dir, fmt.Sprint("localhost:", testPort)
}

// ZkTearDownEnvironment destroys the ZooKeeper test environment.
func ZkTearDownEnvironment(t *testing.T, srv *zookeeper.Server, dir string) {
	srv.Destroy()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal("cannot remove temporary directory: %v", err)
	}
}
