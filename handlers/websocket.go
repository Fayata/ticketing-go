package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"ticketing/config"
	"ticketing/models"
	"ticketing/services"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow websocket upgrades for authenticated sessions on this host
		return true
	},
}

// WSHandler handles WebSocket connections for tickets.
type WSHandler struct {
	hub *services.WSHub
}

// NewWSHandler returns a new WSHandler instance.
func NewWSHandler(hub *services.WSHub) *WSHandler {
	return &WSHandler{hub: hub}
}

// HandleTicketWS upgrades connection to WebSocket for the requested ticket.
func (h *WSHandler) HandleTicketWS(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r).(*models.User)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// URL format: /ws/ticket/{id}
	path := strings.TrimPrefix(r.URL.Path, "/ws/ticket/")
	ticketID, err := strconv.Atoi(path)
	if err != nil || ticketID <= 0 {
		http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
		return
	}

	// Verify authorization: SuperAdmin, Staff, or ticket creator
	var ticket models.Ticket
	if err := config.DB.Select("id", "created_by_id", "department_id").First(&ticket, ticketID).Error; err != nil {
		http.Error(w, "Ticket not found", http.StatusNotFound)
		return
	}

	isAllowed := user.IsSuperAdmin || user.IsStaff || ticket.CreatedByID == user.ID
	if !isAllowed {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade error for user #%d ticket #%d: %v", user.ID, ticketID, err)
		return
	}

	client := &services.WSClient{
		Hub:      h.hub,
		Conn:     conn,
		TicketID: uint(ticketID),
		UserID:   user.ID,
		Send:     make(chan *services.WSMessage, 64),
	}

	h.hub.RegisterClient(client)

	go client.WritePump()
	client.ReadPump()
}