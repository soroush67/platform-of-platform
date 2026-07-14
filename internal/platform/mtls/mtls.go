// Package mtls builds real mutual-TLS transport credentials for the
// Worker<->Control Plane gRPC channel (docs/architecture/17-workers.md's
// own "worker identity token" framing) - both sides present a
// certificate signed by the same dev CA (docker-compose.yml's
// mtls-init service) and verify the peer's certificate against it, not
// just encrypt the channel. Shared between cmd/control-plane and
// cmd/worker so the two sides can't drift on TLS version/cipher
// defaults.
package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc/credentials"
)

func loadCAPool(caCertPath string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("mtls: reading CA cert: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("mtls: %s did not contain a valid PEM certificate", caCertPath)
	}
	return pool, nil
}

// ServerCredentials requires and verifies every connecting Worker's
// client certificate against caCertPath - a Worker whose cert wasn't
// signed by this CA is rejected at the TLS handshake, before any gRPC
// call is even attempted.
func ServerCredentials(caCertPath, certPath, keyPath string) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("mtls: loading server keypair: %w", err)
	}
	pool, err := loadCAPool(caCertPath)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}), nil
}

// ClientCredentials presents the Worker's own certificate (so the
// server's RequireAndVerifyClientCert accepts the connection) and
// verifies the Control Plane's server certificate against the same CA -
// serverName must match a DNS SAN on the server cert (mtls-init
// generates "control-plane" for the compose network's DNS name).
func ClientCredentials(caCertPath, certPath, keyPath, serverName string) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("mtls: loading client keypair: %w", err)
	}
	pool, err := loadCAPool(caCertPath)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS12,
	}), nil
}
