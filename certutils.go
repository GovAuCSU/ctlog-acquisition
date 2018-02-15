package ctlogacquisition

import (
	"encoding/base64"
	"fmt"

	ct "github.com/google/certificate-transparency-go"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/certificate-transparency-go/x509"
	// Think about using standard library
)

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
	// return x509cert
	// Adding commonname as well. Forget duplication
	// count, _ := x509util.OIDInExtensions(x509.OIDExtensionSubjectAltName, x509cert.Extensions)
	// if count > 0 {
	var str []string
	for _, name := range x509cert.DNSNames {
		str = append(str, name)
	}
	return str, nil
}

// This will cause program to exit...
func paniciferr(err error) {
	if err != nil {
		panic(err)
	}
}
