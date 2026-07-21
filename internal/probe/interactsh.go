package probe

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	correlationIDLength = 20
	nonceLength         = 13
)

// Interaction is the subset of the Interactsh event schema used by llmfp.
type Interaction struct {
	Protocol      string    `json:"protocol"`
	UniqueID      string    `json:"unique-id"`
	FullID        string    `json:"full-id"`
	QType         string    `json:"q-type,omitempty"`
	RawRequest    string    `json:"raw-request,omitempty"`
	RawResponse   string    `json:"raw-response,omitempty"`
	RemoteAddress string    `json:"remote-address"`
	Timestamp     time.Time `json:"timestamp"`
}

type pollResponse struct {
	Data    []string `json:"data"`
	Extra   []string `json:"extra"`
	AESKey  string   `json:"aes_key"`
	TLDData []string `json:"tlddata,omitempty"`
}

type Client struct {
	baseURL       *url.URL
	httpClient    *http.Client
	correlationID string
	secretKey     string
	privateKey    *rsa.PrivateKey
	events        chan *Interaction
	cancel        context.CancelFunc
	done          chan struct{}
	once          sync.Once
}

func New(serverName string) (*Client, error) {
	baseURL, err := normalizeServerURL(serverName)
	if err != nil {
		return nil, err
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate Interactsh session key: %w", err)
	}
	correlationID, err := randomHex(correlationIDLength / 2)
	if err != nil {
		return nil, fmt.Errorf("generate Interactsh correlation ID: %w", err)
	}
	secretKey, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("generate Interactsh secret: %w", err)
	}

	client := &Client{
		baseURL:       baseURL,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		correlationID: correlationID,
		secretKey:     secretKey,
		privateKey:    privateKey,
		events:        make(chan *Interaction, 64),
		done:          make(chan struct{}),
	}
	if err := client.register(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	client.cancel = cancel
	go client.poll(ctx, time.Second)
	return client, nil
}

func normalizeServerURL(serverName string) (*url.URL, error) {
	if serverName == "" {
		return nil, errors.New("Interactsh server is empty")
	}
	if !strings.Contains(serverName, "://") {
		serverName = "https://" + serverName
	}
	parsed, err := url.Parse(serverName)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Interactsh server %q", serverName)
	}
	if parsed.Scheme != "https" {
		return nil, errors.New("Interactsh server must use https")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func (c *Client) register() error {
	publicDER, err := x509.MarshalPKIXPublicKey(&c.privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("marshal Interactsh public key: %w", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: publicDER})
	payload := struct {
		PublicKey     string `json:"public-key"`
		SecretKey     string `json:"secret-key"`
		CorrelationID string `json:"correlation-id"`
	}{base64.StdEncoding.EncodeToString(publicPEM), c.secretKey, c.correlationID}
	return c.postJSON("/register", payload, "registration successful")
}

func (c *Client) Host() string {
	nonce, err := randomHex((nonceLength + 1) / 2)
	if err != nil {
		return ""
	}
	nonce = nonce[:nonceLength]
	return c.correlationID + nonce + "." + c.baseURL.Hostname()
}

func (c *Client) Events() <-chan *Interaction {
	return c.events
}

func (c *Client) poll(ctx context.Context, interval time.Duration) {
	defer close(c.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, interaction := range c.getInteractions(ctx) {
				select {
				case c.events <- interaction:
				default:
				}
			}
		}
	}
}

func (c *Client) getInteractions(ctx context.Context) []*Interaction {
	pollURL := *c.baseURL
	pollURL.Path += "/poll"
	query := pollURL.Query()
	query.Set("id", c.correlationID)
	query.Set("secret", c.secretKey)
	pollURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL.String(), nil)
	if err != nil {
		return nil
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	var result pollResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&result); err != nil {
		return nil
	}

	interactions := make([]*Interaction, 0, len(result.Data)+len(result.Extra)+len(result.TLDData))
	for _, encrypted := range result.Data {
		plaintext, err := c.decrypt(result.AESKey, encrypted)
		if err == nil {
			interactions = appendInteraction(interactions, plaintext)
		}
	}
	for _, plaintext := range append(result.Extra, result.TLDData...) {
		interactions = appendInteraction(interactions, []byte(plaintext))
	}
	return interactions
}

func appendInteraction(target []*Interaction, data []byte) []*Interaction {
	var interaction Interaction
	if err := json.Unmarshal(bytes.TrimSpace(data), &interaction); err == nil {
		return append(target, &interaction)
	}
	return target
}

func (c *Client) decrypt(encodedKey, encodedMessage string) ([]byte, error) {
	encryptedKey, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, err
	}
	key, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, c.privateKey, encryptedKey, nil)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encodedMessage)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < aes.BlockSize {
		return nil, errors.New("encrypted interaction is shorter than its IV")
	}
	plaintext := make([]byte, len(ciphertext)-aes.BlockSize)
	cipher.NewCTR(block, ciphertext[:aes.BlockSize]).XORKeyStream(plaintext, ciphertext[aes.BlockSize:])
	return plaintext, nil
}

func (c *Client) Close() error {
	var closeErr error
	c.once.Do(func() {
		c.cancel()
		<-c.done
		payload := struct {
			CorrelationID string `json:"correlation-id"`
			SecretKey     string `json:"secret-key"`
		}{c.correlationID, c.secretKey}
		closeErr = c.postJSON("/deregister", payload, "deregistration successful")
	})
	return closeErr
}

func (c *Client) postJSON(path string, payload any, expectedMessage string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := *c.baseURL
	endpoint.Path += path
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Interactsh %s: %w", strings.TrimPrefix(path, "/"), err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Interactsh %s returned HTTP %d: %s", strings.TrimPrefix(path, "/"), response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var envelope struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return fmt.Errorf("decode Interactsh %s response: %w", strings.TrimPrefix(path, "/"), err)
	}
	if envelope.Message != expectedMessage {
		return fmt.Errorf("Interactsh %s failed: %s", strings.TrimPrefix(path, "/"), envelope.Message)
	}
	return nil
}

func randomHex(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
