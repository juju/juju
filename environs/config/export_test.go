package config

// ResetJujuHome empties the stored juju home variable
// to allow testing of error situations.
func ResetJujuHome() {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	jujuHome = ""
}
