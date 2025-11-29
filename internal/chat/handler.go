package chat

import (
	"log"
	"sync"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type Hub struct {
	clients    map[string]map[string]bool
	broadcast  chan Message
	register   chan Subscription
	unregister chan Subscription
	mu         sync.RWMutex
}

type Subscription struct {
	userID   string
	streamID string
}

type Message struct {
	StreamID string      `json:"stream_id"`
	UserID   string      `json:"user_id"`
	Username string      `json:"username"`
	Message  string      `json:"message"`
	Type     string      `json:"type"`
	Data     interface{} `json:"data,omitempty"`
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]map[string]bool),
		broadcast:  make(chan Message),
		register:   make(chan Subscription),
		unregister: make(chan Subscription),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case sub := <-h.register:
			h.mu.Lock()
			if h.clients[sub.streamID] == nil {
				h.clients[sub.streamID] = make(map[string]bool)
			}
			h.clients[sub.streamID][sub.userID] = true
			h.mu.Unlock()
			log.Printf("User %s joined stream %s", sub.userID, sub.streamID)

		case sub := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[sub.streamID]; ok {
				delete(clients, sub.userID)
				if len(clients) == 0 {
					delete(h.clients, sub.streamID)
				}
			}
			h.mu.Unlock()
			log.Printf("User %s left stream %s", sub.userID, sub.streamID)

		case msg := <-h.broadcast:
			h.mu.RLock()
			log.Printf("Broadcasting message to stream %s: %s", msg.StreamID, msg.Message)
			h.mu.RUnlock()
		}
	}
}

type Handler struct {
	hub *Hub
}

func NewHandler(hub *Hub) *Handler {
	return &Handler{hub: hub}
}

// HandleWebSocket - Placeholder for WebSocket implementation
// TODO: Implement proper WebSocket support when Fiber v3 stable release is available
func (h *Handler) HandleWebSocket(c fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"error":   "WebSocket not implemented yet",
		"message": "Use REST API for chat messages",
	})
}

// SendMessage - REST endpoint for sending chat messages
func (h *Handler) SendMessage(c fiber.Ctx) error {
	streamID := c.Params("stream_id")
	userID := c.Locals("userID").(uuid.UUID)

	var req struct {
		Message string `json:"message"`
	}

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	msg := Message{
		StreamID: streamID,
		UserID:   userID.String(),
		Message:  req.Message,
		Type:     "chat",
	}

	h.hub.broadcast <- msg

	return c.JSON(fiber.Map{
		"success": true,
		"message": "message sent",
	})
}
