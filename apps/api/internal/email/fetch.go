package email

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Resend's inbound webhook carries metadata only. The text, html and headers of
// a received message are read from the Receiving API with an API key.

// BodyFetcher loads the content of a received message the webhook left out.
type BodyFetcher interface {
	FetchReceived(ctx context.Context, emailID string) (*ReceivedContent, error)
}

// ReceivedContent is the part of a received email the webhook does not carry.
type ReceivedContent struct {
	Text       string
	HTML       string
	InReplyTo  string
	References string
}

// ResendFetcher reads received mail from the Resend Receiving API.
type ResendFetcher struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

// NewResendFetcher returns a fetcher, or nil when there is no key.
func NewResendFetcher(apiKey string) *ResendFetcher {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	return &ResendFetcher{
		APIKey:  strings.TrimSpace(apiKey),
		BaseURL: "https://api.resend.com",
		Client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (f *ResendFetcher) FetchReceived(ctx context.Context, emailID string) (*ReceivedContent, error) {
	endpoint := strings.TrimRight(f.BaseURL, "/") + "/emails/receiving/" + url.PathEscape(emailID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("email: build fetch request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("email: fetch received message: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 25<<20))
	if err != nil {
		return nil, fmt.Errorf("email: read received message: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("email: fetch received message: resend answered %d", resp.StatusCode)
	}
	return parseReceived(raw)
}

func parseReceived(raw []byte) (*ReceivedContent, error) {
	var r struct {
		Text    *string        `json:"text"`
		HTML    *string        `json:"html"`
		Headers map[string]any `json:"headers"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("email: received message is not valid JSON: %w", err)
	}
	c := &ReceivedContent{}
	if r.Text != nil {
		c.Text = *r.Text
	}
	if r.HTML != nil {
		c.HTML = *r.HTML
	}
	for k, v := range r.Headers {
		s, ok := v.(string)
		if !ok {
			continue
		}
		switch strings.ToLower(k) {
		case "in-reply-to":
			c.InReplyTo = strings.TrimSpace(s)
		case "references":
			c.References = strings.TrimSpace(s)
		}
	}
	return c, nil
}

// inboundEmailID returns the provider's id for a received message, or "".
func inboundEmailID(body []byte) string {
	var env struct {
		Data struct {
			EmailID string `json:"email_id"`
		} `json:"data"`
		EmailID string `json:"email_id"`
	}
	if json.Unmarshal(body, &env) != nil {
		return ""
	}
	if env.Data.EmailID != "" {
		return env.Data.EmailID
	}
	return env.EmailID
}
