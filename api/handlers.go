package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/rekk30/marimo-hub/pkg/core"
)

var validate = validator.New()

func init() {
	validate.RegisterValidation("filepath", func(fl validator.FieldLevel) bool {
		path := fl.Field().String()
		return path != "" && !strings.Contains(path, "..")
	})
}

func validateRequest(req interface{}) error {
	if err := validate.Struct(req); err != nil {
		if _, ok := err.(*validator.InvalidValidationError); ok {
			return err
		}

		var errMsgs []string
		for _, err := range err.(validator.ValidationErrors) {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %s", err.Field(), err.Tag()))
		}
		return fmt.Errorf("validation failed: %s", strings.Join(errMsgs, "; "))
	}
	return nil
}

func SetupAPIRoutes(app *fiber.App, reg core.Registry, runner *core.Runner) {
	api := app.Group("/api/v1")
	api.Get("/notebooks/:id", getNotebook(reg))
	api.Get("/notebooks/:id/status", getNotebookStatus(runner))
	api.Get("/notebooks", getNotebooks(reg))
	api.Post("/notebooks", postNotebook(reg))
	api.Put("/notebooks/:id", putNotebook(reg))
	api.Delete("/notebooks/:id", deleteNotebook(reg))
}

//--- Handlers ---//

func getNotebook(reg core.Registry) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("id")
		nb, exists := reg.Get(id)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(core.ErrorResponse{Error: "Notebook not found"})
		}
		return c.JSON(core.NotebookResponse{Notebook: nb})
	}
}

func getNotebookStatus(runner *core.Runner) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("id")
		status, err := runner.GetStatus(id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(core.ErrorResponse{Error: err.Error()})
		}
		return c.JSON(core.StatusResponse{Status: status})
	}
}

func getNotebooks(reg core.Registry) fiber.Handler {
	return func(c fiber.Ctx) error {
		nbs := reg.List()
		return c.JSON(core.NotebooksResponse{Notebooks: nbs})
	}
}

func postNotebook(reg core.Registry) fiber.Handler {
	return func(c fiber.Ctx) error {

		var req core.CreateUpdateNotebookRequest
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(core.ErrorResponse{Error: "Invalid request"})
		}

		if req.Name == "" || req.Path == "" || req.Domain == "" {
			return c.Status(fiber.StatusBadRequest).JSON(core.ErrorResponse{Error: "Missing required fields"})
		}

		if err := validateRequest(req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(core.ErrorResponse{Error: err.Error()})
		}

		nb, err := reg.Add(req)
		if err != nil {
			return c.Status(fiber.StatusConflict).JSON(core.ErrorResponse{Error: err.Error()})
		}

		return c.Status(fiber.StatusCreated).JSON(core.NotebookResponse{Notebook: nb})
	}
}

func putNotebook(reg core.Registry) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("id")
		var req core.CreateUpdateNotebookRequest
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(core.ErrorResponse{Error: "Invalid request"})
		}

		if err := validateRequest(req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(core.ErrorResponse{Error: err.Error()})
		}

		nb, err := reg.Update(id, req)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(core.ErrorResponse{Error: err.Error()})
		}

		return c.JSON(core.NotebookResponse{Notebook: nb})
	}
}

func deleteNotebook(reg core.Registry) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("id")
		if err := reg.Delete(id); err != nil {
			return c.Status(fiber.StatusNotFound).JSON(core.ErrorResponse{Error: err.Error()})
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}
