package identity

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/oauth2"
)

// idClaims is the part of the ID token this package reads. Nothing here is
// trusted until verifyIDToken has checked the token that carried it.
type idClaims struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Locale        string `json:"locale"`
}

// verifyIDToken validates the token properly rather than decoding it:
// signature against the provider's JWKS, issuer, audience equal to this
// client, expiry, and nonce equal to the flow's. The library does the first
// four; hand-rolling them is how the check silently becomes a base64 decode.
//
// The nonce is checked here rather than by the library because only the
// consumed auth_flows row knows what it should be. It is what proves this ID
// token was minted for this sign-in attempt and is not a replayed or injected
// one.
func (s *Service) verifyIDToken(ctx context.Context, token *oauth2.Token, wantNonce string) (*idClaims, error) {
	raw, ok := token.Extra("id_token").(string)
	if !ok || raw == "" {
		return nil, errors.New("identity: response carried no id_token")
	}

	idToken, err := s.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("identity: verifying id token: %w", err)
	}
	if idToken.Nonce != wantNonce {
		return nil, errors.New("identity: id token nonce does not match the flow")
	}

	var claims idClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("identity: reading claims: %w", err)
	}
	if claims.Subject == "" {
		return nil, errors.New("identity: id token carried no subject")
	}
	return &claims, nil
}
