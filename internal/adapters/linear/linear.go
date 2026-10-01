package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

var _ app.TitleResolver = Client{}

const defaultEndpoint = "https://api.linear.app/graphql"

const query = `query($id: String!) { issue(id: $id) { title } }`

const maxResponse = 1 << 20

// why: HTTP defaults to a client that does not follow redirects, so the token is never forwarded anywhere else.
type Client struct {
	Token    string
	Endpoint string
	HTTP     *http.Client
}

var defaultHTTP = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (c Client) Title(ctx context.Context, task domain.Task) (string, error) {
	if c.Token == "" || task.Source != domain.TaskLinear || task.Ref == "" {
		return "", nil
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": map[string]string{"id": task.Ref}})
	if err != nil {
		return "", err
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.Token)
	client := c.HTTP
	if client == nil {
		client = defaultHTTP
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("linear: %s", resp.Status)
	}
	var reply struct {
		Data struct {
			Issue *struct {
				Title string `json:"title"`
			} `json:"issue"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return "", fmt.Errorf("linear: %w", err)
	}
	if len(reply.Errors) > 0 {
		return "", fmt.Errorf("linear: %s", reply.Errors[0].Message)
	}
	if reply.Data.Issue == nil {
		return "", fmt.Errorf("linear: no issue %s", task.Ref)
	}
	return strings.TrimSpace(reply.Data.Issue.Title), nil
}

func LoadToken(path string) (string, error) {
	var cfg struct {
		Linear struct {
			Token string `toml:"token"`
		} `toml:"linear"`
	}
	_, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("linear: %s: %w", path, err)
	}
	return strings.TrimSpace(cfg.Linear.Token), nil
}
