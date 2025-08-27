package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"os"
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
	defer conn.Close()

	log.Printf("Client connected: %s", conn.RemoteAddr())

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			position := generateRandomPosition()
			
			if err := conn.WriteJSON(position); err != nil {
				log.Printf("WebSocket write failed: %v", err)
				return
			}
			
			log.Printf("Sent position: lat=%.6f, lng=%.6f", position.Latitude, position.Longitude)
		}
	}
}

func generateRandomPosition() GPSPosition {
	baseLat := 52.5200
	baseLng := 13.4050
	
	latOffset := (rand.Float64() - 0.5) * 0.1
	lngOffset := (rand.Float64() - 0.5) * 0.1
	
	return GPSPosition{
		Latitude:  baseLat + latOffset,
		Longitude: baseLng + lngOffset,
		Timestamp: time.Now().Unix(),
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