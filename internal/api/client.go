// Package api parle à l'API du CLI, api.koryhub.dev/v1 (TRD §17). Les types de
// réponse suivent le contrat OpenAPI exporté par le serveur (api/openapi-v1.json).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"koryhub.dev/kory/internal/version"
)

// Error est une erreur renvoyée par l'API : statut, code stable, message déjà traduit.
type Error struct {
	Status  int                 `json:"-"`
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

// CodeUpgradeRequired : version du CLI inférieure au minimum (réponse 426).
const CodeUpgradeRequired = "upgrade_required"

// ErrUnreachable : l'API n'a pas répondu (réseau, adresse).
var ErrUnreachable = errors.New("api unreachable")

// ErrInvalidResponse : l'API a répondu quelque chose qui n'est pas au format attendu.
var ErrInvalidResponse = errors.New("invalid api response")

// Client appelle l'API dans la langue de l'utilisateur.
type Client struct {
	BaseURL string
	Lang    string
	HTTP    *http.Client
}

// NewClient prépare un client vers baseURL (sans /v1).
func NewClient(baseURL, lang string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Lang:    lang,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Meta : versions de l'API et du CLI (GET /v1/meta).
type Meta struct {
	APIVersion        string `json:"api_version"`
	MinimumCLIVersion string `json:"minimum_cli_version"`
	LatestCLIVersion  string `json:"latest_cli_version"`
}

// Meta lit les versions annoncées par l'API.
func (c *Client) Meta(ctx context.Context) (*Meta, error) {
	var meta Meta
	if err := c.get(ctx, "/v1/meta", &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func (c *Client) get(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}

	request.Header.Set("User-Agent", version.UserAgent())
	request.Header.Set("Accept", "application/json")
	if c.Lang != "" {
		request.Header.Set("Accept-Language", c.Lang)
	}

	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}

	if response.StatusCode >= 400 {
		apiErr := &Error{Status: response.StatusCode}
		if json.Unmarshal(body, apiErr) != nil || apiErr.Code == "" {
			return fmt.Errorf("%w: HTTP %d", ErrInvalidResponse, response.StatusCode)
		}

		return apiErr
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	return nil
}
