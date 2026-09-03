package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeHTTPSuccessAndFailure(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok-live"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("missing"))
	}))
	defer okSrv.Close()

	ctx := context.Background()
	up := ProbeHTTP(ctx, 2*time.Second, true, HTTPTarget{
		ServiceKey: "site-ok", Name: "ok", URL: okSrv.URL + "/health", Method: "GET", ExpectStatus: []int{200}, ExpectContains: "ok",
	})
	if !up.OK || up.StatusCode != 200 {
		t.Fatalf("healthy probe: %+v", up)
	}
	if !strings.Contains(up.Summary, "HTTP 200") {
		t.Fatalf("summary = %q", up.Summary)
	}
	state, _, sev := up.Projection(3 * time.Second)
	if state != "running" || sev != "normal" {
		t.Fatalf("projection = %s/%s", state, sev)
	}
	items := up.StatusItems(3 * time.Second)
	if len(items) < 3 {
		t.Fatalf("status items = %d", len(items))
	}

	down := ProbeHTTP(ctx, 2*time.Second, true, HTTPTarget{
		ServiceKey: "site-404", Name: "missing", URL: okSrv.URL + "/nope", ExpectStatus: []int{200},
	})
	if down.OK || down.StatusCode != 404 {
		t.Fatalf("404 probe: %+v", down)
	}
	st, _, sev := down.Projection(0)
	if st != "failed" || sev != "error" {
		t.Fatalf("404 projection = %s/%s", st, sev)
	}
	if !strings.Contains(down.LogMarkdown(), "探测失败") {
		t.Fatalf("log markdown = %s", down.LogMarkdown())
	}

	mismatch := ProbeHTTP(ctx, 2*time.Second, true, HTTPTarget{
		ServiceKey: "site-body", URL: okSrv.URL + "/health", ExpectContains: "never-this",
	})
	if mismatch.OK {
		t.Fatal("expected body mismatch failure")
	}
}

func TestProbeHTTPTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := ProbeHTTP(context.Background(), 80*time.Millisecond, true, HTTPTarget{
		ServiceKey: "slow", URL: srv.URL,
	})
	if res.OK {
		t.Fatalf("expected timeout failure, got %+v", res)
	}
	if !strings.Contains(res.Summary, "超时") {
		t.Fatalf("summary = %q", res.Summary)
	}
}

func TestProbeHTTPRedirectAndTLS(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("landed"))
	}))
	defer final.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redir.Close()

	followed := ProbeHTTP(context.Background(), 2*time.Second, true, HTTPTarget{
		ServiceKey: "redir", URL: redir.URL, ExpectStatus: []int{200},
	})
	if !followed.OK || followed.StatusCode != 200 {
		t.Fatalf("follow redirects: %+v", followed)
	}
	stopped := ProbeHTTP(context.Background(), 2*time.Second, false, HTTPTarget{
		ServiceKey: "redir", URL: redir.URL, ExpectStatus: []int{200},
	})
	if stopped.OK || stopped.StatusCode != 302 {
		t.Fatalf("no follow: %+v", stopped)
	}

	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer tlsSrv.Close()
	tlsRes := ProbeHTTP(context.Background(), 2*time.Second, true, HTTPTarget{
		ServiceKey: "tls", URL: tlsSrv.URL, TLSInsecure: true,
	})
	if !tlsRes.OK {
		t.Fatalf("tls probe: %+v", tlsRes)
	}
	if tlsRes.SSLDaysLeft == nil || *tlsRes.SSLDaysLeft < 0 {
		t.Fatalf("ssl days = %v", tlsRes.SSLDaysLeft)
	}
	items := tlsRes.StatusItems(0)
	foundSSL := false
	for _, it := range items {
		if it.Key == "ssl_days" {
			foundSSL = true
		}
	}
	if !foundSSL {
		t.Fatalf("missing ssl_days item: %+v", items)
	}
}

func TestProbeAllPreservesOrder(t *testing.T) {
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer b.Close()

	results := ProbeAll(context.Background(), time.Second, true, []HTTPTarget{
		{ServiceKey: "a", URL: a.URL},
		{ServiceKey: "b", URL: b.URL},
	})
	if len(results) != 2 {
		t.Fatalf("len = %d", len(results))
	}
	if !results[0].OK || results[0].Target.ServiceKey != "a" {
		t.Fatalf("first = %+v", results[0])
	}
	if results[1].OK || results[1].Target.ServiceKey != "b" {
		t.Fatalf("second = %+v", results[1])
	}

	ss := results[1].ServiceState(180, 0)
	if ss.Type != "virtual" || ss.State != "failed" || ss.TTLSeconds == nil || *ss.TTLSeconds != 180 {
		t.Fatalf("service state = %+v", ss)
	}
	if ss.Metadata["url"] != results[1].Target.URL || ss.Metadata["path"] != results[1].Target.URL {
		t.Fatalf("probe metadata = %#v", ss.Metadata)
	}
}
