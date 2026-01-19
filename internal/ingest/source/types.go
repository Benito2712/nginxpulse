package source

import (
	"context"
	"errors"
	"io"
	"time"
)

type SourceType string

const (
	SourceLocal SourceType = "local"
	SourceSFTP  SourceType = "sftp"
	SourceHTTP  SourceType = "http"
	SourceS3    SourceType = "s3"
	SourceAgent SourceType = "agent"
)

type RangePolicy string

const (
	RangeAuto  RangePolicy = "auto"
	RangeForce RangePolicy = "range"
	RangeFull  RangePolicy = "full"
)

var (
	ErrRangeNotSupported  = errors.New("range not supported")
	ErrStreamNotSupported = errors.New("stream not supported")
)

type TargetRef struct {
	WebsiteID string
	SourceID  string
	Key       string
	Meta      TargetMeta
}

type TargetMeta struct {
	Size       int64
	ModTime    time.Time
	ETag       string
	Compressed bool
}

type LogSource interface {
	ID() string
	Type() SourceType
	ListTargets(ctx context.Context) ([]TargetRef, error)
	OpenRange(ctx context.Context, target TargetRef, start, end int64) (io.ReadCloser, error)
	OpenStream(ctx context.Context, target TargetRef) (io.ReadCloser, error)
	Stat(ctx context.Context, target TargetRef) (TargetMeta, error)
}
