package core

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

type Runner struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.RWMutex
	managers map[string]*NotebookManager
	nextPort atomic.Int64
}

func NewRunner(ctx context.Context) *Runner {
	ctx, cancel := context.WithCancel(ctx)
	r := &Runner{
		ctx:      ctx,
		cancel:   cancel,
		managers: make(map[string]*NotebookManager),
	}
	r.nextPort.Store(3000)
	return r
}

func (r *Runner) HandleRegistryEvent(nb Notebook, action RegistryAction) {
	log.Debug().Str("method", "Runner.HandleRegistryEvent").
		Interface("notebook", nb).
		Interface("action", action).
		Msg("Handling registry event")
	switch action {
	case ActionAdd, ActionUpdate:
		r.handleNotebook(nb)
	case ActionDelete:
		r.mu.Lock()
		if manager, exists := r.managers[nb.ID]; exists {
			manager.stop()
			delete(r.managers, nb.ID)
		}
		r.mu.Unlock()
	}
}

func (r *Runner) handleNotebook(nb Notebook) {
	r.mu.Lock()
	if existingManager, exists := r.managers[nb.ID]; exists {
		log.Debug().Str("method", "Runner.handleNotebook").
			Str("notebook", nb.ID).
			Msg("Updating notebook")
		if err := existingManager.update(nb); err != nil {
			log.Error().Str("method", "Runner.handleNotebook").
				Str("notebook", nb.ID).
				Err(err).
				Msg("Failed to update notebook")
		}
		r.mu.Unlock()
		return
	}

	port := int(r.nextPort.Add(1))
	newManager := &NotebookManager{
		notebook: nb,
		port:     port,
		ctx:      r.ctx,
	}
	r.managers[nb.ID] = newManager
	r.mu.Unlock()

	if err := newManager.start(); err != nil {
	}
}

func (r *Runner) Stop() {

	r.cancel()

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, manager := range r.managers {
		_ = id
		manager.stop()
	}
}

func (r *Runner) GetStatus(id string) (Status, error) {
	r.mu.RLock()
	manager, exists := r.managers[id]
	r.mu.RUnlock()

	if !exists {
		return StatusStopped, &NotRunningError{ID: id}
	}

	return manager.getStatus(), nil
}

func (r *Runner) GetPort(id string) (int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	manager, exists := r.managers[id]
	if !exists {
		return 0, false
	}
	return manager.port, true
}

//--- NotebookManager ---//

type NotebookManager struct {
	notebook Notebook
	port     int
	ctx      context.Context
	cmd      *exec.Cmd
	status   Status
	mu       sync.RWMutex
}

func (m *NotebookManager) update(nb Notebook) error {
	m.mu.Lock()
	needsRestart := m.cmd != nil
	m.notebook = nb
	m.mu.Unlock()

	if needsRestart {
		if err := m.stop(); err != nil {
			return err
		}
		return m.start()
	}
	return nil
}

func (m *NotebookManager) stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == nil {
		return &NotRunningError{ID: m.notebook.ID}
	}

	if err := m.cmd.Process.Kill(); err != nil {
		return &ProcessKillError{PID: m.cmd.Process.Pid, Err: err}
	}

	m.cmd = nil
	m.status = StatusStopped
	log.Debug().Str("method", "NotebookManager.stop").
		Str("notebook", m.notebook.ID).
		Msg("Notebook stopped")
	return nil
}

func (m *NotebookManager) start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil {
		return &AlreadyRunningError{ID: m.notebook.ID}
	}

	cmd := exec.CommandContext(m.ctx, "marimo", "run", m.notebook.Path,
		"--port", fmt.Sprintf("%d", m.port),
		"--host", "0.0.0.0",
		"--headless",
		"--no-token")
	if m.notebook.Watch {
		cmd.Args = append(cmd.Args, "--watch")
	}
	if m.notebook.ShowCode {
		cmd.Args = append(cmd.Args, "--include-code")
	}

	if err := cmd.Start(); err != nil {
		m.status = StatusError
		return &ExecError{Command: "marimo run", Err: err}
	}

	log.Debug().Str("method", "NotebookManager.start").
		Str("notebook", m.notebook.ID).
		Str("command", strings.Join(cmd.Args, " ")).
		Msg("Notebook started")

	m.cmd = cmd
	m.status = StatusRunning

	go m.monitor()
	return nil
}

func (m *NotebookManager) getStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *NotebookManager) monitor() {
	log.Debug().Str("method", "NotebookManager.monitor").
		Str("notebook", m.notebook.ID).
		Msg("Monitoring notebook")
	err := m.cmd.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()

	if err != nil && err.Error() != "signal: killed" {
		m.status = StatusError
		log.Error().Str("method", "NotebookManager.monitor").
			Str("notebook", m.notebook.ID).
			Err(err).
			Msg("Notebook failed")
	} else {
		m.status = StatusStopped
		log.Debug().Str("method", "NotebookManager.monitor").
			Str("notebook", m.notebook.ID).
			Msg("Notebook stopped")
	}

	m.cmd = nil
}
