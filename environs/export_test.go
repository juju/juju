package environs

func Setenv(env []string, val string) []string {
	return setenv(env, val)
}
