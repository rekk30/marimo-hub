package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"

	"github.com/gofiber/fiber/v3"
	"github.com/rekk30/marimo-hub/api"
	"github.com/rekk30/marimo-hub/pkg/config"
	"github.com/rekk30/marimo-hub/pkg/core"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	cmd := exec.Command("marimo", "edit", "--headless", "-p", fmt.Sprintf("%d", cfg.Server.MarimoPort), "--skip-update-check", "--watch", "--allow-origins", "*", "--no-token")
	if err := cmd.Start(); err != nil {
		log.Fatal("Failed to start marimo:", err)
	}
	defer cmd.Process.Kill()
	log.Printf("Marimo started on port %d", cfg.Server.MarimoPort)

	apiApp := fiber.New(fiber.Config{})
	proxyApp := fiber.New(fiber.Config{})

	runner := core.NewRunner(context.Background())
	reg, err := core.NewBadgerRegistry(cfg.Database.Path, runner.HandleRegistryEvent)
	if err != nil {
		log.Fatal(err)
	}

	api.SetupAPIRoutes(apiApp, reg, runner)
	api.SetupProxyRoutes(proxyApp, reg, runner)

	log.Printf("Starting API server on port %d and proxy server on port %d", cfg.Server.APIPort, cfg.Server.ProxyPort)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := apiApp.Listen(fmt.Sprintf(":%d", cfg.Server.APIPort)); err != nil {
			log.Fatal("API server error:", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := proxyApp.Listen(fmt.Sprintf(":%d", cfg.Server.ProxyPort)); err != nil {
			log.Fatal("Proxy server error:", err)
		}
	}()

	wg.Wait()
}
