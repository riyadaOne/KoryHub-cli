package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"koryhub.dev/kory/internal/version"
)

// PublishedVersion : rapport de publication (POST /v1/modules/{auteur}/{nom}/versions).
type PublishedVersion struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
	FilesCount int    `json:"files_count"`
	Size       int    `json:"size"`
	URL        string `json:"url"`
}

// PublishRequest : archive et champs envoyés par kory push.
type PublishRequest struct {
	Archive         []byte
	SHA256          string
	Visibility      string // nouveau module : public ou private ; vide sinon
	RightsCertified bool   // premier push du compte
	NoticeAccepted  bool
	Progress        func(sent, total int64)
}

// GetModule lit un module ; (nil, nil) s'il n'existe pas encore.
func (c *Client) GetModule(ctx context.Context, token, author, name string) (*Module, error) {
	var m Module
	err := c.do(ctx, http.MethodGet, "/v1/modules/"+url.PathEscape(author)+"/"+url.PathEscape(name), token, nil, &m)

	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &m, nil
}

// Publish envoie une version (multipart : archive, sha256, et champs du premier envoi).
func (c *Client) Publish(ctx context.Context, token, author, name string, req PublishRequest) (*PublishedVersion, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)

	part, err := form.CreateFormFile("archive", "kory.tar.gz")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(req.Archive); err != nil {
		return nil, err
	}
	fields := map[string]string{"sha256": req.SHA256}
	if req.Visibility != "" {
		fields["visibility"] = req.Visibility
	}
	if req.RightsCertified {
		fields["rights_certified"] = "1"
	}
	if req.NoticeAccepted {
		fields["notice_accepted"] = "1"
	}
	for key, value := range fields {
		if err := form.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	if err := form.Close(); err != nil {
		return nil, err
	}

	total := int64(body.Len())
	reader := io.Reader(&body)
	if req.Progress != nil {
		reader = &progressReader{r: &body, total: total, report: req.Progress}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/modules/"+url.PathEscape(author)+"/"+url.PathEscape(name)+"/versions", reader)
	if err != nil {
		return nil, err
	}
	request.ContentLength = total
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("User-Agent", version.UserAgent())
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	if c.Lang != "" {
		request.Header.Set("Accept-Language", c.Lang)
	}

	// L'envoi et les contrôles du serveur (recherche de secrets) peuvent prendre du temps.
	client := &http.Client{Timeout: 5 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if response.StatusCode >= 400 {
		apiErr := &Error{Status: response.StatusCode}
		if json.Unmarshal(data, apiErr) != nil || apiErr.Code == "" {
			return nil, fmt.Errorf("%w: HTTP %d", ErrInvalidResponse, response.StatusCode)
		}
		return nil, apiErr
	}

	var published PublishedVersion
	if err := json.Unmarshal(data, &published); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	return &published, nil
}

type progressReader struct {
	r      io.Reader
	sent   int64
	total  int64
	report func(sent, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.sent += int64(n)
	p.report(p.sent, p.total)

	return n, err
}
