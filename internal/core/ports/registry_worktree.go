package ports

type StaticWorktreeProviderRegistry struct {
	providers map[string]WorktreeProvider
}

func NewStaticWorktreeProviderRegistry(providers map[string]WorktreeProvider) *StaticWorktreeProviderRegistry {
	copied := make(map[string]WorktreeProvider, len(providers))
	for name, provider := range providers {
		copied[name] = provider
	}
	return &StaticWorktreeProviderRegistry{providers: copied}
}

func (r *StaticWorktreeProviderRegistry) Get(name string) (WorktreeProvider, bool) {
	provider, ok := r.providers[name]
	return provider, ok
}

func (r *StaticWorktreeProviderRegistry) HasProvider(name string) bool {
	_, ok := r.providers[name]
	return ok
}
