package blob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// testStore skips when no live object store is configured: PresignPut's
// whole point is what a real store's signature verification accepts and
// rejects, which no mock can stand in for. CI's "database" job does not
// start one; this test suite runs wherever OBJECT_STORE_ENDPOINT is set --
// e.g. against the MinIO the local overlay runs (task 7.1).
func testStore(t *testing.T) *S3Store {
	t.Helper()
	endpoint := os.Getenv("OBJECT_STORE_ENDPOINT")
	if endpoint == "" {
		t.Skip("OBJECT_STORE_ENDPOINT not set; skipping a test that needs a live S3-compatible store")
	}
	bucket := os.Getenv("OBJECT_STORE_BUCKET")
	if bucket == "" {
		bucket = "vekst"
	}
	return New(Config{
		Endpoint:    endpoint,
		Bucket:      bucket,
		Region:      "us-east-1",
		AccessKeyID: envOr("OBJECT_STORE_ACCESS_KEY_ID", "minioadmin"),
		SecretKey:   envOr("OBJECT_STORE_SECRET_KEY", "minioadmin"),
		PathStyle:   true,
	})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func putViaPresignedURL(t *testing.T, url string, headers map[string]string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("building PUT request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	return resp
}

func TestPresignPutAcceptsAMatchingUpload(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	key := "test/" + t.Name()

	body := []byte("hello, vekst")
	url, headers, err := s.PresignPut(ctx, key, int64(len(body)), "text/csv", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}

	resp := putViaPresignedURL(t, url, headers, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT status = %d, want 200: %s", resp.StatusCode, b)
	}

	if err := s.Head(ctx, key); err != nil {
		t.Fatalf("Head after a matching upload: %v", err)
	}
	rc, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after a matching upload: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading object body: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("object body = %q, want %q", got, body)
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// The client's declared size and type are signing conditions, not facts
// (design D2) -- but only because the store itself refuses a request that
// does not match what was signed. This is that refusal, exercised end to end
// rather than assumed.
func TestPresignPutRejectsAMismatchedUpload(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	t.Run("wrong length", func(t *testing.T) {
		key := "test/" + t.Name()
		url, headers, err := s.PresignPut(ctx, key, 5, "text/csv", 5*time.Minute)
		if err != nil {
			t.Fatalf("PresignPut: %v", err)
		}
		resp := putViaPresignedURL(t, url, headers, []byte("this body is longer than five bytes"))
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Error("store accepted a body longer than the signed Content-Length")
		}
		if err := s.Head(ctx, key); !errors.Is(err, ErrNotFound) {
			t.Errorf("Head after a rejected upload = %v, want ErrNotFound", err)
		}
	})

	t.Run("wrong content type", func(t *testing.T) {
		key := "test/" + t.Name()
		body := []byte("same five")
		url, headers, err := s.PresignPut(ctx, key, int64(len(body)), "text/csv", 5*time.Minute)
		if err != nil {
			t.Fatalf("PresignPut: %v", err)
		}
		headers["Content-Type"] = "application/json"
		resp := putViaPresignedURL(t, url, headers, body)
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Error("store accepted a Content-Type different from what was signed")
		}
	})
}

func TestPresignPutExpires(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	key := "test/" + t.Name()

	// A negative-looking expiry is rejected by the signer itself on some
	// implementations; a very short one that has already elapsed by the time
	// the request lands is the portable way to exercise the same rejection.
	url, headers, err := s.PresignPut(ctx, key, 4, "text/plain", time.Nanosecond)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	resp := putViaPresignedURL(t, url, headers, []byte("late"))
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("store accepted a PUT against an expired presigned URL")
	}
}

func TestHeadAndGetReportNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	key := "test/does-not-exist/" + t.Name()

	if err := s.Head(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Errorf("Head(%q) = %v, want ErrNotFound", key, err)
	}
	if _, err := s.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(%q) = %v, want ErrNotFound", key, err)
	}
}

// The expiry job deletes a batch's object whether or not the upload ever
// arrived (design D2/D5's abandon path); a missing key must not be an error.
func TestDeleteOfMissingKeyIsNotAnError(t *testing.T) {
	s := testStore(t)
	if err := s.Delete(context.Background(), "test/never-existed/"+t.Name()); err != nil {
		t.Errorf("Delete of a missing key = %v, want nil", err)
	}
}
