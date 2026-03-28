package arista

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// eosClient wraps HTTP transport details for talking to the EOS command API.
type eosClient struct {
	url        string
	httpClient *http.Client
}

// NewEosClient creates a client tuned for lab use (short timeout, TLS verify disabled).
func NewEosClient(url string) eosClient {
	// Configure a custom http.Transport with a modified TLS configuration
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// Create a custom client with a timeout
	cl := &http.Client{
		Timeout:   10 * time.Second,
		Transport: tr,
	}
	client := eosClient{url: url, httpClient: cl}
	return client
}

// getCreds centralizes auth lookup so callers do not hard-code credentials repeatedly.
func (c eosClient) getCreds() (string, string) {
	// TODO: this should be a call to a vault
	username := "admin"
	password := "admin"
	return username, password
}

// Run posts an eAPI request payload and decodes the JSON response into cmdResp.
func (c eosClient) Run(reqBody []byte, cmdResp any) error {
	// Create a new POST request with a body and custom headers
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(reqBody))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return fmt.Errorf("Error creating request:", err)
	}

	username, password := c.getCreds()
	// Set Basic Authentication headers.
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Println("Error performing request:", err)
		return fmt.Errorf("Error performing request:", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return fmt.Errorf("Error reading response body:", err)
	}

	if err := json.Unmarshal(body, &cmdResp); err != nil {
		return fmt.Errorf("Error unmarshalling resp body:", err)
	}

	return nil
}
