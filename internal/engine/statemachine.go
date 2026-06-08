package engine

import (
	"fmt"

	"github.com/flowctl/flowctl/internal/model"
)

type Transition struct {
	From   model.ExecutionStatus
	To     model.ExecutionStatus
	Guard  func(*model.Execution) bool
}

type StepTransition struct {
	From  model.StepStatus
	To    model.StepStatus
	Guard func(*model.ExecutionStep) bool
}

var validExecutionTransitions = []Transition{
	{From: model.ExecutionStatusPending, To: model.ExecutionStatusQueued},
	{From: model.ExecutionStatusQueued, To: model.ExecutionStatusRunning},
	{From: model.ExecutionStatusRunning, To: model.ExecutionStatusSucceeded},
	{From: model.ExecutionStatusRunning, To: model.ExecutionStatusFailed},
	{From: model.ExecutionStatusRunning, To: model.ExecutionStatusWaitingApproval},
	{From: model.ExecutionStatusRunning, To: model.ExecutionStatusCancelled},
	{From: model.ExecutionStatusRunning, To: model.ExecutionStatusPaused},
	{From: model.ExecutionStatusWaitingApproval, To: model.ExecutionStatusRunning},
	{From: model.ExecutionStatusWaitingApproval, To: model.ExecutionStatusCancelled},
	{From: model.ExecutionStatusPaused, To: model.ExecutionStatusRunning},
	{From: model.ExecutionStatusPaused, To: model.ExecutionStatusCancelled},
	{From: model.ExecutionStatusFailed, To: model.ExecutionStatusRetrying},
	{From: model.ExecutionStatusRetrying, To: model.ExecutionStatusRunning},
	{From: model.ExecutionStatusRetrying, To: model.ExecutionStatusFailed},
	{From: model.ExecutionStatusPending, To: model.ExecutionStatusCancelled},
	{From: model.ExecutionStatusQueued, To: model.ExecutionStatusCancelled},
}

var validStepTransitions = []StepTransition{
	{From: model.StepStatusPending, To: model.StepStatusQueued},
	{From: model.StepStatusQueued, To: model.StepStatusRunning},
	{From: model.StepStatusRunning, To: model.StepStatusSucceeded},
	{From: model.StepStatusRunning, To: model.StepStatusFailed},
	{From: model.StepStatusRunning, To: model.StepStatusCancelled},
	{From: model.StepStatusRunning, To: model.StepStatusWaitingApproval},
	{From: model.StepStatusPending, To: model.StepStatusSkipped},
	{From: model.StepStatusFailed, To: model.StepStatusRetrying},
	{From: model.StepStatusRetrying, To: model.StepStatusRunning},
	{From: model.StepStatusWaitingApproval, To: model.StepStatusRunning},
	{From: model.StepStatusWaitingApproval, To: model.StepStatusCancelled},
}

func ValidateExecutionTransition(from, to model.ExecutionStatus) error {
	for _, t := range validExecutionTransitions {
		if t.From == from && t.To == to {
			return nil
		}
	}
	return fmt.Errorf("invalid execution transition: %s -> %s", from, to)
}

func ValidateStepTransition(from, to model.StepStatus) error {
	for _, t := range validStepTransitions {
		if t.From == from && t.To == to {
			return nil
		}
	}
	return fmt.Errorf("invalid step transition: %s -> %s", from, to)
}

type StateMachine struct {
	execution *model.Execution
}

func NewStateMachine(exec *model.Execution) *StateMachine {
	return &StateMachine{execution: exec}
}

func (sm *StateMachine) CanTransitionTo(to model.ExecutionStatus) bool {
	return ValidateExecutionTransition(sm.execution.Status, to) == nil
}

func (sm *StateMachine) TransitionTo(to model.ExecutionStatus) error {
	if err := ValidateExecutionTransition(sm.execution.Status, to); err != nil {
		return err
	}
	sm.execution.Status = to
	return nil
}

func (sm *StateMachine) IsTerminal() bool {
	switch sm.execution.Status {
	case model.ExecutionStatusSucceeded, model.ExecutionStatusFailed, model.ExecutionStatusCancelled:
		return true
	}
	return false
}

func (sm *StateMachine) IsActive() bool {
	switch sm.execution.Status {
	case model.ExecutionStatusRunning, model.ExecutionStatusQueued, model.ExecutionStatusRetrying:
		return true
	}
	return false
}
