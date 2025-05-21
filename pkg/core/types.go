package core

import (
	"time"
)

type RegistryAction string

const (
	ActionAdd    RegistryAction = "add"
	ActionUpdate RegistryAction = "update"
	ActionDelete RegistryAction = "delete"
)

type Status string

const (
	StatusPending    Status = "Pending"
	StatusRunning    Status = "Running"
	StatusStopped    Status = "Stopped"
	StatusError      Status = "Error"
	StatusRestarting Status = "Restarting"
)

type Notebook struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Domain    string    `json:"domain"`
	ShowCode  bool      `json:"show_code"`
	Watch     bool      `json:"watch"`
	CreatedAt time.Time `json:"created_at"`
}

type Registry interface {
	Add(nb CreateUpdateNotebookRequest) (Notebook, error)
	Get(id string) (Notebook, bool)
	GetByDomain(domain string) (Notebook, bool)
	List() []Notebook
	Update(id string, req CreateUpdateNotebookRequest) (Notebook, error)
	Delete(id string) error
}

// TODO: Think about separating create and update requests
type CreateUpdateNotebookRequest struct {
	Name     string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	Path     string `json:"path,omitempty" validate:"omitempty,filepath"`
	Domain   string `json:"domain,omitempty" validate:"omitempty,hostname"`
	ShowCode *bool  `json:"show_code,omitempty"`
	Watch    *bool  `json:"watch,omitempty"`
}

type NotebookResponse struct {
	Notebook Notebook `json:"notebook"`
}

type NotebooksResponse struct {
	Notebooks []Notebook `json:"notebooks"`
}

type StatusResponse struct {
	Status Status `json:"status"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
