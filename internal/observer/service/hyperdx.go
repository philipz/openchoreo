package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
)

// HyperDXSigner signs embed URLs so that HyperDX dashboards can be shared securely.
type HyperDXSigner struct {
	baseURL string
	key     []byte
	ttl     time.Duration
}

func newHyperDXSigner(cfg config.HyperDXConfig) *HyperDXSigner {
	if cfg.SigningKey == "" || cfg.BaseURL == "" {
		return nil
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &HyperDXSigner{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		key:     []byte(cfg.SigningKey),
		ttl:     ttl,
	}
}

// Generate builds a signed URL for the provided path and query parameters.
func (s *HyperDXSigner) Generate(path string, params map[string]string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("hyperdx signer is not configured")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	expiry := time.Now().Add(s.ttl).Unix()

	queryParams := url.Values{}
	for k, v := range params {
		queryParams.Set(k, v)
	}
	queryParams.Set("expires", fmt.Sprintf("%d", expiry))

	sig := s.sign(path, queryParams)
	queryParams.Set("signature", sig)

	u := fmt.Sprintf("%s%s?%s", s.baseURL, path, queryParams.Encode())
	return u, nil
}

func (s *HyperDXSigner) sign(path string, values url.Values) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteString(path)
	for _, k := range keys {
		builder.WriteString("|")
		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(values.Get(k))
	}

	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(builder.String()))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
