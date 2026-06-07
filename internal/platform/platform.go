package platform

import (
	"context"
	"stella/entity"
)

type Platform interface {
	Start(ctx context.Context) error
	Chan() <-chan entity.Event
	Send(ctx context.Context, evt entity.Event) error
}
