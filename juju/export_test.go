package juju

func SetJujuRoot(new string) (old string) {
	old = jujuRoot
	jujuRoot = new
	return
}
