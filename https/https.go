package https

import (
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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

var subFS fs.FS

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
	tmpl := template.Must(template.ParseFS(subFS, "layout.html", "main.html"))
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
	tmpl := template.Must(template.ParseFS(subFS, "layout.html", "saved.html"))
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
	tmpl := template.Must(template.ParseFS(subFS, "layout.html", "view.html"))
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

func StartServer(_ipAddress string, _port int, staticFiles *embed.FS, certificateFilePath string, keyFilePath string) {
	subFS, _ = fs.Sub(*staticFiles, "assets")
	ipAddress = _ipAddress
	port = _port

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
