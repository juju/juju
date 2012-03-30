package testing

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/gozk/zookeeper"
	"net"
	"os"
	pathpkg "path"
)

type Fatalfer interface {
	Fatalf(format string, args ...interface{})
}

func chooseZkPort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// StartZkServer starts a ZooKeeper server in a temporary directory.
// It panics if it encounters an error.
func StartZkServer() *zookeeper.Server {
	// In normal use, dir will be deleted by srv.Destroy, and does not need to
	// be tracked separately.
	dir, err := ioutil.TempDir("", "test-zk")
	if err != nil {
		panic(fmt.Errorf("cannot create temporary directory: %v", err))
	}
	srv, err := zookeeper.CreateServer(chooseZkPort(), dir, "")
	if err != nil {
		os.RemoveAll(dir)
		panic(fmt.Errorf("cannot create ZooKeeper server: %v", err))
	}
	err = srv.Start()
	if err != nil {
		srv.Destroy()
		panic(fmt.Errorf("cannot start ZooKeeper server: %v", err))
	}
	return srv
}

// ZkRemoveTree recursively removes a zookeeper node
// and all its children; it panics if it encounters an error.
// It does not delete "/zookeeper" or the root node itself and it does not
// consider deleting a nonexistent node to be an error.
func ZkRemoveTree(zk *zookeeper.Conn, path string) {
	// If we try to delete the zookeeper node (for example when
	// calling ZkRemoveTree(zk, "/")) we silently ignore it.
	if path == "/zookeeper" {
		return
	}
	// First recursively delete the children.
	children, _, err := zk.Children(path)
	if err != nil {
		if zookeeper.IsError(err, zookeeper.ZNONODE) {
			return
		}
		panic(err)
	}
	for _, child := range children {
		ZkRemoveTree(zk, pathpkg.Join(path, child))
	}
	// Now delete the path itself unless it's the root node.
	if path == "/" {
		return
	}
	err = zk.Delete(path, -1)
	// Technically we can't get a ZNONODE error unless something
	// else is deleting nodes concurrently, because otherwise the
	// call to Children above would have failed, but check anyway
	// for completeness.
	if err != nil && !zookeeper.IsError(err, zookeeper.ZNONODE) {
		panic(err)
	}
}
