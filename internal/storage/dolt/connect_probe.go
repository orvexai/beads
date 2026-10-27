package dolt

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	localConnectProbeTimeout   = 500 * time.Millisecond
	defaultConnectProbeTimeout = 3 * time.Second
	maxConnectProbeTimeout     = 60 * time.Second
	connectProbeTimeoutEnv     = "BEADS_DOLT_CONNECT_TIMEOUT"
	maxConnectProbeAttempts    = 3
)

var connectProbeBackoffBase = [...]time.Duration{250 * time.Millisecond, 750 * time.Millisecond}

type connectProbeDialFunc func(network, address string, timeout time.Duration) (net.Conn, error)
type connectProbeSleepFunc func(time.Duration)
type connectProbeRandomFunc func() float64

type connectProbePolicy struct {
	invalidTimeoutWarning sync.Once
	warnInvalidTimeout    func()
}

var defaultConnectProbePolicy = &connectProbePolicy{
	warnInvalidTimeout: func() {
		fmt.Fprintf(os.Stderr, "Warning: invalid %s; using the default %s timeout\n", connectProbeTimeoutEnv, defaultConnectProbeTimeout)
	},
}

func probeConnect(cfg *Config) (net.Conn, int, error) {
	network := "tcp"
	host := cfg.ServerHost
	address := net.JoinHostPort(host, fmt.Sprintf("%d", cfg.ServerPort))
	if cfg.ServerSocket != "" {
		network = "unix"
		address = cfg.ServerSocket
	}
	return probeConnectWith(network, address, host, net.DialTimeout, time.Sleep, rand.Float64, defaultConnectProbePolicy)
}

func probeConnectWith(
	network, address, host string,
	dial connectProbeDialFunc,
	sleep connectProbeSleepFunc,
	random connectProbeRandomFunc,
	policy *connectProbePolicy,
) (net.Conn, int, error) {
	timeout := policy.timeout(network, host)
	maxAttempts := 1
	if network == "tcp" && !isLocalConnectProbeHost(host) {
		maxAttempts = maxConnectProbeAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		conn, err := dial(network, address, timeout)
		if err == nil {
			return conn, attempt, nil
		}
		lastErr = err
		if attempt < maxAttempts {
			sleep(jitteredConnectProbeBackoff(connectProbeBackoffBase[attempt-1], random()))
		}
	}
	return nil, maxAttempts, lastErr
}

func (p *connectProbePolicy) timeout(network, host string) time.Duration {
	if network != "tcp" || isLocalConnectProbeHost(host) {
		return localConnectProbeTimeout
	}

	value, set := os.LookupEnv(connectProbeTimeoutEnv)
	if !set {
		return defaultConnectProbeTimeout
	}
	parsed, err := time.ParseDuration(value)
	if err == nil && parsed > 0 && parsed <= maxConnectProbeTimeout {
		return parsed
	}
	p.invalidTimeoutWarning.Do(p.warnInvalidTimeout)
	return defaultConnectProbeTimeout
}

func isLocalConnectProbeHost(host string) bool {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func jitteredConnectProbeBackoff(base time.Duration, random float64) time.Duration {
	if random < 0 {
		random = 0
	} else if random > 1 {
		random = 1
	}
	return base/2 + time.Duration(float64(base)*random)
}

func connectProbeError(address string, attempts int, err error) error {
	return fmt.Errorf("Dolt server unreachable at %s (after %d attempts): %w", address, attempts, err)
}
