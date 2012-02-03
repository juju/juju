package ec2

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func isNotFoundError(e error) bool {
	if e == os.ENOENT {
		return true
	}
	if e, ok := e.(*os.PathError); ok {
		return e.Err == os.ENOENT
	}
	return false
}

func expandFileName(f string) string {
	// TODO expansion of other user's home directories.
	// Q what characters are valid in a user name?
	if strings.HasPrefix(f, "~"+string(filepath.Separator)) {
		return os.Getenv("HOME") + f[1:]
	}
	return f
}

// authorizedKeys finds an authorized_keys file and returns its contents
// (see sshd(8) for a description of the format). If path is not empty,
// it names the file to use; otherwise the user's .ssh directory will be
// searched.  Home directory expansion will be performed on the path if it
// starts with a ~; if the expanded path is relative, it will be
// interpreted relative to $HOME/.ssh.
func authorizedKeys(path string) (string, error) {
	var files []string
	if path == "" {
		files = []string{"id_dsa.pub", "id_rsa.pub", "identity.pub"}
	} else {
		files = []string{path}
	}
	var finalError error
	var keys []byte
	for _, f := range files {
		f = expandFileName(f)
		if !filepath.IsAbs(f) {
			f = filepath.Join(expandFileName(filepath.Join("~", ".ssh")), f)
		}
		data, err := ioutil.ReadFile(f)
		if err != nil {
			if finalError == nil && !isNotFoundError(err) {
				finalError = err
			}
			continue
		}
		keys = append(keys, data...)
		// ensure that a file without a final newline
		// is properly separated from any others.
		if len(data) > 0 && data[len(data)-1] != '\n' {
			keys = append(keys, '\n')
		}
	}
	if len(keys) == 0 {
		if finalError == nil {
			finalError = fmt.Errorf("no keys found")
		}
		return "", finalError
	}
	return string(keys), nil
}

