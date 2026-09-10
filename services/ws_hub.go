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
	IsRead          bool                  `json:"is_read"`
	IsDelivered     bool                  `json:"is_delivered"`
	ReadStatus      string                `json:"read_status"` // "sent", "delivered", "read"
	CreatedAt       string                `json:"created_at"`
	CreatedAtISO    string                `json:"created_at_iso"`
	Attachments     []WSAttachmentPayload `json:"attachments"`
}

// WSMessage is the envelope sent to connected clients.
type WSMessage struct {
	Type         string          `json:"type"` // e.g. "new_reply", "messages_read"
	TicketID     uint            `json:"ticket_id"`
	ReaderUserID uint            `json:"reader_user_id,omitempty"`
	Reply        *WSReplyPayload `json:"reply,omitempty"`
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
	rooms       map[uint]map[*WSClient]bool
	activeUsers map[uint]int
	register    chan *WSClient
	unregister  chan *WSClient
	broadcast   chan *WSMessage
	mu          sync.RWMutex
}

// NewWSHub creates a new WSHub instance.
func NewWSHub() *WSHub {
	return &WSHub{
		rooms:       make(map[uint]map[*WSClient]bool),
		activeUsers: make(map[uint]int),
		register:    make(chan *WSClient),
		unregister:  make(chan *WSClient),
		broadcast:   make(chan *WSMessage, 256),
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
			if client.UserID > 0 {
				h.activeUsers[client.UserID]++
			}
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
			if client.UserID > 0 {
				h.activeUsers[client.UserID]--
				if h.activeUsers[client.UserID] <= 0 {
					delete(h.activeUsers, client.UserID)
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

// IsUserOnline checks if a user currently has an active WebSocket connection.
func (h *WSHub) IsUserOnline(userID uint) bool {
	if h == nil || userID == 0 {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.activeUsers[userID] > 0
}

// IsUserInTicketRoom checks if a user is actively viewing a specific ticket room.
func (h *WSHub) IsUserInTicketRoom(ticketID uint, userID uint) bool {
	if h == nil || ticketID == 0 || userID == 0 {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients, exists := h.rooms[ticketID]
	if !exists {
		return false
	}
	for client := range clients {
		if client.UserID == userID {
			return true
		}
	}
	return false
}

// HasOtherParticipantInRoom checks if any user other than sender is in the ticket room.
func (h *WSHub) HasOtherParticipantInRoom(ticketID uint, senderUserID uint) bool {
	if h == nil || ticketID == 0 {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients, exists := h.rooms[ticketID]
	if !exists {
		return false
	}
	for client := range clients {
		if client.UserID != senderUserID {
			return true
		}
	}
	return false
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

// BroadcastMessagesRead notifies all participants in the ticket room that messages have been read.
func (h *WSHub) BroadcastMessagesRead(ticketID uint, readerUserID uint) {
	if h == nil || ticketID == 0 {
		return
	}
	h.broadcast <- &WSMessage{
		Type:         "messages_read",
		TicketID:     ticketID,
		ReaderUserID: readerUserID,
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
				log.Printf("[WebSocket] Write error: %v", err)
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

// ReadPump handles incoming WebSocket messages and unregisters on disconnect.
func (c *WSClient) ReadPump() {
	defer func() {
		c.Hub.UnregisterClient(c)
		_ = c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512)
	_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WebSocket] Read error: %v", err)
			}
			break
		}
	}
}