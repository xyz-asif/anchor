package anchors

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchURLMetadata_BlocksNonPublicAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>secret</title></head></html>"))
	}))
	defer srv.Close()

	for _, u := range []string{
		srv.URL,                    // loopback
		"http://169.254.169.254/",  // cloud metadata
		"http://10.0.0.1/",         // private
		"file:///etc/passwd",       // non-http scheme
		"gopher://127.0.0.1:6379/", // non-http scheme
	} {
		if _, err := FetchURLMetadata(context.Background(), u); err == nil {
			t.Errorf("expected %q to be rejected", u)
		}
	}
}

func TestIsPublicIP(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":            true,
		"127.0.0.1":          false,
		"10.1.2.3":           false,
		"192.168.1.1":        false,
		"169.254.169.254":    false,
		"::1":                false,
		"fd00::1":            false,
		"0.0.0.0":            false,
		"0.1.2.3":            false,
		"100.100.100.200":    false,
		"64:ff9b::a9fe:a9fe": false,
		"2002:a9fe:a9fe::1":  false,
		"::ffff:127.0.0.1":   false,
		"1.1.1.1":            true,
		"2606:4700::1111":    true,
	}
	for ip, want := range cases {
		if got := isPublicIP(net.ParseIP(ip)); got != want {
			t.Errorf("isPublicIP(%s) = %v, want %v", ip, got, want)
		}
	}
}
