package chat

import (
	"encoding/json"
	"live-platform/internal/config"
	"live-platform/internal/utils"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
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
	Content   string      `json:"content,omitempty"`
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
	hub    *Hub
	jwtCfg *config.JWTConfig
}

func NewHandler(hub *Hub, jwtCfg *config.JWTConfig) *Handler {
	return &Handler{hub: hub, jwtCfg: jwtCfg}
}

func (h *Handler) HandleWebSocket(c fiber.Ctx) error {
	streamID := c.Params("stream_id")

	// Get token from query parameter for WebSocket connections
	token := c.Query("token")
	if token == "" {
		log.Printf("WebSocket: No token provided")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "token required"})
	}

	// Validate JWT token
	claims, err := utils.ValidateToken(token, h.jwtCfg.AccessSecret)
	if err != nil {
		log.Printf("WebSocket: Invalid token: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	userID := claims.UserID
	username := claims.Email
	if username == "" {
		username = userID.String()
	}

	log.Printf("WebSocket: Authenticated user %s for stream %s", username, streamID)

	// Create a custom HTTP handler that upgrades to WebSocket
	wsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}

		// Handle the WebSocket connection
		h.handleWSConnection(conn, streamID, userID.String(), username)
	})

	// Use Fiber's adaptor to convert and handle the HTTP request
	return adaptor.HTTPHandlerFunc(wsHandler)(c)
}

func (h *Handler) handleWSConnection(conn *websocket.Conn, streamID, userID, username string) {
	client := &Client{
		conn:     conn,
		streamID: streamID,
		userID:   userID,
		username: username,
		send:     make(chan []byte, 256),
	}

	h.hub.register <- client

	// Start write pump in a goroutine
	go h.writePump(client)

	// Read pump runs in the current goroutine (blocks until connection closes)
	h.readPump(client)
}

func (h *Handler) readPump(client *Client) {
	defer func() {
		h.hub.unregister <- client
		client.conn.Close()
	}()

	for {
		_, msgBytes, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			// Try to parse as simple content
			msg = Message{Content: string(msgBytes)}
		}

		msg.StreamID = client.streamID
		msg.UserID = client.userID
		msg.Username = client.username
		msg.Type = "message"
		if msg.Message == "" {
			msg.Message = msg.Content
		}

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
