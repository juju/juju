package testing

import (
	"io/ioutil"
	"launchpad.net/gozk/zookeeper"
	"os"
	pathpkg "path"
)

var ZkPort = 21812

type Fatalfer interface {
	Fatalf(format string, args ...interface{})
}

// StartZkServer starts a ZooKeeper server in a temporary directory.
// It calls Fatalf on t if it encounters an error.
func StartZkServer(t Fatalfer) *zookeeper.Server {
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
// It does not "/zookeeper" or the root node itself and it does not
// consider deleting a nonexistent node to be an error.
func ZkRemoveTree(t Fatalfer, zk *zookeeper.Conn, path string) {
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
	// Now delete the path itself unless it's the root node.
	if path == "/" {
		return
	}
	err = zk.Delete(path, -1)
	if err != nil {
		t.Fatalf("%v", err)
	}
}
