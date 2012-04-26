package juju
import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// tarHeader returns a file header given the 
func tarHeader(i os.FileInfo) *tar.Header {
	return &tar.Header{
		Typeflag: tar.TypeReg,
		Name: i.Name(),
		Size: i.Size(),
		Mode: int64(i.Mode() & 0777),
		ModTime: i.ModTime(),
		AccessTime: i.ModTime(),
		ChangeTime: i.ModTime(),
		Uname: "ubuntu",
		Gname: "ubuntu",
	}
}

// isExecutable returns whether the given info
// represents a regular file executable by (at least) the user.
func isExecutable(i os.FileInfo) bool {
	return i.Mode() & (0100 | os.ModeType) == 0100
}

// archive writes the executable files found in the given
// directory in gzipped tar format to w.
func archive(w io.Writer, dir string) error {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	tarw := tar.NewWriter(gzip.NewWriter(w))
	defer tarw.Close()
	for _, ent := range entries {
		if !isExecutable(ent) {
			continue
		}
		h := tarHeader(ent)
		// ignore local umask
		h.Mode = 0755
		err := tarw.WriteHeader(h)
		if err != nil {
			return err
		}
		if err := copyFile(tarw, filepath.Join(dir, ent.Name())); err != nil {
			return err
		}
	}
	return tarw.Flush()
}

func copyFile(w io.Writer, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}
