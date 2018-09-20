package ctlogacquisition

import (
	"encoding/base64"
	"fmt"
	"strings"

	ct "github.com/google/certificate-transparency-go"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/certificate-transparency-go/x509"
	"golang.org/x/net/publicsuffix"
)

var stripLeading = []string{
	"*.",
	"[",
	"cn=",
	"san=",
	"dns=",
	"dns name=",
	"name=",
	"=",
	"-",
	"?",
	".",
}

var stripTrailing = []string{
	".",
	"]",
	"?",
	"#",
	"\\",
	"\"",
}

// cleanAndValidateHostname sanity check hostname values
func cleanAndValidateHostname(name string) bool {

	// Attempt to salvage names with certain prefixes and suffixes
	name = strings.ToLower(name)
	for _, item := range stripLeading {
		name = strings.TrimPrefix(name, item)
	}

	for _, item := range stripTrailing {
		name = strings.TrimSuffix(name, item)
	}

	name = strings.Replace(name, "..", ".", -1)
	name = strings.TrimSpace(name)

	if name == "" || strings.Contains(name, " ") || strings.Contains(name, ":") {
		return false
	}

	// The following check alone should be sufficient, but by pre-filtering the
	// above we can log more interesting potential hostnames.
	if _, err := publicsuffix.EffectiveTLDPlusOne(name); err != nil {
		return false
	}

	return true
}

// getDomainFromLeaf read the base64 encoded leaf_entry coming from CT log server and decode+extract CN and SNA from it
func getDomainFromLeaf(leafentrystr string) ([]string, error) {
	leafentry, err := base64.StdEncoding.DecodeString(leafentrystr)
	if err != nil {
		return nil, err
	}
	var leaf ct.MerkleTreeLeaf
	rest, err := tls.Unmarshal(leafentry, &leaf)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal MerkleTreeLeaf: %v", err)
	}
	if len(rest) > 0 {
		return nil, fmt.Errorf("trailing data (%d bytes) after MerkleTreeLeaf", len(rest))
	}

	x509cert, err := leaf.X509Certificate()
	if err != nil {
		_, notfatal := err.(x509.NonFatalErrors)
		if !notfatal {
			return nil, err
		}
	}
	var domainlist []string
	for _, name := range x509cert.DNSNames {
		if cleanAndValidateHostname(name) {
			domainlist = append(domainlist, name)
		}
	}
	if cleanAndValidateHostname(x509cert.Subject.CommonName) {
		domainlist = append(domainlist, x509cert.Subject.CommonName)
	}
	return domainlist, nil
}
