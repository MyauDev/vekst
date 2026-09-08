package identity

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

// stubProvider is an OIDC provider under the test's control: it serves a
// discovery document, a JWKS and a token endpoint, and mints ID tokens whose
// signature, audience, expiry and nonce the test chooses.
//
// It exists so the full redirect chain can be driven for real -- design risk
// row "the ID token is decoded rather than verified" is only closed by a test
// that presents a badly signed token to the actual verifier and watches it be
// refused. Asserting on attribute strings would prove nothing.
type stubProvider struct {
	server   *httptest.Server
	key      *rsa.PrivateKey
	wrongKey *rsa.PrivateKey
	clientID string

	// Knobs, read when the token endpoint mints a token.
	nonce         string
	subject       string
	email         string
	name          string
	locale        string
	emailVerified bool
	audience      string        // "" means clientID
	issuerClaim   string        // "" means the stub's own issuer
	lifetime      time.Duration // negative mints an already-expired token
	signWrong     bool          // sign with a key absent from the JWKS
	omitIDToken   bool
}

func newStubProvider(t *testing.T, clientID string) *stubProvider {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating signing key: %v", err)
	}
	wrong, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating decoy key: %v", err)
	}

	p := &stubProvider{
		key:           key,
		wrongKey:      wrong,
		clientID:      clientID,
		subject:       "stub-subject",
		email:         "person@example.test",
		name:          "A Person",
		locale:        "en",
		emailVerified: true,
		lifetime:      time.Hour,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                p.issuer(),
			"authorization_endpoint":                p.issuer() + "/authorize",
			"token_endpoint":                        p.issuer() + "/token",
			"jwks_uri":                              p.issuer() + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key:       key.Public(),
			KeyID:     "stub",
			Algorithm: string(jose.RS256),
			Use:       "sig",
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{
			"access_token": "stub-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}
		if !p.omitIDToken {
			body["id_token"] = p.mintIDToken(t)
		}
		writeJSON(w, body)
	})

	p.server = httptest.NewServer(mux)
	t.Cleanup(p.server.Close)
	return p
}

func (p *stubProvider) issuer() string { return p.server.URL }

func (p *stubProvider) mintIDToken(t *testing.T) string {
	t.Helper()

	aud := p.audience
	if aud == "" {
		aud = p.clientID
	}
	iss := p.issuerClaim
	if iss == "" {
		iss = p.issuer()
	}

	now := time.Now()
	claims := map[string]any{
		"iss":            iss,
		"sub":            p.subject,
		"aud":            aud,
		"exp":            now.Add(p.lifetime).Unix(),
		"iat":            now.Unix(),
		"nonce":          p.nonce,
		"email":          p.email,
		"email_verified": p.emailVerified,
		"name":           p.name,
		"locale":         p.locale,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshalling claims: %v", err)
	}

	signingKey := p.key
	if p.signWrong {
		// Signed by a key the JWKS does not advertise. The verifier must
		// refuse it; a decoder would not notice.
		signingKey = p.wrongKey
	}

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: signingKey, KeyID: "stub"}},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		t.Fatalf("constructing signer: %v", err)
	}
	obj, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	raw, err := obj.CompactSerialize()
	if err != nil {
		t.Fatalf("serialising: %v", err)
	}
	return raw
}

// nonceFromAuthURL reads the nonce the service put in the authorization
// redirect, so the stub can mint a token that matches -- or, by not calling
// this, one that does not.
func nonceFromAuthURL(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing authorization URL %q: %v", raw, err)
	}
	n := u.Query().Get("nonce")
	if n == "" {
		t.Fatalf("authorization URL carries no nonce: %s", raw)
	}
	return n
}

func stateFromAuthURL(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing authorization URL %q: %v", raw, err)
	}
	s := u.Query().Get("state")
	if s == "" {
		t.Fatalf("authorization URL carries no state: %s", raw)
	}
	return s
}

// assertPKCEChallenge fails unless the authorization request carries an S256
// challenge. Downgrading to "plain", or omitting it, would leave the code
// interception PKCE exists to prevent wide open.
func assertPKCEChallenge(t *testing.T, raw string) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing authorization URL: %v", err)
	}
	if got := u.Query().Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", got)
	}
	if u.Query().Get("code_challenge") == "" {
		t.Error("authorization URL carries no code_challenge")
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(fmt.Sprintf("stub provider: encoding response: %v", err))
	}
}
