package services

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSAttachmentPayload represents an attachment in a real-time message.
type WSAttachmentPayload struct {
	ID            uint   `json:"id"`
	FileName      string `json:"file_name"`
	FilePath      string `json:"file_path"`
	IsImage       bool   `json:"is_image"`
	IsPDF         bool   `json:"is_pdf"`
	FormattedSize string `json:"formatted_size"`
}

// WSReplyPayload contains all data needed by client to render a new reply bubble.
type WSReplyPayload struct {
	ID              uint                  `json:"id"`
	TicketID        uint                  `json:"ticket_id"`
	UserID          uint                  `json:"user_id"`
	Username        string                `json:"username"`
	UserDisplayName string                `json:"user_display_name"`
	IsStaff         bool                  `json:"is_staff"`
	Message         string                `json:"message"`
	CreatedAt       string                `json:"created_at"`
	CreatedAtISO    string                `json:"created_at_iso"`
	Attachments     []WSAttachmentPayload `json:"attachments"`
}

// WSMessage is the envelope sent to connected clients.
type WSMessage struct {
	Type     string          `json:"type"` // e.g. "new_reply"
	TicketID uint            `json:"ticket_id"`
	Reply    *WSReplyPayload `json:"reply,omitempty"`
}

// WSClient represents a single active WebSocket connection.
type WSClient struct {
	Hub      *WSHub
	Conn     *websocket.Conn
	TicketID uint
	UserID   uint
	Send     chan *WSMessage
}

// WSHub maintains active clients and broadcasts messages to ticket rooms.
type WSHub struct {
	// rooms maps TicketID -> map of clients
	rooms      map[uint]map[*WSClient]bool
	register   chan *WSClient
	unregister chan *WSClient
	broadcast  chan *WSMessage
	mu         sync.RWMutex
}

// NewWSHub creates a new WSHub instance.
func NewWSHub() *WSHub {
	return &WSHub{
		rooms:      make(map[uint]map[*WSClient]bool),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
		broadcast:  make(chan *WSMessage, 256),
	}
}

// Run listens on channels and manages client membership.
func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.rooms[client.TicketID] == nil {
				h.rooms[client.TicketID] = make(map[*WSClient]bool)
			}
			h.rooms[client.TicketID][client] = true
			h.mu.Unlock()
			log.Printf("[WebSocket] Client registered for ticket #%d (user #%d)", client.TicketID, client.UserID)

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.rooms[client.TicketID]; ok {
				if _, exists := clients[client]; exists {
					delete(clients, client)
					close(client.Send)
					if len(clients) == 0 {
						delete(h.rooms, client.TicketID)
					}
				}
			}
			h.mu.Unlock()
			log.Printf("[WebSocket] Client unregistered for ticket #%d (user #%d)", client.TicketID, client.UserID)

		case msg := <-h.broadcast:
			h.mu.RLock()
			clients := h.rooms[msg.TicketID]
			for client := range clients {
				select {
				case client.Send <- msg:
				default:
					// Slow client: close and delete
					close(client.Send)
					delete(clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastReply sends a new reply message to all clients in the ticket's room.
func (h *WSHub) BroadcastReply(ticketID uint, reply *WSReplyPayload) {
	if h == nil || reply == nil {
		return
	}
	h.broadcast <- &WSMessage{
		Type:     "new_reply",
		TicketID: ticketID,
		Reply:    reply,
	}
}

// RegisterClient adds a client to the hub.
func (h *WSHub) RegisterClient(c *WSClient) {
	h.register <- c
}

// UnregisterClient removes a client from the hub.
func (h *WSHub) UnregisterClient(c *WSClient) {
	h.unregister <- c
}

// WritePump pumps messages from the hub to the websocket connection.
func (c *WSClient) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		_ = c.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// The hub closed the channel.
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteJSON(msg); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ReadPump pumps messages from the websocket connection to the hub.
// Client sends no data other than keepalive pong responses.
func (c *WSClient) ReadPump() {
	defer func() {
		c.Hub.UnregisterClient(c)
		_ = c.Conn.Close()
	}()

	c.Conn.SetReadLimit(4096)
	_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
	}
}