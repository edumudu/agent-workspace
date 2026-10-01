package notify

import (
	"context"
	"os/exec"
	"strings"

	"github.com/giovaniif/agent-workspace/internal/app"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

var _ app.Notifier = TerminalNotifier{}

// why: unlike osascript it can group banners per session, withdraw them and
// run a command on click.
type TerminalNotifier struct {
	Run Runner
	Bin string
	// why: Self and Home build the click command, which runs outside the daemon's environment.
	Self string
	Home string
}

func (n TerminalNotifier) Notify(ctx context.Context, b domain.Banner) error {
	args := []string{"-title", b.Title, "-message", escapeMessage(b.Body)}
	if b.Group != "" {
		args = append(args, "-group", b.Group)
	}
	if b.Sound != "" {
		args = append(args, "-sound", b.Sound)
	}
	if b.Terminal != "" {
		args = append(args, "-activate", b.Terminal)
	}
	if b.Group != "" && n.Self != "" {
		args = append(args, "-execute", "AGENTWS_HOME="+shellQuote(n.Home)+" "+shellQuote(n.Self)+" focus "+shellQuote(b.Group))
	}
	_, err := n.Run(ctx, n.Bin, args...)
	return err
}

func (n TerminalNotifier) Remove(ctx context.Context, group string) error {
	_, err := n.Run(ctx, n.Bin, "-remove", group)
	return err
}

// why: terminal-notifier reads a message starting with [ or - as an option; a
// leading backslash makes it literal.
func escapeMessage(s string) string {
	if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "-") {
		return `\` + s
	}
	return s
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// why: chosen once at daemon start, so a banner never pays for a PATH lookup.
func Select(lookPath func(string) (string, error), self, home string) app.Notifier {
	if bin, err := lookPath("terminal-notifier"); err == nil {
		return TerminalNotifier{Run: execRunner, Bin: bin, Self: self, Home: home}
	}
	return New()
}

func Detect(self, home string) app.Notifier { return Select(exec.LookPath, self, home) }
