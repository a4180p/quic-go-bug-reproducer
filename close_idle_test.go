package closeidlerepro

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func TestCloseIdleConnectionsDuringResponseBody(t *testing.T) {
	for _, cleanup := range []bool{false, true} {
		name := "without_cleanup"
		if cleanup {
			name = "with_cleanup"
		}
		t.Run(name, func(t *testing.T) {
			certSource := httptest.NewTLSServer(http.NotFoundHandler())
			defer certSource.Close()

			conn, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			releaseBody := make(chan struct{})
			server := &http3.Server{
				TLSConfig:   http3.ConfigureTLSConfig(certSource.TLS.Clone()),
				QUICConfig:  &quic.Config{MaxIdleTimeout: time.Hour},
				IdleTimeout: time.Hour,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Length", "11")
					if _, err := io.WriteString(w, "first"); err != nil {
						return
					}
					w.(http.Flusher).Flush()
					select {
					case <-releaseBody:
						_, _ = io.WriteString(w, "second")
					case <-r.Context().Done():
					}
				}),
			}
			serveDone := make(chan error, 1)
			go func() { serveDone <- server.Serve(conn) }()
			defer func() {
				_ = server.Close()
				_ = conn.Close()
				if err := <-serveDone; err != nil && !errors.Is(err, http.ErrServerClosed) {
					t.Errorf("HTTP/3 server: %v", err)
				}
			}()

			transport := &http3.Transport{
				TLSClientConfig: certSource.Client().Transport.(*http.Transport).TLSClientConfig.Clone(),
				QUICConfig:      &quic.Config{MaxIdleTimeout: time.Hour},
			}
			defer transport.Close()
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			response, err := client.Get("https://" + conn.LocalAddr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK || response.ProtoMajor != 3 {
				t.Fatalf("status=%d protocol=%s", response.StatusCode, response.Proto)
			}
			prefix := make([]byte, len("first"))
			if _, err := io.ReadFull(response.Body, prefix); err != nil {
				t.Fatalf("read response prefix: %v", err)
			}

			if cleanup {
				transport.CloseIdleConnections()
			}
			close(releaseBody)
			rest, err := io.ReadAll(response.Body)
			body := string(prefix) + string(rest)
			t.Logf("cleanup=%v status=%d body=%q error=%v", cleanup, response.StatusCode, body, err)
			if err != nil || body != "firstsecond" {
				t.Fatalf("active response interrupted: got %q, want %q; read error: %v", body, "firstsecond", err)
			}
		})
	}
}
