package client

import "context"

// Composite combines a DaemonClient and a LocalClient into a single Client.
type Composite struct {
	*HTTPDaemonClient
	*LocalClient
}

// NewComposite creates a Composite client.
func NewComposite(daemon *HTTPDaemonClient, local *LocalClient) *Composite {
	return &Composite{HTTPDaemonClient: daemon, LocalClient: local}
}

// ListLocalWorkflows delegates to LocalClient.
func (c *Composite) ListLocalWorkflows(ctx context.Context) ([]LocalWorkflow, error) {
	return c.LocalClient.ListLocalWorkflows(ctx)
}

// ValidateWorkflow delegates to LocalClient.
func (c *Composite) ValidateWorkflow(ctx context.Context, ref string) error {
	return c.LocalClient.ValidateWorkflow(ctx, ref)
}

// GraphWorkflow delegates to LocalClient.
func (c *Composite) GraphWorkflow(ctx context.Context, ref string) (string, error) {
	return c.LocalClient.GraphWorkflow(ctx, ref)
}

// DryRunWorkflow delegates to LocalClient.
func (c *Composite) DryRunWorkflow(ctx context.Context, ref string, inputs, vars map[string]any) (string, error) {
	return c.LocalClient.DryRunWorkflow(ctx, ref, inputs, vars)
}
