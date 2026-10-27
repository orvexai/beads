package dolt

import (
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/doltserver"
)

func TestProbeConnectRetriesWithJitteredBackoff(t *testing.T) {
	unsetConnectProbeTimeout(t)
	var attempts int
	var timeouts []time.Duration
	var sleeps []time.Duration
	randomValues := []float64{0, 1}
	var randomCalls int
	dial := func(_, _ string, timeout time.Duration) (net.Conn, error) {
		attempts++
		timeouts = append(timeouts, timeout)
		if attempts < 3 {
			return nil, errors.New("dial failed")
		}
		client, server := net.Pipe()
		go server.Close()
		return client, nil
	}
	conn, gotAttempts, err := probeConnectWith(
		"tcp", "db.example.invalid:3307", "db.example.invalid", dial,
		func(delay time.Duration) { sleeps = append(sleeps, delay) },
		func() float64 {
			value := randomValues[randomCalls]
			randomCalls++
			return value
		},
		newTestConnectProbePolicy(),
	)
	if err != nil {
		t.Fatalf("probeConnectWith() error = %v", err)
	}
	conn.Close()
	if gotAttempts != 3 || attempts != 3 {
		t.Fatalf("attempts = %d, dial calls = %d; want 3", gotAttempts, attempts)
	}
	for _, timeout := range timeouts {
		if timeout != defaultConnectProbeTimeout {
			t.Errorf("dial timeout = %s, want %s", timeout, defaultConnectProbeTimeout)
		}
	}
	wantSleeps := []time.Duration{125 * time.Millisecond, 1125 * time.Millisecond}
	if len(sleeps) != len(wantSleeps) {
		t.Fatalf("sleeps = %v, want %v", sleeps, wantSleeps)
	}
	for i := range wantSleeps {
		if sleeps[i] != wantSleeps[i] {
			t.Errorf("sleep %d = %s, want %s", i+1, sleeps[i], wantSleeps[i])
		}
	}
}

func TestProbeConnectExhaustedErrorIncludesAttempts(t *testing.T) {
	unsetConnectProbeTimeout(t)
	var attempts int
	conn, gotAttempts, err := probeConnectWith(
		"tcp", "db.example.invalid:3307", "db.example.invalid",
		func(_, _ string, _ time.Duration) (net.Conn, error) {
			attempts++
			return nil, errors.New("dial failed")
		}, func(time.Duration) {}, func() float64 { return 0 }, newTestConnectProbePolicy(),
	)
	if conn != nil {
		t.Fatal("probeConnectWith() returned a connection after dial failures")
	}
	if err == nil {
		t.Fatal("probeConnectWith() error = nil, want dial failure")
	}
	formattedErr := connectProbeError("db.example.invalid:3307", gotAttempts, err)
	if !strings.Contains(formattedErr.Error(), "Dolt server unreachable at db.example.invalid:3307") || !strings.Contains(formattedErr.Error(), "after 3 attempts") {
		t.Fatalf("formatted error = %v, want unreachable address and attempt count", formattedErr)
	}
	if gotAttempts != 3 || attempts != 3 {
		t.Fatalf("attempts = %d, dial calls = %d; want 3", gotAttempts, attempts)
	}
}

func TestProbeConnectLocalTargetsUseOne500MillisecondAttempt(t *testing.T) {
	t.Setenv(connectProbeTimeoutEnv, "7s")
	cases := []struct {
		name, network, address, host string
	}{
		{name: "localhost", network: "tcp", address: "localhost:3307", host: "localhost"},
		{name: "loopback IPv4", network: "tcp", address: "127.42.0.1:3307", host: "127.42.0.1"},
		{name: "loopback IPv6", network: "tcp", address: "[::1]:3307", host: "::1"},
		{name: "unix socket", network: "unix", address: "/tmp/beads-test.sock", host: "db.example.invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var attempts int
			_, gotAttempts, err := probeConnectWith(
				tc.network, tc.address, tc.host,
				func(_, _ string, timeout time.Duration) (net.Conn, error) {
					attempts++
					if timeout != localConnectProbeTimeout {
						t.Errorf("dial timeout = %s, want %s", timeout, localConnectProbeTimeout)
					}
					return nil, errors.New("dial failed")
				},
				func(time.Duration) { t.Error("local probe slept before retry") },
				func() float64 { t.Error("local probe requested jitter"); return 0 },
				newTestConnectProbePolicy(),
			)
			if err == nil {
				t.Fatal("probeConnectWith() error = nil, want dial failure")
			}
			if gotAttempts != 1 || attempts != 1 {
				t.Fatalf("attempts = %d, dial calls = %d; want 1", gotAttempts, attempts)
			}
		})
	}
}

func TestProbeConnectEnvironmentTimeoutAndInvalidWarning(t *testing.T) {
	t.Run("valid override", func(t *testing.T) {
		t.Setenv(connectProbeTimeoutEnv, "7s")
		var gotTimeout time.Duration
		client, server := net.Pipe()
		defer server.Close()
		conn, attempts, err := probeConnectWith(
			"tcp", "db.example.invalid:3307", "db.example.invalid",
			func(_, _ string, timeout time.Duration) (net.Conn, error) {
				gotTimeout = timeout
				return client, nil
			}, func(time.Duration) { t.Error("successful probe slept") },
			func() float64 { t.Error("successful probe requested jitter"); return 0 },
			newTestConnectProbePolicy(),
		)
		if err != nil {
			t.Fatalf("probeConnectWith() error = %v", err)
		}
		conn.Close()
		if attempts != 1 || gotTimeout != 7*time.Second {
			t.Fatalf("attempts = %d, timeout = %s; want 1 and 7s", attempts, gotTimeout)
		}
	})

	t.Run("maximum override", func(t *testing.T) {
		t.Setenv(connectProbeTimeoutEnv, "60s")
		if got := newTestConnectProbePolicy().timeout("tcp", "db.example.invalid"); got != maxConnectProbeTimeout {
			t.Fatalf("timeout = %s, want %s", got, maxConnectProbeTimeout)
		}
	})

	t.Run("invalid values warn once and use default", func(t *testing.T) {
		warnings := 0
		policy := &connectProbePolicy{warnInvalidTimeout: func() { warnings++ }}
		for _, value := range []string{"abc", "0", "90s"} {
			t.Setenv(connectProbeTimeoutEnv, value)
			if got := policy.timeout("tcp", "db.example.invalid"); got != defaultConnectProbeTimeout {
				t.Errorf("timeout for %q = %s, want %s", value, got, defaultConnectProbeTimeout)
			}
		}
		if warnings != 1 {
			t.Fatalf("warnings = %d, want 1", warnings)
		}
	})
}

func TestProbeConnectRealLocalhostSuccessDrainsAndCloses(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverDone <- acceptErr
			return
		}
		defer conn.Close()
		if _, writeErr := conn.Write([]byte{1}); writeErr != nil {
			serverDone <- writeErr
			return
		}
		_, copyErr := io.Copy(io.Discard, conn)
		serverDone <- copyErr
	}()
	conn, attempts, err := probeConnectWith(
		"tcp", listener.Addr().String(), "localhost", net.DialTimeout,
		func(time.Duration) { t.Error("successful local probe slept") },
		func() float64 { t.Error("successful local probe requested jitter"); return 0 },
		newTestConnectProbePolicy(),
	)
	if err != nil {
		t.Fatalf("probeConnectWith() error = %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if !doltserver.DrainAndCloseProbe(conn) {
		t.Fatal("DrainAndCloseProbe() did not observe the listener greeting")
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("listener handler error = %v", err)
	}
}

func newTestConnectProbePolicy() *connectProbePolicy {
	return &connectProbePolicy{warnInvalidTimeout: func() {}}
}

func unsetConnectProbeTimeout(t *testing.T) {
	t.Helper()
	value, wasSet := os.LookupEnv(connectProbeTimeoutEnv)
	if err := os.Unsetenv(connectProbeTimeoutEnv); err != nil {
		t.Fatalf("os.Unsetenv() error = %v", err)
	}
	t.Cleanup(func() {
		var err error
		if wasSet {
			err = os.Setenv(connectProbeTimeoutEnv, value)
		} else {
			err = os.Unsetenv(connectProbeTimeoutEnv)
		}
		if err != nil {
			t.Errorf("restore %s: %v", connectProbeTimeoutEnv, err)
		}
	})
}
