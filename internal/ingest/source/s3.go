package source

import (
	"context"
	"io"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Source struct {
	websiteID   string
	id          string
	endpoint    string
	region      string
	bucket      string
	prefix      string
	pattern     string
	accessKey   string
	secretKey   string
	compression string
	client      *s3.Client
}

func NewS3Source(websiteID, id, endpoint, region, bucket, prefix, pattern, accessKey, secretKey, compression string) (*S3Source, error) {
	if region == "" {
		region = "us-east-1"
	}

	cfgOptions := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	if accessKey != "" && secretKey != "" {
		cfgOptions = append(cfgOptions, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}

	if endpoint != "" {
		resolver := aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:               endpoint,
					HostnameImmutable: true,
				}, nil
			},
		)
		cfgOptions = append(cfgOptions, config.WithEndpointResolverWithOptions(resolver))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), cfgOptions...)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		if endpoint != "" {
			options.UsePathStyle = true
		}
	})

	return &S3Source{
		websiteID:   websiteID,
		id:          id,
		endpoint:    endpoint,
		region:      region,
		bucket:      bucket,
		prefix:      prefix,
		pattern:     pattern,
		accessKey:   accessKey,
		secretKey:   secretKey,
		compression: compression,
		client:      client,
	}, nil
}

func (s *S3Source) ID() string {
	return s.id
}

func (s *S3Source) Type() SourceType {
	return SourceS3
}

func (s *S3Source) ListTargets(ctx context.Context) ([]TargetRef, error) {
	var targets []TargetRef
	input := &s3.ListObjectsV2Input{
		Bucket: &s.bucket,
		Prefix: &s.prefix,
	}

	for {
		resp, err := s.client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, err
		}
		for _, obj := range resp.Contents {
			key := aws.ToString(obj.Key)
			if key == "" {
				continue
			}
			if s.pattern != "" && !matchS3Pattern(s.pattern, key) {
				continue
			}
			modTime := time.Time{}
			if obj.LastModified != nil {
				modTime = *obj.LastModified
			}
			meta := TargetMeta{
				Size:       aws.ToInt64(obj.Size),
				ModTime:    modTime,
				ETag:       strings.Trim(aws.ToString(obj.ETag), "\""),
				Compressed: isCompressedByName(key, s.compression),
			}
			targets = append(targets, TargetRef{
				WebsiteID: s.websiteID,
				SourceID:  s.id,
				Key:       key,
				Meta:      meta,
			})
		}

		if aws.ToBool(resp.IsTruncated) && resp.NextContinuationToken != nil {
			input.ContinuationToken = resp.NextContinuationToken
			continue
		}
		break
	}

	return targets, nil
}

func (s *S3Source) OpenRange(ctx context.Context, target TargetRef, start, end int64) (io.ReadCloser, error) {
	input := &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    aws.String(target.Key),
	}
	if start > 0 || end > 0 {
		input.Range = aws.String(buildRangeHeader(start, end))
	}

	resp, err := s.client.GetObject(ctx, input)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (s *S3Source) OpenStream(ctx context.Context, target TargetRef) (io.ReadCloser, error) {
	_ = ctx
	_ = target
	return nil, ErrStreamNotSupported
}

func (s *S3Source) Stat(ctx context.Context, target TargetRef) (TargetMeta, error) {
	input := &s3.HeadObjectInput{
		Bucket: &s.bucket,
		Key:    aws.String(target.Key),
	}
	resp, err := s.client.HeadObject(ctx, input)
	if err != nil {
		return TargetMeta{}, err
	}
	modTime := time.Time{}
	if resp.LastModified != nil {
		modTime = *resp.LastModified
	}
	return TargetMeta{
		Size:       aws.ToInt64(resp.ContentLength),
		ModTime:    modTime,
		ETag:       strings.Trim(aws.ToString(resp.ETag), "\""),
		Compressed: isCompressedByName(target.Key, s.compression),
	}, nil
}

func matchS3Pattern(pattern, key string) bool {
	if ok, _ := path.Match(pattern, key); ok {
		return true
	}
	basePattern := path.Base(pattern)
	baseKey := path.Base(key)
	if ok, _ := path.Match(basePattern, baseKey); ok {
		return true
	}
	return false
}
