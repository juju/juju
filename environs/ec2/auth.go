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

// authorizedKeys implements the standard juju behaviour for finding
// authorized_keys. It returns a set of keys in in authorized_keys format
// (see sshd(8) for a description).  If keys is non-empty, it returns that.
// If path is non-empty, it names the file to use; otherwise the user's .ssh
// directory will be searched.  Home directory expansion will be performed
// on the path if it starts with a ~; if the expanded path is relative,
// it will be interpreted relative to $HOME/.ssh.
func authorizedKeys(keys, path string) (string, error) {
	if keys != "" {
		return keys, nil
	}
	var files []string
	if path == "" {
		files = []string{"id_dsa.pub", "id_rsa.pub", "identity.pub"}
	} else {
		files = []string{path}
	}
	var firstError error
	var keyData []byte
	for _, f := range files {
		f = expandFileName(f)
		if !filepath.IsAbs(f) {
			f = filepath.Join(expandFileName(filepath.Join("~", ".ssh")), f)
		}
		data, err := ioutil.ReadFile(f)
		if err != nil {
			if firstError == nil && !isNotFoundError(err) {
				firstError = err
			}
			continue
		}
		keyData = append(keyData, data...)
		// ensure that a file without a final newline
		// is properly separated from any others.
		if len(data) > 0 && data[len(data)-1] != '\n' {
			keyData = append(keyData, '\n')
		}
	}
	if len(keyData) == 0 {
		if firstError == nil {
			firstError = fmt.Errorf("no keys found")
		}
		return "", firstError
	}
	return string(keyData), nil
}
