// Программа проверочного контура: тестовый upstream для маршрутов Caddy и
// клиент, который проверяет матрицу разрешённых и запрещённых обращений.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

const (
	websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	controlFrame  = "control-frame"
)

var (
	requestCount   atomic.Int64
	websocketCount atomic.Int64
)

type echoResponse struct {
	Service   string `json:"service"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Body      string `json:"body"`
	RequestID string `json:"request_id"`
}

func main() {
	verify := flag.Bool("verify", false, "run the routing matrix against Caddy")
	caPath := flag.String("ca", envOr("CA_PATH", "/caddy-data/pki/authorities/local/root.crt"), "path to the Caddy root CA")
	caddyAddr := flag.String("caddy", envOr("CADDY_ADDR", "caddy:443"), "Caddy address inside the compose network")
	flag.Parse()

	if *verify {
		if err := verifyRouting(*caPath, *caddyAddr); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %v\n", err)
			os.Exit(1)
		}
		fmt.Println("PASS routing matrix")
		return
	}

	if err := serve(); err != nil {
		fmt.Fprintf(os.Stderr, "mock server: %v\n", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// --- тестовый upstream ---

func serve() error {
	service := envOr("SERVICE", "unknown")
	port := envOr("PORT", "8081")

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]int64{
			"requests":   requestCount.Load(),
			"websockets": websocketCount.Load(),
		})
	})
	mux.HandleFunc("/ws", handleWebSocket)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		body, _ := io.ReadAll(r.Body)

		w.Header().Set("X-Request-Id", r.Header.Get("X-Request-Id"))
		writeJSON(w, echoResponse{
			Service:   service,
			Method:    r.Method,
			Path:      r.URL.Path,
			Body:      string(body),
			RequestID: r.Header.Get("X-Request-Id"),
		})
	})

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.NotFound(w, r)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking is not supported", http.StatusInternalServerError)
		return
	}
	connection, buffered, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer connection.Close()

	accept := websocketAccept(r.Header.Get("Sec-WebSocket-Key"))
	fmt.Fprintf(connection, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)

	websocketCount.Add(1)
	requestCount.Add(1)

	_, payload, err := readFrame(buffered.Reader)
	if err != nil {
		return
	}
	_ = writeFrame(connection, payload)
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func readFrame(reader *bufio.Reader) (byte, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return 0, nil, err
	}

	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := int(header[1] & 0x7f)

	switch length {
	case 126:
		extended := make([]byte, 2)
		if _, err := io.ReadFull(reader, extended); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint16(extended))
	case 127:
		extended := make([]byte, 8)
		if _, err := io.ReadFull(reader, extended); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint64(extended))
	}

	var mask []byte
	if masked {
		mask = make([]byte, 4)
		if _, err := io.ReadFull(reader, mask); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for index := range payload {
			payload[index] ^= mask[index%4]
		}
	}
	return opcode, payload, nil
}

func writeFrame(writer io.Writer, payload []byte) error {
	frame := []byte{0x81}
	length := len(payload)
	switch {
	case length < 126:
		frame = append(frame, byte(length))
	case length <= 65535:
		frame = append(frame, 126, byte(length>>8), byte(length))
	default:
		frame = append(frame, 127, 0, 0, 0, 0, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	}
	frame = append(frame, payload...)
	_, err := writer.Write(frame)
	return err
}

// --- проверка маршрутов ---

var servicePorts = map[string]string{
	"auth":     "8081",
	"chat":     "8082",
	"media":    "8083",
	"presence": "8084",
	"gateway":  "8085",
}

var allowedRoutes = map[string]struct {
	serviceName string
	methods     []string
}{
	"auth":     {"AuthService", []string{"Register", "Login", "RefreshToken", "Logout", "GetMe", "UpdateProfile", "ChangePassword", "SearchUsers"}},
	"chat":     {"ChatService", []string{"ListChats", "GetChat", "CreateDirectChat", "CreateGroupChat", "AddGroupMember", "RemoveGroupMember", "LeaveGroup", "SetGroupMemberRole", "SendMessage", "EditMessage", "DeleteMessage", "GetMessages", "ConfirmDelivery", "MarkRead", "GetMessageReceipts", "GetReadStates"}},
	"media":    {"MediaService", []string{"PrepareUpload", "CompleteUpload", "GetAttachment", "GetDownloadURL"}},
	"presence": {"PresenceService", []string{"GetPresence"}},
}

func verifyRouting(caPath, caddyAddr string) error {
	pool, err := loadCA(caPath)
	if err != nil {
		return err
	}

	client := newTLSClient(pool, caddyAddr)
	if err := waitForCaddy(client); err != nil {
		return err
	}

	if err := checkAllowed(client); err != nil {
		return err
	}
	if err := checkForbidden(client); err != nil {
		return err
	}
	if err := checkSPA(client); err != nil {
		return err
	}
	return checkWebSocket(pool, caddyAddr)
}

func loadCA(path string) (*x509.CertPool, error) {
	deadline := time.Now().Add(90 * time.Second)
	for {
		pem, err := os.ReadFile(path)
		if err == nil {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, errors.New("CA file contains no certificates")
			}
			return pool, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("read CA: %w", err)
		}
		time.Sleep(time.Second)
	}
}

func newTLSClient(pool *x509.CertPool, caddyAddr string) *http.Client {
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, caddyAddr)
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:           nil,
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS12},
			DialContext:     dial,
		},
		Timeout: 15 * time.Second,
	}
}

func waitForCaddy(client *http.Client) error {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get("https://localhost/caddy-healthz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return errors.New("caddy did not become ready")
}

func checkAllowed(client *http.Client) error {
	counter := 0
	for service, route := range allowedRoutes {
		for _, method := range route.methods {
			counter++
			path := fmt.Sprintf("/molotalk.v1.%s/%s", route.serviceName, method)
			requestID := fmt.Sprintf("check-%d", counter)
			body := fmt.Sprintf("payload-%d", counter)

			request, err := http.NewRequest(http.MethodPost, "https://localhost"+path, strings.NewReader(body))
			if err != nil {
				return err
			}
			request.Header.Set("X-Request-Id", requestID)
			request.Header.Set("Content-Type", "text/plain")

			response, err := client.Do(request)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			var echoed echoResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&echoed)
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("%s: status %d", path, response.StatusCode)
			}
			if decodeErr != nil {
				return fmt.Errorf("%s: decode: %w", path, decodeErr)
			}
			if echoed.Service != service {
				return fmt.Errorf("%s: reached %q, want %q", path, echoed.Service, service)
			}
			if echoed.Path != path {
				return fmt.Errorf("%s: upstream path %q", path, echoed.Path)
			}
			if echoed.Body != body {
				return fmt.Errorf("%s: upstream body %q, want %q", path, echoed.Body, body)
			}
			if echoed.RequestID != requestID {
				return fmt.Errorf("%s: upstream request id %q, want %q", path, echoed.RequestID, requestID)
			}
		}
	}
	fmt.Printf("PASS allowed routes: %d methods\n", counter)
	return nil
}

var forbiddenPaths = []string{
	"/molotalk.v1.AuthService/IntrospectSession",
	"/molotalk.v1.ChatService/CheckChatAccess",
	"/molotalk.v1.ChatService/GetUserChatIds",
	"/molotalk.v1.PresenceService/Heartbeat",
	"/molotalk.v1.PresenceService/Disconnect",
	"/molotalk.v1.AuthService/UnknownMethod",
	"/molotalk.v1.AuthService/LoginExtra",
	"/molotalk.v1.AuthService/Login/extra",
	"/ws/extra",
}

func checkForbidden(client *http.Client) error {
	before, err := allStats()
	if err != nil {
		return err
	}

	for _, path := range forbiddenPaths {
		request, err := http.NewRequest(http.MethodPost, "https://localhost"+path, strings.NewReader("forbidden"))
		if err != nil {
			return err
		}
		request.Header.Set("Upgrade", "websocket")
		request.Header.Set("Connection", "Upgrade")
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			return fmt.Errorf("%s: status %d, want 404", path, response.StatusCode)
		}
	}

	after, err := allStats()
	if err != nil {
		return err
	}
	for service, count := range before {
		if after[service] != count {
			return fmt.Errorf("forbidden request reached %s: %d -> %d", service, count, after[service])
		}
	}
	fmt.Printf("PASS forbidden routes: %d paths stayed outside upstreams\n", len(forbiddenPaths))
	return nil
}

type counters struct {
	Requests   int64 `json:"requests"`
	Websockets int64 `json:"websockets"`
}

func serviceStats(service string) (counters, error) {
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	response, err := client.Get("http://" + service + ":" + servicePorts[service] + "/stats")
	if err != nil {
		return counters{}, fmt.Errorf("stats %s: %w", service, err)
	}
	defer response.Body.Close()
	var value counters
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return counters{}, err
	}
	return value, nil
}

func allStats() (map[string]int64, error) {
	result := make(map[string]int64, len(servicePorts))
	for service := range servicePorts {
		value, err := serviceStats(service)
		if err != nil {
			return nil, err
		}
		result[service] = value.Requests
	}
	return result, nil
}

type spaResponse struct {
	status      int
	contentType string
	body        string
}

func fetchSPA(client *http.Client, path string) (spaResponse, error) {
	response, err := client.Get("https://localhost" + path)
	if err != nil {
		return spaResponse{}, fmt.Errorf("%s: %w", path, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return spaResponse{}, fmt.Errorf("%s: read: %w", path, err)
	}
	return spaResponse{
		status:      response.StatusCode,
		contentType: response.Header.Get("Content-Type"),
		body:        string(body),
	}, nil
}

// checkSPA проверяет выдачу сборки клиента и отсутствие подмены API.
func checkSPA(client *http.Client) error {
	htmlRoutes := []string{"/", "/login", "/chats/01ARZ3NDEKTSV4RRFFQ69G5FA4"}

	for _, path := range htmlRoutes {
		response, err := fetchSPA(client, path)
		if err != nil {
			return err
		}
		if response.status != http.StatusOK {
			return fmt.Errorf("%s: status %d, want 200", path, response.status)
		}
		if !strings.Contains(response.contentType, "text/html") {
			return fmt.Errorf("%s: content type %q, want text/html", path, response.contentType)
		}
		if !strings.Contains(response.body, "spa-shell") {
			return fmt.Errorf("%s: body is not the SPA shell", path)
		}
	}

	asset, err := fetchSPA(client, "/assets/app.js")
	if err != nil {
		return err
	}
	if asset.status != http.StatusOK {
		return fmt.Errorf("/assets/app.js: status %d, want 200", asset.status)
	}
	if !strings.Contains(asset.contentType, "javascript") {
		return fmt.Errorf("/assets/app.js: content type %q, want javascript", asset.contentType)
	}
	if !strings.Contains(asset.body, "spa-asset") {
		return fmt.Errorf("/assets/app.js: unexpected body")
	}

	missing, err := fetchSPA(client, "/assets/missing.js")
	if err != nil {
		return err
	}
	if missing.status != http.StatusNotFound {
		return fmt.Errorf("/assets/missing.js: status %d, want 404", missing.status)
	}
	if strings.Contains(missing.body, "spa-shell") {
		return fmt.Errorf("/assets/missing.js: SPA fallback masked a missing asset")
	}

	fmt.Printf("PASS spa routes: %d html, 1 asset, 1 missing\n", len(htmlRoutes))
	return nil
}

func checkWebSocket(pool *x509.CertPool, caddyAddr string) error {
	before, err := serviceStats("gateway")
	if err != nil {
		return err
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	connection, err := tls.DialWithDialer(dialer, "tcp", caddyAddr, &tls.Config{
		RootCAs:    pool,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		return fmt.Errorf("dial websocket: %w", err)
	}
	defer connection.Close()

	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	request := fmt.Sprintf("GET /ws HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
		base64.StdEncoding.EncodeToString(key))
	if _, err := connection.Write([]byte(request)); err != nil {
		return err
	}

	reader := bufio.NewReader(connection)
	status, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.Contains(status, "101") {
		return fmt.Errorf("websocket upgrade failed: %s", strings.TrimSpace(status))
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if line == "\r\n" {
			break
		}
	}

	if err := writeMaskedFrame(connection, []byte(controlFrame)); err != nil {
		return err
	}
	_, payload, err := readFrame(reader)
	if err != nil {
		return err
	}
	if string(payload) != controlFrame {
		return fmt.Errorf("websocket echo %q, want %q", payload, controlFrame)
	}

	after, err := serviceStats("gateway")
	if err != nil {
		return err
	}
	if after.Websockets != before.Websockets+1 {
		return fmt.Errorf("gateway websocket counter %d -> %d", before.Websockets, after.Websockets)
	}
	fmt.Println("PASS websocket upgrade and bidirectional control frame")
	return nil
}

func writeMaskedFrame(writer io.Writer, payload []byte) error {
	mask := [4]byte{0x37, 0xfa, 0x21, 0x3d}
	frame := []byte{0x81, 0x80 | byte(len(payload))}
	frame = append(frame, mask[:]...)
	for index, value := range payload {
		frame = append(frame, value^mask[index%4])
	}
	_, err := writer.Write(frame)
	return err
}
