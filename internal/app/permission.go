package app

type PermissionChoice struct {
	ID    string
	Label string
	Keys  []string
}

type PermissionPrompt struct {
	Text    string
	Choices []PermissionChoice
}

type PermissionPrompter interface {
	PermissionPrompt(screen string) (PermissionPrompt, bool)
}
