package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"

	"slices"
	"strings"
	"time"
)

const certCN = "Pastebin Cert"

const certOrg = "eng.vitor"

var certName, keyName string

func verifyCert(ip string) (error, []net.IP) {
	certBytes, err := os.ReadFile(certName)
	if err == nil {
		_, err = os.Stat(keyName)
	}
	var cert *x509.Certificate
	if err == nil {
		block, _ := pem.Decode(certBytes)
		if block != nil && block.Type == "CERTIFICATE" {
			cert, err = x509.ParseCertificate(block.Bytes)
		} else {
			err = errors.New("failed to decode PEM block containing certificate")
		}
	}
	var certIps []net.IP
	if err == nil {
		now := time.Now()
		fmt.Println("\n---------------------")
		fmt.Println("cert.Subject.CommonName:      ", cert.Subject.CommonName)
		fmt.Println("cert.Subject.Organization[0]: ", cert.Subject.Organization[0])
		fmt.Println("cert.IPAddresses:             ", cert.IPAddresses)
		fmt.Println("cert.NotAfter:                ", cert.NotAfter)
		fmt.Printf("---------------------\n\n")
		if cert.Subject.CommonName == certCN && cert.Subject.Organization[0] == certOrg &&
			slices.ContainsFunc(cert.IPAddresses, func(i net.IP) bool { return i.String() == ip }) &&
			cert.NotBefore.Before(now) && cert.NotAfter.After(now) {
		} else {
			certIps = cert.IPAddresses
			err = errors.New("invalid cert data or cert expired/'still not valid' or wrong ip")
		}
	}
	return err, certIps
}

func crypto(calculatedIpv4, certIps []net.IP) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)

	allIp4 := calculatedIpv4
	slices.SortFunc(allIp4, func(x, y net.IP) int {
		return strings.Compare(x.String(), y.String())
	})
	for _, netIp := range certIps {
		if _, found := slices.BinarySearchFunc(allIp4, netIp, func(x, y net.IP) int {
			return strings.Compare(x.String(), y.String())
		}); !found {
			allIp4 = append(allIp4, netIp)
		}
	}

	// Set certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // 1 year

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	check(err)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   certCN,
			Organization: []string{certOrg},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           allIp4,
		DNSNames:              []string{"localhost"},
	}

	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	check(err)

	// Write certificate
	certOut, err := os.Create(certName)
	check(err)
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()
	fmt.Println(certName, " written")

	// Write private key
	keyOut, err := os.Create(keyName)
	check(err)
	b, err := x509.MarshalECPrivateKey(priv)
	check(err)
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	keyOut.Close()
	fmt.Println(keyName, " written")
}

func check(err error) {
	if err != nil {
		log.Fatalln("Unrecoverable error generating SSL certificate: ", err)
	}
}

func InitCert(_certName, _keyName string, calculatedIpv4 []net.IP, ipAddress string) {
	certName, keyName = _certName, _keyName
	err, certIps := verifyCert(ipAddress)
	if err != nil {
		fmt.Println("A new certificate has to be issued (", err, ")")
		crypto(calculatedIpv4, certIps)
	}
}
