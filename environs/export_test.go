package environs

var Setenv = setenv

func Providers() map[string]EnvironProvider {
	return providers
}
