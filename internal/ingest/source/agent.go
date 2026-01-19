package source

import (
	"context"
	"io"
)

type AgentSource struct {
	websiteID string
	id        string
}

func NewAgentSource(websiteID, id string) *AgentSource {
	return &AgentSource{
		websiteID: websiteID,
		id:        id,
	}
}

func (s *AgentSource) ID() string {
	return s.id
}

func (s *AgentSource) Type() SourceType {
	return SourceAgent
}

func (s *AgentSource) ListTargets(ctx context.Context) ([]TargetRef, error) {
	_ = ctx
	return nil, nil
}

func (s *AgentSource) OpenRange(ctx context.Context, target TargetRef, start, end int64) (io.ReadCloser, error) {
	_ = ctx
	_ = target
	_ = start
	_ = end
	return nil, ErrRangeNotSupported
}

func (s *AgentSource) OpenStream(ctx context.Context, target TargetRef) (io.ReadCloser, error) {
	_ = ctx
	_ = target
	return nil, ErrStreamNotSupported
}

func (s *AgentSource) Stat(ctx context.Context, target TargetRef) (TargetMeta, error) {
	_ = ctx
	_ = target
	return TargetMeta{}, ErrStreamNotSupported
}
