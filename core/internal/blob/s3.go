package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// Config configures the S3-compatible client. It is the seven fields
// add-file-upload design names; core/internal/config.Config carries the
// VEKST_OBJECT_STORE_* environment names that populate it.
type Config struct {
	Endpoint    string // e.g. "http://minio:9000". Empty disables upload.
	Bucket      string
	Region      string // any value; S3-compatible stores still require one.
	AccessKeyID string
	SecretKey   string
	PathStyle   bool // true for MinIO: bucket.subdomain addressing needs DNS a real S3 has and MinIO does not.
}

// Configured reports whether upload can work at all, the same shape as
// config.Config.GoogleConfigured: an empty endpoint is a supported state, not
// an error, so a developer with no object store still gets a working stack.
func (c Config) Configured() bool { return c.Endpoint != "" }

// S3Store is the one implementation of ObjectStore. It talks to any store
// that speaks the S3 API with path-style addressing available -- verified
// against MinIO; a real S3 differs only in BaseEndpoint and PathStyle
// (design risk table).
type S3Store struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

// New builds a client bound to cfg. It performs no I/O -- the bucket is
// expected to already exist (created by the local overlay at start, task
// 7.1; provisioned out of band elsewhere), the same way core never creates
// its own database.
func New(cfg Config) *S3Store {
	client := s3.New(s3.Options{
		Region:       cfg.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretKey, ""),
		UsePathStyle: cfg.PathStyle,
		BaseEndpoint: aws.String(cfg.Endpoint),
	})
	return &S3Store{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  cfg.Bucket,
	}
}

// PresignPut signs Content-Length and Content-Type into the URL's
// SignedHeaders -- verified against MinIO (RELEASE.2025-09-07) that a PUT
// carrying either a different length or a different type than what was
// signed here comes back 403 SignatureDoesNotMatch, not 200. The headers map
// returned to the caller carries Content-Type only: Content-Length is in
// SignedHeaders too, but a browser sets that header itself from the request
// body and refuses to let script override it, so there is nothing for a
// caller to do with it except fail trying.
func (s *S3Store) PresignPut(ctx context.Context, key string, contentLength int64, contentType string, expires time.Duration) (string, map[string]string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		ContentLength: aws.Int64(contentLength),
		ContentType:   aws.String(contentType),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", nil, fmt.Errorf("blob: presigning put for %s: %w", key, err)
	}
	return req.URL, map[string]string{"Content-Type": contentType}, nil
}

// Head reports whether key exists, without transferring it.
func (s *S3Store) Head(ctx context.Context, key string) error {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if isNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("blob: head %s: %w", key, err)
	}
	return nil
}

// Get opens key for reading. The caller closes it.
func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("blob: get %s: %w", key, err)
	}
	return out.Body, nil
}

// Delete removes key. Deleting a key that does not exist is not an error --
// verified against MinIO that DeleteObject on a missing key returns no error
// at all, which is the behaviour ObjectStore's doc comment promises.
func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("blob: delete %s: %w", key, err)
	}
	return nil
}

// isNotFound absorbs the disagreement between S3 operations about what a
// missing object is called: HeadObject answers NotFound, GetObject answers
// NoSuchKey -- both verified against MinIO. Nothing above this file should
// need to know either name.
func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "NotFound", "NoSuchKey":
		return true
	default:
		return false
	}
}
