// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package shell

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/utils"
	"github.com/juju/utils/filepath"
)

// unixRenderer is the base shell renderer for "unix" shells.
type unixRenderer struct {
	filepath.UnixRenderer
}

// Quote implements Renderer.
func (unixRenderer) Quote(str string) string {
	// This *may* not be correct for *all* unix shells...
	return utils.ShQuote(str)
}

// ExeSuffix implements Renderer.
func (unixRenderer) ExeSuffix() string {
	return ""
}

// Mkdir implements Renderer.
func (ur unixRenderer) Mkdir(dirname string) []string {
	dirname = ur.Quote(dirname)
	return []string{
		fmt.Sprintf("mkdir %s", dirname),
	}
}

// MkdirAll implements Renderer.
func (ur unixRenderer) MkdirAll(dirname string) []string {
	dirname = ur.Quote(dirname)
	return []string{
		fmt.Sprintf("mkdir -p %s", dirname),
	}
}

// Chmod implements Renderer.
func (ur unixRenderer) Chmod(path string, perm os.FileMode) []string {
	path = ur.Quote(path)
	return []string{
		fmt.Sprintf("chmod %04o %s", perm, path),
	}
}

// Chown implements Renderer.
func (ur unixRenderer) Chown(path, owner, group string) []string {
	path = ur.Quote(path)
	return []string{
		fmt.Sprintf("chown %s:%s %s", owner, group, path),
	}
}

// Touch implements Renderer.
func (ur unixRenderer) Touch(path string, timestamp *time.Time) []string {
	path = ur.Quote(path)
	var opt string
	if timestamp != nil {
		opt = timestamp.Format("-t 200601021504.05 ")
	}
	return []string{
		fmt.Sprintf("touch %s%s", opt, path),
	}
}

// WriteFile implements Renderer.
func (ur unixRenderer) WriteFile(filename string, data []byte) []string {
	filename = ur.Quote(filename)
	return []string{
		// An alternate approach would be to use printf.
		fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", filename, data),
	}
}

func (unixRenderer) outFD(name string) (int, bool) {
	fd, ok := ResolveFD(name)
	if !ok || fd <= 0 {
		return -1, false
	}
	return fd, true
}

// RedirectFD implements OutputRenderer.
func (ur unixRenderer) RedirectFD(dst, src string) []string {
	dstFD, ok := ur.outFD(dst)
	if !ok {
		return nil
	}
	srcFD, ok := ur.outFD(src)
	if !ok {
		return nil
	}
	return []string{
		fmt.Sprintf("exec %d>&%d", srcFD, dstFD),
	}
}

// RedirectOutput implements OutputRenderer.
func (ur unixRenderer) RedirectOutput(filename string) []string {
	filename = ur.Quote(filename)

	return []string{
		"exec >> " + filename,
	}
}

// RedirectOutputReset implements OutputRenderer.
func (ur unixRenderer) RedirectOutputReset(filename string) []string {
	filename = ur.Quote(filename)

	return []string{
		"exec > " + filename,
	}
}

// ScriptFilename implements ScriptWriter.
func (ur *unixRenderer) ScriptFilename(name, dirname string) string {
	return ur.Join(dirname, name+".sh")
}

// ScriptPermissions implements ScriptWriter.
func (ur *unixRenderer) ScriptPermissions() os.FileMode {
	return 0755
}
