package ctlogacquisition

import (
	"encoding/base64"
	"fmt"

	ct "github.com/google/certificate-transparency-go"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/certificate-transparency-go/x509"
)

// getDomainFromLeaf read the base64 encoded leaf_entry coming from CT log server and decode+extract CN and SNA from it
func getDomainFromLeaf(leafentrystr string) ([]string, error) {
	leafentry, err := base64.StdEncoding.DecodeString(leafentrystr)
	if err != nil {
		return nil, err
	}
	var leaf ct.MerkleTreeLeaf
	rest, err := tls.Unmarshal(leafentry, &leaf)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal MerkleTreeLeafv: %v", err)
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
		domainlist = append(domainlist, name)
	}
	domainlist = append(domainlist, x509cert.Subject.CommonName)
	return domainlist, nil
}
