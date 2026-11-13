package api

import (
	"context"
	"net/http"
)

// Codes d'erreur de la connexion (TRD §7.2, §7.3). Pendant l'attente du code, ceux de la
// norme RFC 8628 : authorization_pending, slow_down, expired_token, access_denied.
const (
	CodeAuthorizationPending = "authorization_pending"
	CodeSlowDown             = "slow_down"
	CodeExpiredToken         = "expired_token"
	CodeAccessDenied         = "access_denied"
	CodeTokenInvalid         = "token_invalid"
	CodeAccountSuspended     = "account_suspended"
)

// DeviceCode : code demandé par kory login (POST /v1/auth/device).
type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// IssuedToken : jeton remis une seule fois après l'autorisation (POST /v1/auth/device/token).
type IssuedToken struct {
	Token     string  `json:"token"`
	TokenName string  `json:"token_name"`
	ExpiresAt *string `json:"expires_at"`
	Handle    string  `json:"handle"`
}

// User : compte relié au jeton (GET /v1/user).
type User struct {
	Handle         string  `json:"handle"`
	Name           *string `json:"name"`
	TokenName      string  `json:"token_name"`
	TokenExpiresAt *string `json:"token_expires_at"`
}

// RequestDeviceCode demande un code à afficher dans le terminal.
func (c *Client) RequestDeviceCode(ctx context.Context) (*DeviceCode, error) {
	var code DeviceCode
	if err := c.do(ctx, http.MethodPost, "/v1/auth/device", "", map[string]string{}, &code); err != nil {
		return nil, err
	}

	return &code, nil
}

// PollDeviceToken demande le jeton ; tant que le code n'est pas autorisé, l'API répond
// par une erreur authorization_pending (ou slow_down si le CLI interroge trop vite).
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string) (*IssuedToken, error) {
	var token IssuedToken
	if err := c.do(ctx, http.MethodPost, "/v1/auth/device/token", "", map[string]string{"device_code": deviceCode}, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

// CurrentUser lit le compte relié au jeton.
func (c *Client) CurrentUser(ctx context.Context, token string) (*User, error) {
	var user User
	if err := c.do(ctx, http.MethodGet, "/v1/user", token, nil, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

// RevokeToken révoque le jeton qui fait la requête (kory logout).
func (c *Client) RevokeToken(ctx context.Context, token string) error {
	return c.do(ctx, http.MethodDelete, "/v1/auth/token", token, nil, nil)
}
