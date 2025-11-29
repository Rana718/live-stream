package chat

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Hub struct {
	clients    map[string]map[*websocket.Conn]*Client
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

type Client struct {
	conn     *websocket.Conn
	streamID string
	userID   string
	username string
	send     chan []byte
}

type Message struct {
	StreamID  string      `json:"stream_id"`
	UserID    string      `json:"user_id"`
	Username  string      `json:"username"`
	Message   string      `json:"message"`
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]map[*websocket.Conn]*Client),
		broadcast:  make(chan Message, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.clients[client.streamID] == nil {
				h.clients[client.streamID] = make(map[*websocket.Conn]*Client)
			}
			h.clients[client.streamID][client.conn] = client
			h.mu.Unlock()
			log.Printf("Client %s joined stream %s", client.userID, client.streamID)

			// Send join notification
			joinMsg := Message{
				StreamID:  client.streamID,
				UserID:    client.userID,
				Username:  client.username,
				Type:      "join",
				Message:   client.username + " joined the chat",
				Timestamp: time.Now(),
			}
			h.broadcast <- joinMsg

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.streamID]; ok {
				if _, ok := clients[client.conn]; ok {
					delete(clients, client.conn)
					close(client.send)
					if len(clients) == 0 {
						delete(h.clients, client.streamID)
					}
				}
			}
			h.mu.Unlock()
			log.Printf("Client %s left stream %s", client.userID, client.streamID)

			// Send leave notification
			leaveMsg := Message{
				StreamID:  client.streamID,
				UserID:    client.userID,
				Username:  client.username,
				Type:      "leave",
				Message:   client.username + " left the chat",
				Timestamp: time.Now(),
			}
			h.broadcast <- leaveMsg

		case message := <-h.broadcast:
			message.Timestamp = time.Now()
			h.mu.RLock()
			if clients, ok := h.clients[message.StreamID]; ok {
				messageData, _ := json.Marshal(message)
				for _, client := range clients {
					select {
					case client.send <- messageData:
					default:
						close(client.send)
						delete(clients, client.conn)
					}
				}
			}
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

func (h *Handler) HandleWebSocket(c fiber.Ctx) error {
	streamID := c.Params("stream_id")
	userID := c.Locals("userID").(uuid.UUID)
	
	// Get username from context or use userID as fallback
	username := userID.String()
	if uname, ok := c.Locals("username").(string); ok {
		username = uname
	}

	// Convert Fiber context to fasthttp context
	fctx := c.Context()
	
	// Create HTTP handler for WebSocket upgrade
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}

		client := &Client{
			conn:     conn,
			streamID: streamID,
			userID:   userID.String(),
			username: username,
			send:     make(chan []byte, 256),
		}

		h.hub.register <- client

		// Start goroutines for reading and writing
		go h.writePump(client)
		go h.readPump(client)
	})

	// Adapt and call the handler
	fasthttpadaptor.NewFastHTTPHandler(handler)(fctx.(*fasthttp.RequestCtx))
	
	return nil
}

func (h *Handler) readPump(client *Client) {
	defer func() {
		h.hub.unregister <- client
		client.conn.Close()
	}()

	for {
		var msg Message
		err := client.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		msg.StreamID = client.streamID
		msg.UserID = client.userID
		msg.Username = client.username
		msg.Type = "message"

		h.hub.broadcast <- msg
	}
}

func (h *Handler) writePump(client *Client) {
	defer func() {
		client.conn.Close()
	}()

	for message := range client.send {
		if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}

// SendMessage - REST endpoint for sending chat messages (fallback)
func (h *Handler) SendMessage(c fiber.Ctx) error {
	streamID := c.Params("stream_id")
	userID := c.Locals("userID").(uuid.UUID)

	username := userID.String()
	if uname, ok := c.Locals("username").(string); ok {
		username = uname
	}

	var req struct {
		Message string `json:"message"`
	}

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}

	msg := Message{
		StreamID: streamID,
		UserID:   userID.String(),
		Username: username,
		Message:  req.Message,
		Type:     "message",
	}

	h.hub.broadcast <- msg

	return c.JSON(fiber.Map{
		"success": true,
		"message": "message sent",
	})
}

// GetChatHistory - Get chat history for a stream
func (h *Handler) GetChatHistory(c fiber.Ctx) error {
	streamID := c.Params("stream_id")

	h.hub.mu.RLock()
	clientCount := 0
	if clients, ok := h.hub.clients[streamID]; ok {
		clientCount = len(clients)
	}
	h.hub.mu.RUnlock()

	return c.JSON(fiber.Map{
		"stream_id":    streamID,
		"active_users": clientCount,
		"message":      "Use WebSocket connection for real-time chat",
	})
}
