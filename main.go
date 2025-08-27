package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed static
var staticFiles embed.FS

type GPSPosition struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timestamp int64   `json:"timestamp"`
}

type Config struct {
	MapStyle string `json:"mapStyle"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var (
	connections = make(map[*websocket.Conn]bool)
	connMutex   sync.RWMutex
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "localhost"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal("Failed to create static filesystem:", err)
	}

	http.Handle("/", http.FileServer(http.FS(staticFS)))
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/api/config", handleConfig)
	http.HandleFunc("/position", handlePosition)

	serverAddr := fmt.Sprintf("%s:%s", addr, port)
	log.Printf("Server starting on http://%s", serverAddr)
	log.Fatal(http.ListenAndServe(serverAddr, nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer func() {
		conn.Close()
		connMutex.Lock()
		delete(connections, conn)
		connMutex.Unlock()
	}()

	log.Printf("Client connected: %s", conn.RemoteAddr())

	connMutex.Lock()
	connections[conn] = true
	connMutex.Unlock()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read failed: %v", err)
			break
		}
	}
}

func handlePosition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var position GPSPosition
	if err := json.NewDecoder(r.Body).Decode(&position); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	position.Timestamp = time.Now().Unix()

	broadcastPosition(position)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	log.Printf("Received position: lat=%.6f, lng=%.6f", position.Latitude, position.Longitude)
}

func broadcastPosition(position GPSPosition) {
	connMutex.RLock()
	defer connMutex.RUnlock()

	for conn := range connections {
		if err := conn.WriteJSON(position); err != nil {
			log.Printf("WebSocket write failed: %v", err)
			conn.Close()
			delete(connections, conn)
		}
	}
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	mapStyle := os.Getenv("MAP_STYLE")
	if mapStyle == "" {
		mapStyle = "https://vector.openstreetmap.org/shortbread_v1/tilejson.json"
	}

	config := Config{
		MapStyle: mapStyle,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}
