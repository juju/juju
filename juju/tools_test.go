package juju


import (
	. "launchpad.net/gocheck"
	"os"
)
type ToolsSuite struct{}

var _ = Suite(&ToolsSuite{})

var files []struct {
	mode os.FileMode
	name string
	contents string
	archive bool
}

//func (ToolsSuite) TestArchive(c *C) {
//	create some files in a directory
//	call archive
//	pipe it into tar xzf
//	check 
