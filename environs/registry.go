package environs

var globalRegistry = NewRegistry()

func GlobalRegistry() *Registry {
	return globalRegistry
}

// GlobalProviderRegistry returns the global provider registry.
func GlobalProviderRegistry() *ProviderRegistry {
	return GlobalRegistry().Providers()
}

func GlobalImageSourceRegistry() *ImageSourceRegistry {
	return GlobalRegistry().ImageSources()
}

// NewRegistry returns a new registry for providers
// and simplestreams image sources.
func NewRegistry() *Registry {
	return &Registry{
		providers: NewProviderRegistry(),
		images:    NewImageSourceRegistry(),
	}
}

// Registry holds registered providers and image sources.
type Registry struct {
	providers *ProviderRegistry
	images    *ImageSourceRegistry
}

// Providers returns the provider registry.
func (r *Registry) Providers() *ProviderRegistry {
	return r.providers
}

// ImageSources returns the image source registry.
func (r *Registry) ImageSources() *ImageSourceRegistry {
	return r.images
}
