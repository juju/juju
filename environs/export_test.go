package environs

func Setenv(env []string, val string) []string {
	return setenv(env, val)
}

func Providers() map[string]EnvironProvider {
	return providers
}
