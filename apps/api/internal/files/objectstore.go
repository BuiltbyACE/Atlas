package files

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// ObjectStore is the storage port (ADR-001 repository boundary). The concrete
// adapter talks to any S3-compatible endpoint: Cloudflare R2 in staging and
// production, SeaweedFS locally (§6.5).
type ObjectStore interface {
	// PresignPut returns a URL the client PUTs bytes to, plus the headers the
	// signature covers (the client must send them verbatim).
	PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, map[string]string, error)
	// PresignGet returns a short-lived download URL. downloadName sets the
	// response's content-disposition filename when non-empty.
	PresignGet(ctx context.Context, key, downloadName string, ttl time.Duration) (string, error)
	// Stat reports the object's size and content type, or ErrObjectMissing when
	// there is no such object.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	// Delete removes an object. Archiving a file only marks its metadata
	// deleted (§6.6 keeps business records); the bytes are purged later by a
	// retention job, which is what calls this.
	Delete(ctx context.Context, key string) error
}

// S3Config is the bootstrap configuration for an S3-compatible bucket.
type S3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	// PathStyle forces path-style addressing, which SeaweedFS requires. R2's
	// account endpoint accepts either, so the safe default is true.
	PathStyle bool
}

// S3Store implements ObjectStore over an S3-compatible API.
type S3Store struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

// NewS3Store builds the adapter. It performs no network call, so the API starts
// — and can issue pre-signed URLs — even when storage is briefly unreachable.
func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("files: an S3 bucket is required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "auto"
	}

	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if cfg.AccessKey != "" || cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("files: load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.PathStyle
	})
	return &S3Store{client: client, presign: s3.NewPresignClient(client), bucket: cfg.Bucket}, nil
}

// PresignPut signs a PUT for a single object. The signature covers Content-Type
// when one is given, so the client must send the header back exactly.
func (s *S3Store) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, map[string]string, error) {
	in := &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	headers := map[string]string{}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
		headers["Content-Type"] = contentType
	}
	req, err := s.presign.PresignPutObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", nil, fmt.Errorf("files: presign put %s: %w", key, err)
	}
	return req.URL, headers, nil
}

func (s *S3Store) PresignGet(ctx context.Context, key, downloadName string, ttl time.Duration) (string, error) {
	in := &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	if downloadName != "" {
		in.ResponseContentDisposition = aws.String(`attachment; filename="` + safeName(downloadName) + `"`)
	}
	req, err := s.presign.PresignGetObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("files: presign get %s: %w", key, err)
	}
	return req.URL, nil
}

func (s *S3Store) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	if err != nil {
		if isObjectNotFound(err) {
			return ObjectInfo{}, ErrObjectMissing
		}
		return ObjectInfo{}, fmt.Errorf("files: head %s: %w", key, err)
	}
	info := ObjectInfo{ETag: strings.Trim(aws.ToString(out.ETag), `"`)}
	if out.ContentLength != nil {
		info.SizeBytes = *out.ContentLength
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	return info, nil
}

// Delete removes an object. Archiving a file keeps its metadata; this is the
// operation a retention job uses to purge the bytes.
func (s *S3Store) Delete(ctx context.Context, key string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	}); err != nil {
		return fmt.Errorf("files: delete %s: %w", key, err)
	}
	return nil
}

// isObjectNotFound recognises the S3/S3-compatible "missing object" replies.
func isObjectNotFound(err error) bool {
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket":
			return true
		}
	}
	return false
}
