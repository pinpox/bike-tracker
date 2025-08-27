package main

import (
	"database/sql"
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
	_ "github.com/mattn/go-sqlite3"
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
	db          *sql.DB
)

func main() {
	var err error
	db, err = sql.Open("sqlite3", "bike_tracker.db")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	if err := initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

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
	http.HandleFunc("/api/history", handleHistory)
	http.HandleFunc("/api/last-position", handleLastPosition)
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

	if err := savePosition(position); err != nil {
		log.Printf("Failed to save position: %v", err)
		http.Error(w, "Failed to save position", http.StatusInternalServerError)
		return
	}

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

func initDB() error {
	query := `
	CREATE TABLE IF NOT EXISTS positions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		latitude REAL NOT NULL,
		longitude REAL NOT NULL,
		timestamp INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(query)
	return err
}

func savePosition(position GPSPosition) error {
	query := "INSERT INTO positions (latitude, longitude, timestamp) VALUES (?, ?, ?)"
	_, err := db.Exec(query, position.Latitude, position.Longitude, position.Timestamp)
	return err
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := "SELECT latitude, longitude, timestamp FROM positions ORDER BY created_at"
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Failed to query history: %v", err)
		http.Error(w, "Failed to fetch history", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var positions []GPSPosition
	for rows.Next() {
		var pos GPSPosition
		if err := rows.Scan(&pos.Latitude, &pos.Longitude, &pos.Timestamp); err != nil {
			log.Printf("Failed to scan row: %v", err)
			continue
		}
		positions = append(positions, pos)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(positions)
}

func handleLastPosition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := "SELECT latitude, longitude, timestamp FROM positions ORDER BY created_at DESC LIMIT 1"
	var position GPSPosition
	
	err := db.QueryRow(query).Scan(&position.Latitude, &position.Longitude, &position.Timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(nil)
			return
		}
		log.Printf("Failed to query last position: %v", err)
		http.Error(w, "Failed to fetch last position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(position)
}
