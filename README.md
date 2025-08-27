# Bike Tracker

A real-time GPS tracking application built with Go and WebSockets.

<img width="1000" alt="image" src="https://github.com/user-attachments/assets/906afe43-1308-444a-b075-3e591434d0f5" />

## Features

- Real-time position updates via WebSocket
- Interactive map display using MapLibre GL with OpenStreetMap tiles
- Configurable map styles

## Usage

Environment variables:
- `ADDR` - Server address (default: localhost)
- `PORT` - Server port (default: 8080)  
- `MAP_STYLE` - Map tile style URL (default: OpenStreetMap)
  - Use any MapLibre GL compatible style JSON URL
  - Free options: OpenStreetMap, MapTiler (requires free API key), Maptiler
    styles (free API key at https://www.maptiler.com)

The server starts on `http://localhost:8080` by default.

