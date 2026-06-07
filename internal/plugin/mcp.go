package plugin

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vksir/vkiss-lib/pkg/util/errutil"
)

type Mcp struct {
	client  *mcp.Client
	session *mcp.ClientSession
}

func NewMcp() *Mcp {
	return &Mcp{client: mcp.NewClient(&mcp.Implementation{}, nil)}
}

func (m *Mcp) Connect(ctx context.Context, transport mcp.Transport) error {
	session, err := m.client.Connect(ctx, transport, nil)
	if err != nil {
		return errutil.Wrap(err)
	}
	m.session = session
	return nil
}

func (m *Mcp) GetTools(ctx context.Context) ([]*mcp.Tool, error) {
	resp, err := m.session.ListTools(ctx, nil)
	if err != nil {
		return nil, errutil.Wrap(err)
	}
	return resp.Tools, nil
}
