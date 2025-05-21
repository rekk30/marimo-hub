package websocket

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
)

// Config configures the WebSocket middleware.
type Config struct {
	// Filter skips upgrades when returning false.
	Filter func(fiber.Ctx) bool
	// HandshakeTimeout for the WebSocket upgrade.
	HandshakeTimeout time.Duration
	// Subprotocols requested by the client.
	Subprotocols []string
	// Origins allowed for upgrade; defaults to ["*"].
	Origins []string
	// ReadBufferSize for incoming frames (bytes).
	ReadBufferSize int
	// WriteBufferSize for outgoing frames (bytes).
	WriteBufferSize int
	// EnableCompression toggles per-message compression.
	EnableCompression bool
	// RecoverHandler handles panics inside handler.
	RecoverHandler func(*Conn)
}

func defaultRecover(c *Conn) {
	if err := recover(); err != nil {
		fmt.Fprintf(os.Stderr, "panic: %v\n%s\n", err, debug.Stack())
		_ = c.writeJSON(map[string]interface{}{"error": fmt.Sprint(err)})
	}
}

type Conn struct {
	*websocket.Conn

	Hostname string
	Path     string
	RawQuery string
	Headers  map[string]string
	Cookies  map[string]string
}

func (c *Conn) writeJSON(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.WriteMessage(websocket.TextMessage, b)
}

func (c *Conn) GetHeader(key string) (string, bool) {
	val, ok := c.Headers[key]
	return val, ok
}

func New(handler func(*Conn), cfg ...Config) fiber.Handler {
	var config Config
	if len(cfg) > 0 {
		config = cfg[0]
	}
	if config.Origins == nil {
		config.Origins = []string{"*"}
	}

	upgrader := websocket.FastHTTPUpgrader{
		HandshakeTimeout:  config.HandshakeTimeout,
		Subprotocols:      config.Subprotocols,
		ReadBufferSize:    config.ReadBufferSize,
		WriteBufferSize:   config.WriteBufferSize,
		EnableCompression: config.EnableCompression,
		CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
			origin := string(ctx.Request.Header.Peek("Origin"))
			if len(config.Origins) == 1 && config.Origins[0] == "*" {
				return true
			}
			for _, o := range config.Origins {
				if o == origin {
					return true
				}
			}
			return false
		},
	}

	return func(c fiber.Ctx) error {
		if config.Filter != nil && !config.Filter(c) {
			return c.Next()
		}
		if !websocket.FastHTTPIsWebSocketUpgrade(c.RequestCtx()) {
			return c.Next()
		}

		path := c.Path()
		host := c.Hostname()
		rawQS := string(c.RequestCtx().URI().QueryString())

		headers := make(map[string]string)
		c.RequestCtx().Request.Header.VisitAll(func(k, v []byte) {
			headers[string(k)] = string(v)
		})

		cookies := make(map[string]string)
		c.RequestCtx().Request.Header.VisitAllCookie(func(k, v []byte) {
			cookies[string(k)] = string(v)
		})

		err := upgrader.Upgrade(c.RequestCtx(), func(ws *websocket.Conn) {
			conn := &Conn{Conn: ws, Hostname: host, Path: path, RawQuery: rawQS, Headers: headers, Cookies: cookies}
			defer func() {
				if config.RecoverHandler != nil {
					config.RecoverHandler(conn)
				} else {
					defaultRecover(conn)
				}
				_ = conn.Close()
			}()
			handler(conn)
		})
		if err != nil {
			return fiber.ErrUpgradeRequired
		}
		return nil
	}
}
