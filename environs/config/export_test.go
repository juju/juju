package config

// ResetJujuHome empties the stored juju home variables
// to allow testing of error situations.
func ResetJujuHome() {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	jujuHome = ""
	jujuHomeOrig = ""
}
