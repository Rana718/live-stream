package chat

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"live-platform/internal/config"
	"live-platform/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	// maxMessageBytes bounds a single inbound frame so a client can't push
	// the server into an OOM by streaming one huge "message".
	maxMessageBytes = 4 << 10 // 4 KiB
	// maxMessageRunes is the human-facing chat line limit.
	maxMessageRunes = 2000

	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10

	// perClientMinInterval throttles a single socket's send rate.
	perClientMinInterval = 500 * time.Millisecond
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// The connection is already authenticated by a JWT in the query string
	// before the upgrade; a permissive origin check is acceptable because a
	// forged Origin cannot mint a valid token. Tighten if cookie auth is
	// ever added.
	CheckOrigin: func(r *http.Request) bool { return true },
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
	tenantID uuid.UUID
	userID   uuid.UUID
	username string
	send     chan []byte
	lastSend time.Time
}

type Message struct {
	StreamID  string    `json:"stream_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Message   string    `json:"message"`
	Content   string    `json:"content,omitempty"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
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

		case message := <-h.broadcast:
			message.Timestamp = time.Now()
			data, _ := json.Marshal(message)
			h.mu.RLock()
			for _, client := range h.clients[message.StreamID] {
				select {
				case client.send <- data:
				default:
					// Slow consumer — drop it rather than block the hub.
					close(client.send)
					delete(h.clients[message.StreamID], client.conn)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// ViewerCount reports how many live sockets a stream has.
func (h *Hub) ViewerCount(streamID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[streamID])
}

type Handler struct {
	hub    *Hub
	jwtCfg *config.JWTConfig
	svc    *Service
	log    *slog.Logger
}

func NewHandler(hub *Hub, jwtCfg *config.JWTConfig, svc *Service, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{hub: hub, jwtCfg: jwtCfg, svc: svc, log: log}
}

func sanitizeMessage(s string) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > maxMessageRunes {
		s = string([]rune(s)[:maxMessageRunes])
	}
	return s
}

func (h *Handler) HandleWebSocket(c fiber.Ctx) error {
	streamID := c.Params("stream_id")
	if _, err := uuid.Parse(streamID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}

	token := c.Query("token")
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "token required"})
	}
	claims, err := utils.ValidateToken(token, h.jwtCfg.AccessSecret)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	sid := uuid.MustParse(streamID)
	// A user may only join the chat of a stream inside their own tenant.
	if h.svc != nil && !h.svc.StreamBelongsToTenant(c.Context(), claims.TenantID, claims.UserID, sid) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "stream not found in your org"})
	}

	username := claims.Email
	if username == "" {
		username = claims.UserID.String()
	}

	wsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			h.log.Warn("ws upgrade failed", "err", err)
			return
		}
		h.handleWSConnection(conn, streamID, claims.TenantID, claims.UserID, username)
	})
	return adaptor.HTTPHandlerFunc(wsHandler)(c)
}

func (h *Handler) handleWSConnection(conn *websocket.Conn, streamID string, tenantID, userID uuid.UUID, username string) {
	client := &Client{
		conn:     conn,
		streamID: streamID,
		tenantID: tenantID,
		userID:   userID,
		username: username,
		send:     make(chan []byte, 64),
	}

	h.hub.register <- client
	go h.writePump(client)
	h.readPump(client)
}

func (h *Handler) readPump(client *Client) {
	defer func() {
		h.hub.unregister <- client
		_ = client.conn.Close()
	}()

	client.conn.SetReadLimit(maxMessageBytes)
	_ = client.conn.SetReadDeadline(time.Now().Add(pongWait))
	client.conn.SetPongHandler(func(string) error {
		return client.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, msgBytes, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.log.Debug("ws read error", "err", err)
			}
			return
		}

		var in Message
		if err := json.Unmarshal(msgBytes, &in); err != nil {
			in = Message{Content: string(msgBytes)}
		}
		text := sanitizeMessage(firstNonEmpty(in.Message, in.Content))
		if text == "" {
			continue
		}

		// Simple per-socket flood control.
		if !client.lastSend.IsZero() && time.Since(client.lastSend) < perClientMinInterval {
			continue
		}
		client.lastSend = time.Now()

		out := Message{
			StreamID: client.streamID,
			UserID:   client.userID.String(),
			Username: client.username,
			Message:  text,
			Type:     "message",
		}

		// Persist best-effort; a DB blip must not drop the live message.
		if h.svc != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if _, err := h.svc.SaveMessage(ctx, client.tenantID,
				uuid.MustParse(client.streamID), client.userID, text); err != nil {
				h.log.Warn("chat persist failed", "err", err, "stream", client.streamID)
			}
			cancel()
		}

		h.hub.broadcast <- out
	}
}

func (h *Handler) writePump(client *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = client.conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.send:
			_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// SendMessage is the REST fallback for posting a chat line. Requires
// AuthMiddleware + TenantContext.
func (h *Handler) SendMessage(c fiber.Ctx) error {
	streamID := c.Params("stream_id")
	sid, err := uuid.Parse(streamID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}
	userID, _ := c.Locals("userID").(uuid.UUID)
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)

	var req struct {
		Message string `json:"message"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	text := sanitizeMessage(req.Message)
	if text == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "empty message"})
	}

	if h.svc == nil || !h.svc.StreamBelongsToTenant(c.Context(), tenantID, userID, sid) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "stream not found in your org"})
	}

	username := userID.String()
	if uname, ok := c.Locals("username").(string); ok && uname != "" {
		username = uname
	}

	if _, err := h.svc.SaveMessage(c.Context(), tenantID, sid, userID, text); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not send"})
	}

	h.hub.broadcast <- Message{
		StreamID: streamID,
		UserID:   userID.String(),
		Username: username,
		Message:  text,
		Type:     "message",
	}
	return c.JSON(fiber.Map{"success": true})
}

// GetChatHistory returns stored messages plus the live viewer count.
func (h *Handler) GetChatHistory(c fiber.Ctx) error {
	streamID := c.Params("stream_id")
	sid, err := uuid.Parse(streamID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid stream id"})
	}
	userID, _ := c.Locals("userID").(uuid.UUID)
	tenantID, _ := c.Locals("tenantID").(uuid.UUID)

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	var items []HistoryItem
	if h.svc != nil {
		items, err = h.svc.History(c.Context(), tenantID, userID, sid, int32(limit), int32(offset))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not load history"})
		}
	}

	return c.JSON(fiber.Map{
		"stream_id":    streamID,
		"active_users": h.hub.ViewerCount(streamID),
		"messages":     items,
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
