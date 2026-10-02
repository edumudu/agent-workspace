package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/giovaniif/agent-workspace/internal/adapters/notify"
	"github.com/giovaniif/agent-workspace/internal/domain"
)

const notifyUsage = "usage: agentws notify stream | agentws notify bridge [--remote-bin path] <ssh host>"

func runNotify(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, notifyUsage)
		return 2
	}
	switch args[0] {
	case "stream":
		return runNotifyStream(stdout, stderr)
	case "bridge":
		p, err := parseBridge(args[1:])
		if err != nil {
			fmt.Fprintf(stderr, "agentws notify bridge: %v\n%s\n", err, notifyUsage)
			return 2
		}
		return runBridge(p, stderr)
	default:
		fmt.Fprintln(stderr, notifyUsage)
		return 2
	}
}

func runNotifyStream(stdout, stderr io.Writer) int {
	ctx := context.Background()
	c, err := connectHome(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "agentws notify stream: %v\n", err)
		return 1
	}
	defer func() { _ = c.Close() }()
	notices, err := c.StreamNotices(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "agentws notify stream: %v\n", err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	for n := range notices {
		if err := enc.Encode(n); err != nil {
			return 1
		}
	}
	fmt.Fprintln(stderr, "agentws notify stream: daemon went away")
	return 1
}

type bridge struct {
	host      string
	remoteBin string
}

func parseBridge(args []string) (bridge, error) {
	fs := flag.NewFlagSet("bridge", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	remote := fs.String("remote-bin", "agentws", "")
	if err := fs.Parse(args); err != nil {
		return bridge{}, err
	}
	if fs.NArg() != 1 {
		return bridge{}, errors.New("needs exactly one ssh host")
	}
	return bridge{host: fs.Arg(0), remoteBin: *remote}, nil
}

// why: ssh joins the remote command into one shell line, so the binary is quoted there;
// keepalives notice a dead link instead of waiting on TCP for hours.
func (b bridge) streamArgv() []string {
	return []string{"ssh", "-T", "-o", "ServerAliveInterval=15", b.host, remotePath(b.remoteBin) + " notify stream"}
}

// why: a quoted ~ is literal, so a leading ~/ stays outside the quotes for the remote shell to expand.
func remotePath(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return "~/" + shellQuote(rest)
	}
	return shellQuote(p)
}

func (b bridge) focusCmd() string {
	return "ssh -T " + shellQuote(b.host) + " " + remotePath(b.remoteBin) + " focus"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

const (
	bridgeRetryMin = 2 * time.Second
	bridgeRetryMax = time.Minute
	// why: a stream that lasted this long was healthy, so the next drop retries fast again.
	bridgeHealthy = time.Minute
)

func runBridge(b bridge, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log.SetOutput(stderr)
	self, _ := os.Executable()
	home := os.Getenv("AGENTWS_HOME")
	n := notify.Detect(self, home)
	if tn, ok := n.(notify.TerminalNotifier); ok {
		tn.FocusCmd = b.focusCmd()
		n = tn
	} else {
		log.Print("terminal-notifier not on PATH: banners use osascript and clicks do nothing")
	}
	target := notify.RelayTarget{
		Notifier:   n,
		Foreground: notify.New(),
		Terminal:   domain.TerminalBundle(os.Getenv("__CFBundleIdentifier"), os.Getenv("TERM_PROGRAM")),
	}
	wait := bridgeRetryMin
	for ctx.Err() == nil {
		began := time.Now()
		err := streamOnce(ctx, b, target, stderr)
		if ctx.Err() != nil {
			break
		}
		if time.Since(began) > bridgeHealthy {
			wait = bridgeRetryMin
		}
		log.Printf("bridge to %s dropped (%v); retrying in %s", b.host, err, wait)
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
		wait = min(2*wait, bridgeRetryMax)
	}
	return 0
}

func streamOnce(ctx context.Context, b bridge, target notify.RelayTarget, stderr io.Writer) error {
	argv := b.streamArgv()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	log.Printf("bridging banners from %s", b.host)
	relayErr := notify.Relay(ctx, out, target)
	return errors.Join(relayErr, cmd.Wait())
}
