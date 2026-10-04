package rpc

import "context"

const (
	MethodSessionPrompt = "session.prompt"
	MethodSessionAnswer = "session.answer"
)

type PromptParams struct {
	Session string `json:"session"`
}

type AnswerParams struct {
	Session string `json:"session"`
	Choice  string `json:"choice"`
	Prompt  string `json:"prompt,omitempty"`
}

type PromptChoice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type Prompt struct {
	ID      string         `json:"id,omitempty"`
	Text    string         `json:"text"`
	Choices []PromptChoice `json:"choices"`
	Raw     string         `json:"raw,omitempty"`
}

func (c *Client) SessionPrompt(ctx context.Context, session string) (Prompt, error) {
	var out Prompt
	err := c.Call(ctx, MethodSessionPrompt, PromptParams{Session: session}, &out)
	return out, err
}

func (c *Client) SessionAnswer(ctx context.Context, session, choice string) error {
	return c.Call(ctx, MethodSessionAnswer, AnswerParams{Session: session, Choice: choice}, nil)
}
