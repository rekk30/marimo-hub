package core

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
)

type Runner struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.RWMutex
	managers map[string]*NotebookManager
	nextPort int
}

func NewRunner(ctx context.Context) *Runner {
	ctx, cancel := context.WithCancel(ctx)
	return &Runner{
		ctx:      ctx,
		cancel:   cancel,
		managers: make(map[string]*NotebookManager),
		nextPort: 3000,
	}
}

func (r *Runner) HandleRegistryEvent(nb Notebook, action RegistryAction) {
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
		if err := existingManager.update(nb); err != nil {
		}
		r.mu.Unlock()
		return
	}

	port := r.nextPort
	r.nextPort++

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

	m.cmd = cmd
	m.status = StatusRunning

	go m.monitor()
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
	return nil
}

func (m *NotebookManager) update(nb Notebook) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.notebook = nb

	if m.cmd != nil {
		if err := m.stop(); err != nil {
			return err
		}
		return m.start()
	}

	return nil
}

func (m *NotebookManager) getStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *NotebookManager) monitor() {
	err := m.cmd.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()

	if err != nil {
		m.status = StatusError
		fmt.Printf("Notebook %s failed: %v\n", m.notebook.ID, err)
	} else {
		m.status = StatusStopped
	}

	m.cmd = nil
}
