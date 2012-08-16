package testing
import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"time"
)

// File represents a file to be archived.
type File struct {
	Header   tar.Header
	Contents string
}

var modes = map[os.FileMode] byte {
	os.ModeDir: tar.TypeDir,
	os.ModeSymlink: tar.TypeSymlink,
	0: tar.TypeReg,
}

// NewFile returns a new File instance with the given file
// mode and contents.
func NewFile(name string, mode os.FileMode, contents string) *File {
	ftype := modes[mode & os.ModeType]
	if ftype == 0 {
		panic(fmt.Errorf("unexpected mode %v", mode))
	}
	return &File{
		Header: tar.Header{
			Typeflag:   ftype,
			Name:       name,
			Size:       int64(len(contents)),
			Mode:       int64(mode & 0777),
			ModTime:    time.Now(),
			AccessTime: time.Now(),
			ChangeTime: time.Now(),
			Uname:      "ubuntu",
			Gname:      "ubuntu",
		},
		Contents: contents,
	}
}

// Archive returns the given files in gzipped tar-archive format.
func Archive(files ...*File) []byte {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tarw := tar.NewWriter(gzw)

	for _, f := range files {
		err := tarw.WriteHeader(&f.Header)
		if err != nil {
			panic(err)
		}
		_, err = tarw.Write([]byte(f.Contents))
		if err != nil {
			panic(err)
		}
	}
	err := tarw.Close()
	if err != nil {
		panic(err)
	}
	err = gzw.Close()
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}
