package identity

import (
	"context"
	"testing"
)

// ContextForTest returns ctx with u planted the same way Middleware plants a
// resolved session, for a test that needs an authenticated context without
// running a real OAuth flow.
//
// It takes a testing.TB it does not use, the same reasoning
// core/internal/db.OrgIDForTest gives for its own unused parameter:
// production code cannot call this without importing "testing", which is
// obvious in review and greppable in CI. Without it, planting a User in a
// context would be reachable from anywhere, and ctxKey's whole point --
// that only Middleware can put a person in a request -- would be a comment,
// not a guarantee.
//
// This file is deliberately not _test.go: tests in *other* packages
// (core/internal/server's handler tests) need it, and a _test.go file is not
// importable.
func ContextForTest(tb testing.TB, ctx context.Context, u User) context.Context {
	tb.Helper()
	return context.WithValue(ctx, ctxKey{}, u)
}
