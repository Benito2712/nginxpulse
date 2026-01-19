package source

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SFTPSource struct {
	websiteID   string
	id          string
	host        string
	port        int
	user        string
	keyFile     string
	password    string
	path        string
	pattern     string
	compression string
}

func NewSFTPSource(websiteID, id, host string, port int, user, keyFile, password, pathValue, pattern, compression string) *SFTPSource {
	return &SFTPSource{
		websiteID:   websiteID,
		id:          id,
		host:        host,
		port:        port,
		user:        user,
		keyFile:     keyFile,
		password:    password,
		path:        pathValue,
		pattern:     pattern,
		compression: compression,
	}
}

func (s *SFTPSource) ID() string {
	return s.id
}

func (s *SFTPSource) Type() SourceType {
	return SourceSFTP
}

func (s *SFTPSource) ListTargets(ctx context.Context) ([]TargetRef, error) {
	client, sshClient, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	defer sshClient.Close()

	var targets []TargetRef
	if s.pattern != "" {
		dir := path.Dir(s.pattern)
		base := path.Base(s.pattern)
		entries, err := client.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if ok, _ := path.Match(base, entry.Name()); !ok {
				continue
			}
			fullPath := path.Join(dir, entry.Name())
			targets = append(targets, TargetRef{
				WebsiteID: s.websiteID,
				SourceID:  s.id,
				Key:       fullPath,
				Meta: TargetMeta{
					Size:       entry.Size(),
					ModTime:    entry.ModTime(),
					Compressed: isCompressedByName(fullPath, s.compression),
				},
			})
		}
		return targets, nil
	}

	if s.path == "" {
		return nil, nil
	}
	info, err := client.Stat(s.path)
	if err != nil {
		return nil, err
	}
	targets = append(targets, TargetRef{
		WebsiteID: s.websiteID,
		SourceID:  s.id,
		Key:       s.path,
		Meta: TargetMeta{
			Size:       info.Size(),
			ModTime:    info.ModTime(),
			Compressed: isCompressedByName(s.path, s.compression),
		},
	})
	return targets, nil
}

func (s *SFTPSource) OpenRange(ctx context.Context, target TargetRef, start, end int64) (io.ReadCloser, error) {
	client, sshClient, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}

	file, err := client.Open(target.Key)
	if err != nil {
		client.Close()
		sshClient.Close()
		return nil, err
	}

	if start > 0 {
		if _, err := file.Seek(start, io.SeekStart); err != nil {
			file.Close()
			client.Close()
			sshClient.Close()
			return nil, err
		}
	}

	var reader io.Reader = file
	if end > 0 && end > start {
		reader = io.NewSectionReader(file, start, end-start)
	}

	closer := multiCloser{file, client, sshClient}
	return newReadCloser(reader, closer), nil
}

func (s *SFTPSource) OpenStream(ctx context.Context, target TargetRef) (io.ReadCloser, error) {
	_ = ctx
	_ = target
	return nil, ErrStreamNotSupported
}

func (s *SFTPSource) Stat(ctx context.Context, target TargetRef) (TargetMeta, error) {
	client, sshClient, err := s.connect(ctx)
	if err != nil {
		return TargetMeta{}, err
	}
	defer client.Close()
	defer sshClient.Close()

	info, err := client.Stat(target.Key)
	if err != nil {
		return TargetMeta{}, err
	}
	return TargetMeta{
		Size:       info.Size(),
		ModTime:    info.ModTime(),
		Compressed: isCompressedByName(target.Key, s.compression),
	}, nil
}

func (s *SFTPSource) connect(ctx context.Context) (*sftp.Client, *ssh.Client, error) {
	if s.port == 0 {
		s.port = 22
	}

	auths := []ssh.AuthMethod{}
	if strings.TrimSpace(s.password) != "" {
		auths = append(auths, ssh.Password(s.password))
	}
	if strings.TrimSpace(s.keyFile) != "" {
		key, err := os.ReadFile(s.keyFile)
		if err != nil {
			return nil, nil, err
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, nil, err
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}
	if len(auths) == 0 {
		return nil, nil, fmt.Errorf("sftp auth missing")
	}

	cfg := &ssh.ClientConfig{
		User:            s.user,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	addr := net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port))
	sshClient, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, nil, err
	}

	client, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, nil, err
	}

	return client, sshClient, nil
}

type multiCloser []io.Closer

func (m multiCloser) Close() error {
	for _, closer := range m {
		if closer != nil {
			_ = closer.Close()
		}
	}
	return nil
}
