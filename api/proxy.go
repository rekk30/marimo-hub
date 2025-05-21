package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/gofiber/fiber/v3"
	"github.com/gorilla/websocket"
	"github.com/rekk30/marimo-hub/pkg/core"
	wsproxy "github.com/rekk30/marimo-hub/pkg/websocket"
)

func SetupProxyRoutes(app *fiber.App, reg core.Registry, runner *core.Runner) {
	app.Get("/ws", wsproxy.New(func(conn *wsproxy.Conn) {
		host := conn.Hostname
		nb, ok := reg.GetByDomain(host)
		if !ok {
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "no such notebook"))
			return
		}
		port, ok := runner.GetPort(nb.ID)
		if !ok {
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "port unavailable"))
			return
		}

		path := conn.Path
		rawQS := conn.RawQuery
		targetUrl := fmt.Sprintf("ws://127.0.0.1:%d%s", port, path)
		if rawQS != "" {
			targetUrl += "?" + rawQS
		}

		backend, _, err := websocket.DefaultDialer.Dial(targetUrl, nil)
		if err != nil {
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseTryAgainLater, err.Error()))
			return
		}
		defer backend.Close()

		go func() {
			for {
				t, msg, err := backend.ReadMessage()
				if err != nil {
					conn.WriteMessage(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					return
				}
				if err := conn.WriteMessage(t, msg); err != nil {
					return
				}
			}
		}()
		for {
			t, msg, err := conn.ReadMessage()
			if err != nil {
				backend.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if err := backend.WriteMessage(t, msg); err != nil {
				return
			}
		}
	}))

	app.Use(func(c fiber.Ctx) error {
		host := c.Hostname()

		nb, exists := reg.GetByDomain(host)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(core.ErrorResponse{Error: "Notebook not found for this domain"})
		}

		port, ok := runner.GetPort(nb.ID)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(core.ErrorResponse{Error: "Notebook found but port not available"})
		}

		status, err := runner.GetStatus(nb.ID)
		if err != nil || status != core.StatusRunning {
			return c.Status(fiber.StatusInternalServerError).JSON(core.ErrorResponse{Error: "Notebook not running"})
		}

		client := &http.Client{}
		req, err := http.NewRequest(c.Method(), fmt.Sprintf("http://localhost:%d%s", port, c.Path()), bytes.NewReader(c.Body()))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(core.ErrorResponse{Error: "Failed to proxy request"})
		}

		for k, v := range c.GetReqHeaders() {
			if len(v) > 0 {
				req.Header.Set(k, v[0])
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(core.ErrorResponse{Error: "Failed to proxy request"})
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(core.ErrorResponse{Error: "Failed to read response"})
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))

		for k, v := range resp.Header {
			c.Set(k, v[0])
		}
		if _, exists := resp.Header["Content-Type"]; !exists {
			c.Set("Content-Type", "application/json")
		}

		return c.Status(resp.StatusCode).SendStream(resp.Body)
	})
}
