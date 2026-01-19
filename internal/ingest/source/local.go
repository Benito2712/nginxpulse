package source

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

type LocalSource struct {
	websiteID   string
	id          string
	path        string
	pattern     string
	compression string
}

func NewLocalSource(websiteID, id, path, pattern, compression string) *LocalSource {
	return &LocalSource{
		websiteID:   websiteID,
		id:          id,
		path:        path,
		pattern:     pattern,
		compression: compression,
	}
}

func (s *LocalSource) ID() string {
	return s.id
}

func (s *LocalSource) Type() SourceType {
	return SourceLocal
}

func (s *LocalSource) ListTargets(ctx context.Context) ([]TargetRef, error) {
	_ = ctx
	var paths []string
	if s.pattern != "" {
		matches, err := filepath.Glob(s.pattern)
		if err != nil {
			return nil, err
		}
		paths = matches
	} else if s.path != "" {
		paths = []string{s.path}
	}

	targets := make([]TargetRef, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		targets = append(targets, TargetRef{
			WebsiteID: s.websiteID,
			SourceID:  s.id,
			Key:       path,
			Meta: TargetMeta{
				Size:       info.Size(),
				ModTime:    info.ModTime(),
				Compressed: isCompressedByName(path, s.compression),
			},
		})
	}
	return targets, nil
}

func (s *LocalSource) OpenRange(ctx context.Context, target TargetRef, start, end int64) (io.ReadCloser, error) {
	_ = ctx
	file, err := os.Open(target.Key)
	if err != nil {
		return nil, err
	}

	if start > 0 {
		if _, err := file.Seek(start, 0); err != nil {
			file.Close()
			return nil, err
		}
	}

	if end > 0 && end > start {
		section := io.NewSectionReader(file, start, end-start)
		return newReadCloser(section, file), nil
	}

	return file, nil
}

func (s *LocalSource) OpenStream(ctx context.Context, target TargetRef) (io.ReadCloser, error) {
	_ = ctx
	_ = target
	return nil, ErrStreamNotSupported
}

func (s *LocalSource) Stat(ctx context.Context, target TargetRef) (TargetMeta, error) {
	_ = ctx
	info, err := os.Stat(target.Key)
	if err != nil {
		return TargetMeta{}, err
	}
	return TargetMeta{
		Size:       info.Size(),
		ModTime:    info.ModTime(),
		Compressed: isCompressedByName(target.Key, s.compression),
	}, nil
}
