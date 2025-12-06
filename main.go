package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"os"
	"pastebin/main/certs"
	"pastebin/main/https"
	"pastebin/main/nets"
	"pastebin/main/ui"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/skip2/go-qrcode"
)

//go:embed assets/*
var staticFiles embed.FS

var certificateFilePath string

var keyFilePath string

var dataDir string

const certName = "cert.pem"

const keyName = "key.pem"

func init() {
	var err error
	dataDir, err = getUserDataPath()
	if err != nil {
		log.Fatalln("Impossible to use the app data directory")
	}
	err = os.MkdirAll(dataDir, 0755) // Read/write/execute for owner, read/execute for group and others
	if err != nil {
		log.Fatalln("Error creating directory:", err)
	}
	certificateFilePath = filepath.Join(dataDir, certName)
	keyFilePath = filepath.Join(dataDir, keyName)
}

func getUserDataPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "pastebin"), nil
}

func readVersion() (string, error) {
	subFS, _ := fs.Sub(staticFiles, "assets")
	versionFile, err := subFS.Open("version")
	var versionString string
	if err == nil {
		content, err := io.ReadAll(versionFile)
		if err == nil {
			versionString = string(content)
		}
	}
	return versionString, err
}

func main() {
	var port int
	var version bool

	// handling commandline options
	flag.IntVar(&port, "port", 8443, "server listening port")
	flag.BoolVar(&version, "version", false, "application release version")
	flag.Parse()

	if version {
		if flag.NFlag() > 1 {
			flag.Usage()
			log.Fatal("Syntax error")
		}
		versionString, err := readVersion()
		if err != nil {
			log.Fatal("Error reading file:", err)
		}
		fmt.Println("Application version:", versionString)
		os.Exit(0)
	}

	var ipAddress string
	// select the network interface/listening address
	interfaces, err := nets.GetRealInterfaces()
	if err != nil || interfaces == nil || len(interfaces) == 0 {
		log.Fatalf("No network interface found: %v", err)
	}
	fmt.Println("Network interfaces found:")
	for _, intf := range interfaces {
		fmt.Println(intf.String())
	}
	fmt.Println("Desired port: ", port)

	//asking user input for network interface selection
	fmt.Println("")
	if len(interfaces) > 1 {
		strs := make([]fmt.Stringer, len(interfaces))
		for i, in := range interfaces {
			strs[i] = in
		}
		p := tea.NewProgram(ui.Model{Items: strs})
		finalModel, err := p.Run()
		if err != nil {
			log.Fatalf("Unrecoverable error handling user input: %v", err)
		}
		m := finalModel.(ui.Model)

		ipAddress = interfaces[m.Selected].IP.String()
	} else {
		ipAddress = interfaces[0].IP.String()
	}
	allIPs := make([]net.IP, len(interfaces)+1)
	for i, in := range interfaces {
		allIPs[i] = in.IP
	}
	allIPs[len(interfaces)] = net.ParseIP("127.0.0.1")

	certs.InitCert(certificateFilePath, keyFilePath, allIPs, ipAddress)

	fmt.Println("Server ip: ", ipAddress)
	qr, err := qrcode.New(fmt.Sprintf("https://%v:%d/", ipAddress, port), qrcode.Low)
	if err != nil {
		log.Fatalf("Error generating QR code: %v", err)
	}

	fmt.Printf("\nLocal server url: https://localhost:%d/\n", port)

	fmt.Printf("\nServer url for remote clients(QR code below): https://%v:%d/\n", ipAddress, port)

	// Print QR as ASCII
	fmt.Println(qr.ToSmallString(false)) // false = white background, true = black background

	https.StartServer(ipAddress, port, &staticFiles, certificateFilePath, keyFilePath)
}
