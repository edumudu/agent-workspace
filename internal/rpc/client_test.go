package rpc_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/giovaniif/agent-workspace/internal/rpc"
)

func shortDir(t *testing.T) string {
	t.Helper()
	// why: macOS caps Unix socket paths at 104 bytes, and t.TempDir() can exceed it.
	dir, err := os.MkdirTemp("/tmp", "agentws-rpc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func serveStatus(t *testing.T, path string, pid int) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Error(err)
		return
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				sc := bufio.NewScanner(conn)
				enc := json.NewEncoder(conn)
				for sc.Scan() {
					var req rpc.Request
					if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
						return
					}
					result, _ := json.Marshal(rpc.Status{PID: pid, Sessions: 2})
					_ = enc.Encode(rpc.Response{V: rpc.Version, ID: req.ID, Result: result})
				}
			}()
		}
	}()
}

func TestConnectStartsTheDaemonWhenTheSocketIsMissing(t *testing.T) {
	path := filepath.Join(shortDir(t), "agentws.sock")
	starts := 0
	start := func() error {
		starts++
		go func() {
			time.Sleep(200 * time.Millisecond)
			serveStatus(t, path, 42)
		}()
		return nil
	}
	ctx := context.Background()
	c, err := rpc.Connect(ctx, path, start)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	st, err := c.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if starts != 1 || st.PID != 42 || st.Sessions != 2 {
		t.Fatalf("starts %d, status %+v", starts, st)
	}
}

func TestConnectDoesNotStartARunningDaemon(t *testing.T) {
	path := filepath.Join(shortDir(t), "agentws.sock")
	serveStatus(t, path, 7)
	c, err := rpc.Connect(context.Background(), path, func() error {
		t.Error("started a second daemon")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
}

func TestConnectGivesUpAfterTheStartTimeout(t *testing.T) {
	path := filepath.Join(shortDir(t), "agentws.sock")
	began := time.Now()
	_, err := rpc.Connect(context.Background(), path, func() error { return nil })
	if err == nil {
		t.Fatal("want error")
	}
	if took := time.Since(began); took < rpc.StartTimeout || took > rpc.StartTimeout+time.Second {
		t.Fatalf("gave up after %v", took)
	}
}

func TestConnectReportsAFailedStart(t *testing.T) {
	path := filepath.Join(shortDir(t), "agentws.sock")
	boom := errors.New("boom")
	_, err := rpc.Connect(context.Background(), path, func() error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err %v, want %v", err, boom)
	}
}
