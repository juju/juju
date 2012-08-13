package environs

var ZkPortSuffix = zkPortSuffix

func Setenv(env []string, val string) []string {
	return setenv(env, val)
}
