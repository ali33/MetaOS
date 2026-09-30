// Package server là metaos-ws: HTTPS, phiên, đăng nhập và chuyển tiếp WebSocket ↔ bridge.
package server

import (
	"errors"
	"fmt"
	"net"
	"time"
)

type Config struct {
	Listen       string
	TLSCert      string
	TLSKey       string
	AuthPath     string
	AllowRoot    bool
	SessionMax   time.Duration
	SessionIdle  time.Duration
	HelloTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{
		Listen:       "127.0.0.1:9090",
		AuthPath:     "/usr/lib/metaos/metaos-auth",
		SessionMax:   12 * time.Hour,
		SessionIdle:  30 * time.Minute,
		HelloTimeout: 15 * time.Second,
	}
}

func (c Config) TLS() bool { return c.TLSCert != "" }

func (c Config) Validate() error {
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return fmt.Errorf("listen %q: %v", c.Listen, err)
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("tls-cert và tls-key phải đi cùng nhau")
	}
	if !c.TLS() && host != "127.0.0.1" && host != "::1" && host != "localhost" {
		return fmt.Errorf("HTTP không TLS chỉ được lắng nghe 127.0.0.1/::1/localhost, không phải %q", host)
	}
	if c.SessionIdle <= 0 || c.SessionMax <= 0 || c.HelloTimeout <= 0 {
		return errors.New("session-idle, session-max, hello-timeout phải > 0")
	}
	if c.SessionMax < c.SessionIdle {
		return errors.New("session-max phải ≥ session-idle")
	}
	return nil
}
