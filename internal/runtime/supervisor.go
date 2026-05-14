package runtime

import (
	"context"

	"github.com/thejerf/suture/v4"
)

type ServiceFunc func(context.Context) error

func (f ServiceFunc) Serve(ctx context.Context) error {
	if err := f(ctx); err != nil {
		return err
	}
	return suture.ErrDoNotRestart
}

func Run(ctx context.Context, name string, services ...suture.Service) error {
	supervisor := suture.NewSimple(name)
	for _, service := range services {
		supervisor.Add(service)
	}
	return supervisor.Serve(ctx)
}
