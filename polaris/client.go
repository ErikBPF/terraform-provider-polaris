package polaris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type client struct {
	base, realm, token, id, secret string
	staticToken                    bool
	expires                        time.Time
	http                           *http.Client
	authMu                         sync.Mutex
	mutationMu                     sync.Mutex
}

func newClient(base, realm, token, id, secret string) (*client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("base_url must be an HTTP(S) origin without credentials, query, fragment or API path")
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, fmt.Errorf("HTTPS is required except for loopback fixtures")
		}
	}
	if token == "" && (id == "" || secret == "") {
		return nil, fmt.Errorf("provide token or both client_id and client_secret")
	}
	return &client{
		base: strings.TrimRight(base, "/"), realm: realm, token: token,
		id: id, secret: secret, staticToken: token != "",
		http: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func (c *client) bearer(ctx context.Context) (string, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.staticToken || (c.token != "" && time.Now().Before(c.expires)) {
		return c.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.id}, "client_secret": {c.secret}, "scope": {"PRINCIPAL_ROLE:ALL"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/catalog/v1/oauth/tokens", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("cannot build token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Polaris-Realm", c.realm)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("Polaris token request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Polaris token request failed: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil || payload.AccessToken == "" {
		return "", fmt.Errorf("invalid Polaris token response")
	}
	c.token = payload.AccessToken
	if payload.ExpiresIn <= 0 {
		payload.ExpiresIn = 60
	}
	c.expires = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second * 9 / 10)
	return c.token, nil
}

func (c *client) request(ctx context.Context, method, path string, input, output any, accepted ...int) (int, error) {
	token, err := c.bearer(ctx)
	if err != nil {
		return 0, err
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return 0, fmt.Errorf("cannot encode Polaris request")
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/api/management/v1"+path, body)
	if err != nil {
		return 0, fmt.Errorf("cannot build Polaris request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Polaris-Realm", c.realm)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("Polaris %s request failed", method)
	}
	defer resp.Body.Close()
	ok := false
	for _, status := range accepted {
		if resp.StatusCode == status {
			ok = true
			break
		}
	}
	if !ok {
		return resp.StatusCode, fmt.Errorf("Polaris %s request failed: HTTP %d", method, resp.StatusCode)
	}
	if output != nil && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(output); err != nil {
			return resp.StatusCode, fmt.Errorf("invalid Polaris JSON response")
		}
	}
	return resp.StatusCode, nil
}
