package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"koryhub.dev/kory/internal/version"
)

// Module : module vu par le CLI (GET /v1/modules/{auteur}/{nom}), ouvert sans compte.
type Module struct {
	Name          string           `json:"name"`
	Visibility    string           `json:"visibility"`
	Status        string           `json:"status"`
	LatestVersion *string          `json:"latest_version"`
	Description   *string          `json:"description"`
	Language      *string          `json:"language"`
	Certified     bool             `json:"certified"`
	DownloadCount int64            `json:"download_count"`
	Versions      []VersionSummary `json:"versions"`
}

// VersionSummary : une version dans la liste d'un module (publiée, dépréciée ou retirée).
type VersionSummary struct {
	Version            string  `json:"version"`
	Status             string  `json:"status"`
	Prerelease         bool    `json:"prerelease"`
	PublishedAt        *string `json:"published_at"`
	SHA256             string  `json:"sha256"`
	Size               int64   `json:"size"`
	DeprecationMessage *string `json:"deprecation_message"`
}

// VersionDetail : une version, ses prérequis et ses fichiers (GET …/versions/{version}).
type VersionDetail struct {
	Name               string        `json:"name"`
	Version            string        `json:"version"`
	Status             string        `json:"status"`
	Prerelease         bool          `json:"prerelease"`
	License            string        `json:"license"`
	Language           *string       `json:"language"`
	PublishedAt        *string       `json:"published_at"`
	SHA256             string        `json:"sha256"`
	Size               int64         `json:"size"`
	DeprecationMessage *string       `json:"deprecation_message"`
	Requires           []Requirement `json:"requires"`
	Files              []VersionFile `json:"files"`
}

// Requirement : code extérieur à installer soi-même (F-REG-27).
type Requirement struct {
	Ecosystem *string `json:"ecosystem"`
	Name      string  `json:"name"`
	Version   *string `json:"version"`
}

// VersionFile : un fichier de la version, avec son empreinte.
type VersionFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// AuditResult : état d'une version installée (POST /v1/audit).
type AuditResult struct {
	Name          string  `json:"name"`
	Version       string  `json:"version"`
	State         string  `json:"state"`
	LatestVersion *string `json:"latest_version"`
	Message       *string `json:"message"`
}

// AuditEntry : une ligne de kory.lock envoyée à l'audit.
type AuditEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Codes d'erreur du téléchargement.
const (
	CodeVersionRemoved     = "version_removed"
	CodeDownloadLimitGuest = "download_limit_guest"
	CodeDownloadLimitUser  = "download_limit_user"
	CodeNotFound           = "not_found"
)

func modulePath(author, name string) string {
	return "/v1/modules/" + url.PathEscape(author) + "/" + url.PathEscape(name)
}

// GetVersion lit une version, ses prérequis et ses fichiers.
func (c *Client) GetVersion(ctx context.Context, token, author, name, number string) (*VersionDetail, error) {
	var v VersionDetail
	if err := c.do(ctx, http.MethodGet, modulePath(author, name)+"/versions/"+url.PathEscape(number), token, nil, &v); err != nil {
		return nil, err
	}

	return &v, nil
}

// Audit envoie les versions de kory.lock ; rien n'est enregistré sur le serveur.
func (c *Client) Audit(ctx context.Context, token string, entries []AuditEntry) ([]AuditResult, error) {
	var response struct {
		Results []AuditResult `json:"results"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/audit", token, map[string]any{"modules": entries}, &response); err != nil {
		return nil, err
	}

	return response.Results, nil
}

// Download écrit l'archive d'une version dans w (redirection vers un lien signé suivie) ;
// progress reçoit les octets lus et la taille annoncée. Délai : 5 minutes.
func (c *Client) Download(ctx context.Context, token, author, name, number string, w io.Writer, progress func(done, total int64)) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+modulePath(author, name)+"/versions/"+url.PathEscape(number)+"/download", nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", version.UserAgent())
	request.Header.Set("Accept", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if c.Lang != "" {
		request.Header.Set("Accept-Language", c.Lang)
	}

	client := *c.HTTP
	client.Timeout = 0 // le contexte borne la durée
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<16))
		apiErr := &Error{Status: response.StatusCode}
		if json.Unmarshal(data, apiErr) != nil || apiErr.Code == "" {
			return fmt.Errorf("%w: HTTP %d", ErrInvalidResponse, response.StatusCode)
		}

		return apiErr
	}

	var done int64
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			if _, err := w.Write(buffer[:n]); err != nil {
				return err
			}
			done += int64(n)
			if progress != nil {
				progress(done, response.ContentLength)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("%w: %v", ErrUnreachable, readErr)
		}
	}
}
