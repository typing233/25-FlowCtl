package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIClient wraps an HTTP client with auth and tenant injection.
type APIClient struct {
	BaseURL    string
	Token      string
	Tenant     string
	HTTPClient *http.Client
}

// APIError represents an error response from the API.
type APIError struct {
	StatusCode int
	Message    string `json:"message"`
	Detail     string `json:"detail,omitempty"`
}

func (e *APIError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("API error %d: %s - %s", e.StatusCode, e.Message, e.Detail)
	}
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// NewAPIClient creates a new API client using the global config values.
func NewAPIClient() *APIClient {
	return &APIClient{
		BaseURL: cfgServer,
		Token:   cfgToken,
		Tenant:  cfgTenant,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Do executes an HTTP request with auth and tenant headers.
func (c *APIClient) Do(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.Tenant != "" {
		req.Header.Set("X-Tenant", c.Tenant)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	return resp, nil
}

// DoJSON executes a request and decodes the JSON response into dest.
func (c *APIClient) DoJSON(method, path string, body interface{}, dest interface{}) error {
	resp, err := c.Do(method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseAPIError(resp)
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}

// Get performs a GET request and decodes the response.
func (c *APIClient) Get(path string, dest interface{}) error {
	return c.DoJSON(http.MethodGet, path, nil, dest)
}

// Post performs a POST request and decodes the response.
func (c *APIClient) Post(path string, body interface{}, dest interface{}) error {
	return c.DoJSON(http.MethodPost, path, body, dest)
}

// Put performs a PUT request and decodes the response.
func (c *APIClient) Put(path string, body interface{}, dest interface{}) error {
	return c.DoJSON(http.MethodPut, path, body, dest)
}

// Delete performs a DELETE request and decodes the response.
func (c *APIClient) Delete(path string, dest interface{}) error {
	return c.DoJSON(http.MethodDelete, path, nil, dest)
}

func parseAPIError(resp *http.Response) error {
	apiErr := &APIError{StatusCode: resp.StatusCode}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		apiErr.Message = fmt.Sprintf("HTTP %d (failed to read body)", resp.StatusCode)
		return apiErr
	}

	if err := json.Unmarshal(data, apiErr); err != nil {
		apiErr.Message = string(data)
	}

	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(resp.StatusCode)
	}

	return apiErr
}
