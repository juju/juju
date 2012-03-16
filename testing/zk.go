package testing

import (
	"io/ioutil"
	"launchpad.net/gozk/zookeeper"
	"os"
	pathpkg "path"
)

var ZkPort = 21812

type TestingT interface {
	Fatalf(format string, args ...interface{})
}

// StartZkServer starts a ZooKeeper server in a temporary directory.
// It calls Fatalf on t if it encounters an error.
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

// ZkRemoveTree recursively removes a zookeeper node
// and all its children, calling Fatalf on t if it encounters an error.
// It does not delete the /zookeeper node, and it does not
// consider deleting a nonexistent node to be an error.
func ZkRemoveTree(t TestingT, zk *zookeeper.Conn, path string) {
	// If we try to delete the zookeeper node (for example when
	// calling zkRemoveTree(zk, "/")) we silently ignore it.
	if path == "/zookeeper" {
		return
	}
	// First recursively delete the children.
	children, _, err := zk.Children(path)
	if err != nil {
		if zookeeper.IsError(err, zookeeper.ZNONODE) {
			return
		}
		t.Fatalf("%v", err)
	}
	for _, child := range children {
		ZkRemoveTree(t, zk, pathpkg.Join(path, child))
	}
	// Now delete the path itself.
	err = zk.Delete(path, -1)
	if err != nil {
		t.Fatalf("%v", err)
	}
}
