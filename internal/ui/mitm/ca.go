package mitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	caCertFile      = "tracto-ca.crt"
	caKeyFile       = "tracto-ca.key"
	caCommonName    = "Tracto MITM Root CA"
	caOrganization  = "Tracto"
	caValidity      = 10 * 365 * 24 * time.Hour
	leafValidity    = 365 * 24 * time.Hour
	leafCacheLimit  = 256
)

type CA struct {
	Cert    *x509.Certificate
	Key     *rsa.PrivateKey
	CertPEM []byte
	KeyPEM  []byte

	mu     sync.Mutex
	leaves map[string]*tls.Certificate
}

func CACertPath(dir string) string { return filepath.Join(dir, caCertFile) }
func CAKeyPath(dir string) string  { return filepath.Join(dir, caKeyFile) }

// LoadCA reads an existing CA from dir. Returns os.ErrNotExist if either
// the certificate or key file is missing.
func LoadCA(dir string) (*CA, error) {
	certPEM, err := os.ReadFile(CACertPath(dir))
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(CAKeyPath(dir))
	if err != nil {
		return nil, err
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, errors.New("ca: invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ca: parse cert: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errors.New("ca: invalid key PEM")
	}
	var key *rsa.PrivateKey
	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	case "PRIVATE KEY":
		k, e := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if e != nil {
			return nil, fmt.Errorf("ca: parse pkcs8: %w", e)
		}
		var ok bool
		if key, ok = k.(*rsa.PrivateKey); !ok {
			return nil, errors.New("ca: not an RSA key")
		}
	default:
		return nil, fmt.Errorf("ca: unsupported key block %q", keyBlock.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("ca: parse key: %w", err)
	}

	return &CA{
		Cert:    cert,
		Key:     key,
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		leaves:  make(map[string]*tls.Certificate),
	}, nil
}

// GenerateCA creates a fresh self-signed root certificate. It does not
// touch the filesystem; pair with Save to persist it.
func GenerateCA() (*CA, error) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, fmt.Errorf("ca: rsa keygen: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("ca: serial: %w", err)
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("ca: marshal public: %w", err)
	}
	skid := sha1.Sum(pubBytes)

	now := time.Now().Add(-1 * time.Minute)
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   caCommonName,
			Organization: []string{caOrganization},
		},
		NotBefore:             now,
		NotAfter:              now.Add(caValidity),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		SubjectKeyId:          skid[:],
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("ca: self-sign: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("ca: re-parse: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return &CA{
		Cert:    cert,
		Key:     key,
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		leaves:  make(map[string]*tls.Certificate),
	}, nil
}

// Save writes the CA's PEM-encoded certificate and key to dir using
// AtomicWriteFile-style temp+rename via os.WriteFile (good enough here —
// these files are written rarely and only by an explicit user action).
func (ca *CA) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(CACertPath(dir), ca.CertPEM, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(CAKeyPath(dir), ca.KeyPEM, 0o600); err != nil {
		return err
	}
	return nil
}

// Fingerprint returns a SHA-1 hex fingerprint of the CA cert (the same
// digest Windows shows in certmgr).
func (ca *CA) Fingerprint() string {
	if ca.Cert == nil {
		return ""
	}
	h := sha1.Sum(ca.Cert.Raw)
	parts := make([]string, len(h))
	for i, b := range h {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

// LeafFor returns a leaf certificate signed by this CA for the given
// hostname (SNI / CONNECT host), generating and caching it on first use.
// IP literals are supported via SAN IP entries.
func (ca *CA) LeafFor(host string) (*tls.Certificate, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return nil, errors.New("ca: empty host")
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	ca.mu.Lock()
	if cached, ok := ca.leaves[host]; ok {
		ca.mu.Unlock()
		return cached, nil
	}
	ca.mu.Unlock()

	leaf, err := ca.mintLeaf(host)
	if err != nil {
		return nil, err
	}

	ca.mu.Lock()
	if len(ca.leaves) >= leafCacheLimit {
		// Trivial eviction: drop the cache when full. Simpler than LRU and
		// fine for a developer tool that rarely exceeds a few dozen hosts.
		ca.leaves = make(map[string]*tls.Certificate, leafCacheLimit)
	}
	ca.leaves[host] = leaf
	ca.mu.Unlock()
	return leaf, nil
}

func (ca *CA) mintLeaf(host string) (*tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("ca: leaf keygen: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("ca: leaf serial: %w", err)
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("ca: marshal leaf pub: %w", err)
	}
	skid := sha1.Sum(pubBytes)

	now := time.Now().Add(-1 * time.Minute)
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: host,
		},
		NotBefore:             now,
		NotAfter:              now.Add(leafValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		// Browsers (especially Firefox/NSS) expect AKI on leaf certs to
		// chain explicitly to the issuer's SKI. CreateCertificate sets
		// AKI from parent.SubjectKeyId automatically, but stating it
		// here makes the intent explicit and survives any quirks.
		SubjectKeyId:   skid[:],
		AuthorityKeyId: ca.Cert.SubjectKeyId,
	}
	if ip := net.ParseIP(host); ip != nil {
		tpl.IPAddresses = []net.IP{ip}
	} else {
		tpl.DNSNames = []string{host}
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, ca.Cert, &key.PublicKey, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("ca: sign leaf: %w", err)
	}
	return &tls.Certificate{
		Certificate: [][]byte{der, ca.Cert.Raw},
		PrivateKey:  key,
		Leaf:        nil,
	}, nil
}
