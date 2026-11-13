// Fake Redfish BMC server for local Tilt development.
// Serves GET /redfish/v1 over HTTPS (self-signed cert) with configurable
// ServiceTag and ManagerType, allowing the moo controller to exercise the
// serial-first lookup path without real hardware.
//
// Configuration via environment variables:
//
//	REDFISH_SERVICE_TAG  — Oem.Dell.ServiceTag value (default: "FAKETAG1")
//	REDFISH_MANAGER_TYPE — ManagerType value       (default: "BMC")
//	REDFISH_UUID         — UUID fallback value      (default: "00000000-0000-0000-0000-000000000001")
//	REDFISH_PORT         — HTTPS listen port        (default: "443")
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	serviceTag := env("REDFISH_SERVICE_TAG", "FAKETAG1")
	managerType := env("REDFISH_MANAGER_TYPE", "BMC")
	uuid := env("REDFISH_UUID", "00000000-0000-0000-0000-000000000001")
	port := env("REDFISH_PORT", "8443")

	tlsCfg, err := selfSignedTLS()
	if err != nil {
		log.Fatalf("generate TLS cert: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/redfish/v1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ManagerType": managerType,
			"UUID":        uuid,
			"Oem": map[string]any{
				"Dell": map[string]any{
					"ServiceTag": serviceTag,
				},
			},
		})
	})

	srv := &http.Server{
		Addr:      ":" + port,
		Handler:   mux,
		TLSConfig: tlsCfg,
	}

	log.Printf("fake-redfish listening on :%s  ServiceTag=%s ManagerType=%s", port, serviceTag, managerType)
	if err := srv.ListenAndServeTLS("", ""); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// selfSignedTLS generates an in-memory ECDSA self-signed certificate valid for 10 years.
func selfSignedTLS() (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "fake-redfish"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("key pair: %w", err)
	}

	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil //nolint:gosec
}
