package notify

import (
	"context"
	"errors"
	"log"
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
	// why: set by a bridge, whose sessions live on another machine; the session id is appended.
	FocusCmd string
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
	if b.Group != "" && n.FocusCmd != "" {
		args = append(args, "-execute", n.FocusCmd+" "+shellQuote(b.Group))
	} else if b.Group != "" && n.Self != "" {
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

// why: Self and Home build the default click command; FocusCmd replaces it
// for a bridge, whose sessions live on another machine.
type Click struct {
	Self     string
	Home     string
	FocusCmd string
}

// why: chosen once at daemon start, so a banner never pays for a PATH lookup.
func Select(lookPath func(string) (string, error), click Click) app.Notifier {
	_, osaErr := lookPath("osascript")
	if bin, err := lookPath("terminal-notifier"); err == nil {
		tn := TerminalNotifier{Run: execRunner, Bin: bin, Self: click.Self, Home: click.Home, FocusCmd: click.FocusCmd}
		if osaErr != nil {
			return tn
		}
		return Fallback{Primary: tn, Secondary: New()}
	}
	if osaErr == nil {
		return New()
	}
	return Silent{}
}

func Detect(click Click) app.Notifier { return Select(exec.LookPath, click) }

// why: terminal-notifier fails when macOS has its notifications turned off,
// which a fresh install often does; osascript posts the banner instead, without grouping or click.
type Fallback struct {
	Primary   app.Notifier
	Secondary app.Notifier
}

func (f Fallback) Notify(ctx context.Context, b domain.Banner) error {
	err := f.Primary.Notify(ctx, b)
	if err == nil {
		return nil
	}
	log.Printf("notify: %v; posting with the fallback", err)
	if err2 := f.Secondary.Notify(ctx, b); err2 != nil {
		return errors.Join(err, err2)
	}
	return nil
}

func (f Fallback) Remove(ctx context.Context, group string) error {
	return f.Primary.Remove(ctx, group)
}
