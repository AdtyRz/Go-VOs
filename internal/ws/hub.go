package ws

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Event struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

type Client struct {
	hub        *Hub
	Conn       *websocket.Conn
	Send       chan []byte
	AdminID    int64
	SchoolID   int64
	ElectionID int64
	closeOnce  sync.Once
}

type Hub struct {
	mu          sync.RWMutex
	clients     map[*Client]struct{}
	perAdmin    map[int64]int
	maxClients  int
	maxPerAdmin int
}

func NewHub() *Hub {
	return &Hub{
		clients:     make(map[*Client]struct{}),
		perAdmin:    make(map[int64]int),
		maxClients:  500,
		maxPerAdmin: 4,
	}
}

func (h *Hub) NewClient(conn *websocket.Conn, adminID, schoolID, electionID int64) *Client {
	return &Client{
		hub:        h,
		Conn:       conn,
		Send:       make(chan []byte, 16),
		AdminID:    adminID,
		SchoolID:   schoolID,
		ElectionID: electionID,
	}
}

func (h *Hub) Register(c *Client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients) >= h.maxClients || h.perAdmin[c.AdminID] >= h.maxPerAdmin {
		return false
	}
	h.clients[c] = struct{}{}
	h.perAdmin[c.AdminID]++
	go c.writePump()
	go c.readPump()
	return true
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		h.perAdmin[c.AdminID]--
		if h.perAdmin[c.AdminID] <= 0 {
			delete(h.perAdmin, c.AdminID)
		}
	}
	h.mu.Unlock()

	c.closeOnce.Do(func() {
		close(c.Send)
		_ = c.Conn.Close()
	})
}

func (h *Hub) Broadcast(electionID int64, event Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		slog.Error("websocket marshal", "error", err)
		return
	}
	h.mu.RLock()
	targets := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		if c.ElectionID == electionID {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.Send <- payload:
		default:

		}
	}
}

func (h *Hub) Send(c *Client, event Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	select {
	case c.Send <- payload:
	default:
	}
}

func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) CloseAll() {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	for _, c := range clients {
		h.unregister(c)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
		c.hub.unregister(c)
	}()

	_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	for {
		select {
		case msg, ok := <-c.Send:
			if !ok {
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return
			}
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) readPump() {
	defer c.hub.unregister(c)
	c.Conn.SetReadLimit(1024)
	_ = c.Conn.SetReadDeadline(time.Now().Add(65 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		return c.Conn.SetReadDeadline(time.Now().Add(65 * time.Second))
	})
	for {
		if _, _, err := c.Conn.ReadMessage(); err != nil {
			return
		}
	}
}
