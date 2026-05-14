package ports

type StaticAgentProviderRegistry struct {
	providers map[string]AgentProvider
}

func NewStaticAgentProviderRegistry(providers map[string]AgentProvider) *StaticAgentProviderRegistry {
	copied := make(map[string]AgentProvider, len(providers))
	for name, provider := range providers {
		copied[name] = provider
	}
	return &StaticAgentProviderRegistry{providers: copied}
}

func (r *StaticAgentProviderRegistry) Get(name string) (AgentProvider, bool) {
	provider, ok := r.providers[name]
	return provider, ok
}

func (r *StaticAgentProviderRegistry) HasProvider(name string) bool {
	_, ok := r.providers[name]
	return ok
}
