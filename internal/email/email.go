package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type Sender struct {
	host      string
	token     string
	fromEmail string
	client    *http.Client
	log       *slog.Logger
}

func NewSender(host, token, fromEmail string, log *slog.Logger) *Sender {
	return &Sender{
		host:      host,
		token:     token,
		fromEmail: fromEmail,
		client:    &http.Client{Timeout: 30 * time.Second},
		log:       log,
	}
}

type zeptoRequest struct {
	From    zeptoAddress `json:"from"`
	To      []zeptoTo    `json:"to"`
	Subject string       `json:"subject"`
	HTML    string       `json:"htmlbody"`
}

type zeptoAddress struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

type zeptoTo struct {
	EmailAddress zeptoAddress `json:"email_address"`
}

func (s *Sender) Send(ctx context.Context, to, subject, htmlBody string) error {
	payload := zeptoRequest{
		From:    zeptoAddress{Address: s.fromEmail, Name: "Crow Watch"},
		To:      []zeptoTo{{EmailAddress: zeptoAddress{Address: to}}},
		Subject: subject,
		HTML:    htmlBody,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal email payload: %w", err)
	}

	url := fmt.Sprintf("https://%s/v1.1/email", s.host)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create email request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var respBody bytes.Buffer
		respBody.ReadFrom(resp.Body)
		s.log.Error("zeptomail error", "status", resp.StatusCode, "body", respBody.String())
		return fmt.Errorf("zeptomail returned status %d", resp.StatusCode)
	}

	s.log.Info("email sent", "to", to, "subject", subject)
	return nil
}
