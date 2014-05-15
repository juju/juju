package azure

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"

	"github.com/juju/errors"
)

// tempCertFile is a temporary file containing an x509 certificate.
// It's possible to pass a certificate to libcurl in-memory, but much more
// complicated.  We went with this hack for now.  Call newTempCertFile to
// store a certificate in a temporary file, and once you're done with the
// file, invoke its Delete method to clean it up.
type tempCertFile struct {
	tempDir  string
	filename string
}

// Path returns the full absolute path for the temporary certificate file.
func (certFile *tempCertFile) Path() string {
	return path.Join(certFile.tempDir, certFile.filename)
}

// Delete cleans up a tempCertFile.  You must call this after use, or you'll
// leave not just garbage but security-sensitive garbage.
// This method is idempotent.  If called after it's already been run, it
// does nothing.
func (certFile *tempCertFile) Delete() {
	if certFile.tempDir == "" {
		// Either it wasn't constructed, or it's been deleted already.
		return
	}
	err := os.RemoveAll(certFile.tempDir)
	if err != nil {
		panic(err)
	}
	// We no longer own a file that needs cleaning up.
	certFile.filename = ""
	certFile.tempDir = ""
}

// newTempCertFile stores the given x509 certificate in a temporary file,
// which only the current user will be allowed to access.
// You *must* clean up the file after use, by calling its Delete method.
func newTempCertFile(data []byte) (certFile *tempCertFile, err error) {
	// Add context to any error we may return.
	defer errors.Maskf(&err, "failed while writing temporary certificate file")

	// Access permissions for these temporary files:
	const (
		// Owner can read/write temporary files.  Not backed up.
		fileMode = 0600 | os.ModeTemporary | os.ModeExclusive
		// Temporary dirs are like files, but owner also has "x"
		// permission.
		dirMode = fileMode | 0100
	)

	certFile = &tempCertFile{}

	// We'll randomize the file's name, so that even someone with access
	// to the temporary directory (perhaps a group member sneaking in
	// just before we close access to the directory) won't be able to
	// guess its name and inject their own file.
	certFile.filename = fmt.Sprintf("x509-%d.cert", rand.Int31())

	// To guarantee that nobody else will be able to access the file, even
	// by predicting or guessing its name, we create the file in its own
	// private directory.
	certFile.tempDir, err = ioutil.TempDir("", "juju-azure")
	if err != nil {
		return nil, err
	}
	err = os.Chmod(certFile.tempDir, dirMode)
	if err != nil {
		return nil, err
	}

	// Now, at last, write the file.  WriteFile could have done most of
	// the work on its own, but it doesn't guarantee that nobody creates
	// a file of the same name first.  When that happens, you get a file
	// but not with the requested permissions.
	err = ioutil.WriteFile(certFile.Path(), data, fileMode)
	if err != nil {
		os.RemoveAll(certFile.tempDir)
		return nil, err
	}

	return certFile, nil
}
