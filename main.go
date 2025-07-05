package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"pastebin/main/certs"
	"pastebin/main/nets"
	"pastebin/main/ui"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/skip2/go-qrcode"
)

type Page struct {
	IP         string
	Content    string
	ContentUrl string
	Qrcode     string
	Private    bool
	Home       string
}

type Data struct {
	Text    string `json:"text"`
	Private bool   `json:"private"`
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var contentById sync.Map

var ipAddress string

var port int

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

func homeUrl(clientHost string, remoteAddr string) string {
	log.Println("Supposed client host: ", remoteAddr)
	remoteAddrHostPart := strings.Split(remoteAddr, ":")[0]
	clientHostPart := strings.Split(clientHost, ":")[0]
	var serverHost string
	if clientHostPart == "localhost" || remoteAddrHostPart == ipAddress {
		serverHost = "localhost"
	} else {
		serverHost = ipAddress
	}
	return fmt.Sprintf("https://%v:%d/", serverHost, port)
}

func randomString(length int) string {
	rg := rand.New(rand.NewSource(time.Now().UnixNano()))
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rg.Intn(len(charset))]
	}
	return string(result)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("layout.html", "main.html"))
	p := &Page{IP: ipAddress}
	err := tmpl.ExecuteTemplate(w, "layout.html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func saveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, errors.New("only POST method is allowed for this endpoint").Error(), http.StatusMethodNotAllowed)
	}
	tmpl := template.Must(template.ParseFiles("layout.html", "saved.html"))
	err := r.ParseForm()
	var p *Page
	if err == nil {
		private, err := strconv.ParseBool(r.PostForm.Get("private"))
		if err != nil {
			private = true
		}
		contentId := randomString(4)
		data := Data{Text: r.PostForm.Get("content"), Private: private}
		for _, loaded := contentById.LoadOrStore(contentId, data); loaded; {
			contentId = randomString(4)
		}
		log.Printf("Remote host %s saved content %s", r.RemoteAddr, contentId)

		log.Printf("New text copied: https://localhost:%d/c#%v", port, contentId)
		curl := fmt.Sprintf("https://%v:%d/c#%v", ipAddress, port, contentId)
		qr, err := qrcode.New(curl, qrcode.Low)
		if err != nil {
			fmt.Println("Error generating QR code: ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		qrcode, err := qr.PNG(300)
		if err != nil {
			fmt.Println("Error generating QR code image: ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		encoded := base64.StdEncoding.EncodeToString(qrcode)
		p = &Page{Content: data.Text, ContentUrl: curl, Qrcode: encoded,
			Private: data.Private, Home: homeUrl(r.Host, r.RemoteAddr)}
	} else {
		fmt.Println("Error parsing form: ", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tmpl.ExecuteTemplate(w, "layout.html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func viewHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("layout.html", "view.html"))
	var p = &Page{
		Home: homeUrl(r.Host, r.RemoteAddr)}
	err := tmpl.ExecuteTemplate(w, "layout.html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getContentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, errors.New("only POST method is allowed for this endpoint").Error(), http.StatusMethodNotAllowed)
	}
	err := r.ParseForm()
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		contentKey := r.PostForm.Get("contentKey")
		log.Printf("Remote host %s asked for content %s", r.RemoteAddr, contentKey)
		response, found := contentById.Load(contentKey)
		if !found {
			http.Error(w, "content not found", http.StatusNotFound)
			return
		}
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func startServer(ipAddress string, port int) {
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/c", viewHandler)
	http.HandleFunc("/getContent", getContentHandler)
	http.HandleFunc("/save", saveHandler)
	http.HandleFunc("/content", getContentHandler)

	server := &http.Server{}
	// Load certificate
	cert, err := tls.LoadX509KeyPair(certificateFilePath, keyFilePath)
	if err != nil {
		log.Fatalf("failed to load cert: %v", err)
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}

	addresses := []string{fmt.Sprintf("127.0.0.1:%d", port), fmt.Sprintf("%s:%d", ipAddress, port)}

	for _, addr := range addresses {
		go func(addr string) {
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				log.Fatalf("failed to listen on %s: %v", addr, err)
			}
			tlsListener := tls.NewListener(ln, tlsConfig)
			fmt.Printf("Serving on https://%s\n", addr)
			if err := server.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server failed on %s: %v", addr, err)
			}
		}(addr)
	}
	select {}
}

func main() {
	// select the network interface/listening address
	interfaces, err := nets.GetRealInterfaces()
	if err != nil || interfaces == nil || len(interfaces) == 0 {
		log.Fatalf("No network interface found: %v", err)
	}
	fmt.Println("Network interfaces found:")
	for _, intf := range interfaces {
		fmt.Println(intf.String())
	}

	// handling commandline options
	flag.IntVar(&port, "port", 8443, "server listening port")
	flag.Parse()

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

	startServer(ipAddress, port)
}
