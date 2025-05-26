package main

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/gofiber/fiber/v3"
	"github.com/rekk30/marimo-hub/api"
	"github.com/rekk30/marimo-hub/pkg/config"
	"github.com/rekk30/marimo-hub/pkg/core"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

func main() {
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Stack().Err(err).Msg("Failed to load configuration")
	}
	log.Info().Interface("config", cfg).Msgf("Configuration loaded")

	cmd := exec.Command("marimo", "edit", "--headless", "--host", "0.0.0.0", "-p", fmt.Sprintf("%d", cfg.Server.MarimoPort), "--skip-update-check", "--watch", "--allow-origins", "*", "--no-token")
	if err := cmd.Start(); err != nil {
		log.Fatal().Stack().Err(err).Msg("Failed to start marimo")
	}
	defer cmd.Process.Kill()
	log.Info().Msgf("Marimo started on port %d", cfg.Server.MarimoPort)

	apiApp := fiber.New(fiber.Config{})
	proxyApp := fiber.New(fiber.Config{})

	runner := core.NewRunner(context.Background())
	reg, err := core.NewBadgerRegistry(cfg.Database.Path, runner.HandleRegistryEvent)
	if err != nil {
		log.Fatal().Stack().Err(err).Msg("Failed to create registry")
	}

	api.SetupAPIRoutes(apiApp, reg, runner)
	api.SetupProxyRoutes(proxyApp, reg, runner)

	log.Info().Msgf("Starting API server on port %d and proxy server on port %d", cfg.Server.APIPort, cfg.Server.ProxyPort)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := apiApp.Listen(fmt.Sprintf(":%d", cfg.Server.APIPort)); err != nil {
			log.Error().Stack().Err(err).Msg("API server error")
		}
	}()

	go func() {
		defer wg.Done()
		if err := proxyApp.Listen(fmt.Sprintf(":%d", cfg.Server.ProxyPort)); err != nil {
			log.Error().Stack().Err(err).Msg("Proxy server error")
		}
	}()

	wg.Wait()
}
