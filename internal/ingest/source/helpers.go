package source

import (
	"io"
	"strings"
)

func normalizeCompression(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isCompressedByName(name string, compression string) bool {
	switch normalizeCompression(compression) {
	case "gz":
		return true
	case "none":
		return false
	default:
		return strings.HasSuffix(strings.ToLower(name), ".gz")
	}
}

func normalizeRangePolicy(value string) RangePolicy {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(RangeForce):
		return RangeForce
	case string(RangeFull):
		return RangeFull
	default:
		return RangeAuto
	}
}

type readCloser struct {
	reader io.Reader
	closer io.Closer
}

func newReadCloser(reader io.Reader, closer io.Closer) io.ReadCloser {
	return &readCloser{reader: reader, closer: closer}
}

func (r *readCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *readCloser) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}
