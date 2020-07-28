package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

// NewRegistryClient is a wrapper for creating a customized http.Client based on go-containerregistry abstractions
func NewRegistryClient(registry name.Registry, keychain authn.Keychain, scopes ...string) (*http.Client, error) {
	auth, err := keychain.Resolve(registry)
	if err != nil {
		return nil, fmt.Errorf("Error resolving credentials: %w", err)
	}
	tr, err := transport.New(registry, auth, http.DefaultTransport, scopes)
	if err != nil {
		return nil, fmt.Errorf("Error creating registry client transport: %w", err)
	}
	client := &http.Client{Transport: tr}
	if err != nil {
		return nil, fmt.Errorf("Error creating registry client: %w", err)
	}
	return client, nil
}

// ExecuteRequest executes an HTTP request for <method> and <url> using <client>
// and returns the decoded response in <result> (which therefore should be a pointer to a struct type with json tags)
// Opinionated and expects a JSON response
func ExecuteRequest(ctx context.Context, client *http.Client, method string, url string, result interface{}) error {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return fmt.Errorf("Error preparing http request: %w", err)
	}
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error sending request: %w", err)
	}

	if err := transport.CheckError(resp, http.StatusOK); err != nil {
		return fmt.Errorf("Request failed: %w", err)
	}

	// var body2 bytes.Buffer
	// body1 := io.TeeReader(resp.Body, &body2)
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("Error parsing json response: %w", err)
	}

	// tmp, _ := ioutil.ReadAll(&body2)
	// debugvar := string(tmp)
	// fmt.Print(debugvar)

	return nil
}
