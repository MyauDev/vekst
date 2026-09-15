// Package blob is the boundary between core and the object store's
// specifics. ARCHITECTURE.md §3 puts an S3-compatible store beside Postgres
// and says that if it writes to either, it is core -- and add-file-upload
// design D5 puts MinIO in the local overlay for the same reason and by the
// same precedent as Postgres.
//
// Everything above ObjectStore -- core/internal/ingest and its jobs --
// depends on this interface and nothing narrower, so an S3-compatible client
// library is imported in exactly one file in this package (s3.go).
package blob

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound is returned by Head and Get when no object exists at the given
// key. It is one error regardless of which store reports it and however that
// store spells its own "not found" -- S3 and MinIO already disagree with each
// other about the code (NotFound for a HEAD, NoSuchKey for a GET), and an
// implementation of ObjectStore is where that disagreement is absorbed rather
// than passed up.
var ErrNotFound = errors.New("blob: object not found")

// ObjectStore is what core/internal/ingest is written against. There is one
// implementation, in s3.go, but the boundary exists so that MinIO drifting
// from whatever the production store turns out to be is a change to that one
// file (design risk table).
type ObjectStore interface {
	// PresignPut signs a PUT to key, valid for expires, that the store
	// accepts only from a request carrying exactly this Content-Length and
	// Content-Type (add-file-upload design D1/D3). expires is rounded down
	// to whole seconds -- SigV4's X-Amz-Expires is an integer, verified
	// against MinIO that anything under one second signs a URL already
	// expired by the time a client can use it. UploadURLLifetime's default
	// is fifteen minutes, so this only matters to a test that shortens it.
	// It returns the URL and the
	// headers the caller must send verbatim -- deliberately never including
	// Content-Length, which a browser computes from the request body and
	// refuses to let script set (it is on the Fetch spec's forbidden-header
	// list). That omission costs nothing: the signature still binds the
	// upload to the length CreateImportBatch signed, and the length a browser
	// sends is whatever its own file object actually is, which is the number
	// this was signed against in the first place.
	PresignPut(ctx context.Context, key string, contentLength int64, contentType string, expires time.Duration) (url string, headers map[string]string, err error)

	// Head reports whether an object exists at key, without transferring it.
	// ErrNotFound if it does not -- the measurement job's first check, before
	// it opens a stream against an upload that never arrived.
	Head(ctx context.Context, key string) error

	// Get opens the object for reading. The caller closes it. ErrNotFound if
	// key does not exist.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object at key. Deleting a key that does not exist is
	// not an error: the expiry job calls this for a batch whose upload may or
	// may not have arrived, and "already gone" and "never existed" are the
	// same outcome from here.
	Delete(ctx context.Context, key string) error
}
