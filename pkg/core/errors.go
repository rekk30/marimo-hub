package core

import (
	"fmt"
)

// Base error types
type NotFoundError struct {
	ID string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("notebook %s not found", e.ID)
}

type AlreadyRunningError struct {
	ID string
}

func (e *AlreadyRunningError) Error() string {
	return fmt.Sprintf("notebook %s already running", e.ID)
}

type NotRunningError struct {
	ID string
}

func (e *NotRunningError) Error() string {
	return fmt.Sprintf("notebook %s not running", e.ID)
}

type StartError struct {
	ID  string
	Err error
}

func (e *StartError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("failed to start notebook %s: %v", e.ID, e.Err)
	}
	return fmt.Sprintf("failed to start notebook %s", e.ID)
}

func (e *StartError) Unwrap() error {
	return e.Err
}

type StopError struct {
	ID  string
	Err error
}

func (e *StopError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("failed to stop notebook %s: %v", e.ID, e.Err)
	}
	return fmt.Sprintf("failed to stop notebook %s", e.ID)
}

func (e *StopError) Unwrap() error {
	return e.Err
}

type ReloadError struct {
	ID  string
	Err error
}

func (e *ReloadError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("failed to reload notebook %s: %v", e.ID, e.Err)
	}
	return fmt.Sprintf("failed to reload notebook %s", e.ID)
}

func (e *ReloadError) Unwrap() error {
	return e.Err
}

type ExecError struct {
	Command string
	Err     error
}

func (e *ExecError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("failed to execute command %s: %v", e.Command, e.Err)
	}
	return fmt.Sprintf("failed to execute command %s", e.Command)
}

func (e *ExecError) Unwrap() error {
	return e.Err
}

type ProcessKillError struct {
	PID int
	Err error
}

func (e *ProcessKillError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("failed to kill process %d: %v", e.PID, e.Err)
	}
	return fmt.Sprintf("failed to kill process %d", e.PID)
}

func (e *ProcessKillError) Unwrap() error {
	return e.Err
}
