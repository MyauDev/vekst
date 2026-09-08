package identity

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"
)

// ctxKey is unexported so nothing outside this package can plant a user in a
// context. The only way a request carries an authenticated person is for
// Middleware to have resolved a real session for it.
type ctxKey struct{}

// FromContext returns the authenticated person, if any. The boolean is false
// for an anonymous request -- no cookie, an unknown one, an expired session or
// a revoked one, all of which are indistinguishable by design.
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// Middleware resolves the session cookie and puts the person in the request
// context. It never rejects: /healthz, /readyz and the auth routes themselves
// must answer without a session, and deciding what requires one is the
// interceptor's job, not this one's.
//
// A nil *Service -- sign-in not configured -- passes every request through
// anonymously, which is what keeps a stack with no OAuth client working.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.configured() {
			next.ServeHTTP(w, r)
			return
		}

		user, err := s.Resolve(r.Context(), r)
		switch {
		case err == nil:
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, user))
		case errors.Is(err, ErrNoSession):
			// Anonymous. Not an error here.
		default:
			// The database is unreachable or the query failed. Log it and
			// continue anonymously: the interceptor turns that into an
			// unauthenticated response for calls that need a session, and
			// probes keep answering, which is what stops a database blip from
			// failing liveness.
			s.log.Error("auth: resolving session", "err", err)
		}
		next.ServeHTTP(w, r)
	})
}

// NewInterceptor rejects Connect calls that carry no session. Procedures in
// exempt stay reachable without one.
//
// HealthService/Check must be exempt: it is a Connect RPC behind /rpc rather
// than an HTTP probe path, and change 0.2's spec requires it to answer with no
// database and no credential. Forgetting it breaks that test.
func NewInterceptor(exempt ...string) connect.UnaryInterceptorFunc {
	open := make(map[string]bool, len(exempt))
	for _, p := range exempt {
		open[p] = true
	}

	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if open[req.Spec().Procedure] {
				return next(ctx, req)
			}
			if _, ok := FromContext(ctx); !ok {
				// A code, not a sentence, and no user data in the response.
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New(CodeUnauthenticated))
			}
			return next(ctx, req)
		}
	}
}

// CodeUnauthenticated is the body of an unauthenticated rejection. The
// Connect code carries the meaning; this string is what a client keys a
// translation off.
const CodeUnauthenticated = "unauthenticated"
