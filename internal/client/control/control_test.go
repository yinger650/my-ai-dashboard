package control

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPingAndUnknown(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "c.sock")
	srv, err := Listen(sock, func(req Request) Response {
		if req.Op == "ping" {
			return Response{OK: true}
		}
		if req.Op == "reload" {
			return Response{OK: true}
		}
		return Response{Error: "unknown op " + req.Op}
	})
	if err != nil {
		t.Fatal(err)
	}
	ctxDone := make(chan struct{})
	go func() {
		_ = srv.Serve(t.Context())
		close(ctxDone)
	}()
	t.Cleanup(func() { _ = srv.ln.Close() })

	resp, err := Call(sock, Request{Op: "ping"}, time.Second)
	if err != nil || !resp.OK {
		t.Fatalf("%v %+v", err, resp)
	}
	resp, err = Call(sock, Request{Op: "nope"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Error == "" {
		t.Fatalf("%+v", resp)
	}
}
