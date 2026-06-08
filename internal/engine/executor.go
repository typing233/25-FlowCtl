package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowctl/flowctl/internal/huml"
	"github.com/flowctl/flowctl/internal/model"
	"github.com/flowctl/flowctl/internal/runner"
)

type Executor struct {
	pool          *pgxpool.Pool
	runnerFactory *runner.Factory
	logger        *slog.Logger
}

func NewExecutor(pool *pgxpool.Pool, factory *runner.Factory, logger *slog.Logger) *Executor {
	return &Executor{
		pool:          pool,
		runnerFactory: factory,
		logger:        logger,
	}
}

func (e *Executor) Execute(ctx context.Context, executionID uuid.UUID) error {
	exec, def, err := e.loadExecution(ctx, executionID)
	if err != nil {
		return fmt.Errorf("load execution: %w", err)
	}

	dag, err := BuildDAG(def)
	if err != nil {
		return fmt.Errorf("build DAG: %w", err)
	}

	if err := e.transitionExecution(ctx, exec, model.ExecutionStatusRunning); err != nil {
		return err
	}

	completed := e.loadCompletedSteps(exec)
	evalCtx := e.buildEvalContext(exec, completed)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ready := dag.GetReadyNodes(completedSet(completed))
		if len(ready) == 0 {
			if len(completed) == len(dag.Nodes) {
				return e.transitionExecution(ctx, exec, model.ExecutionStatusSucceeded)
			}
			return e.transitionExecution(ctx, exec, model.ExecutionStatusFailed)
		}

		for _, nodeID := range ready {
			node := dag.Nodes[nodeID]

			switch node.Type {
			case "step":
				result, err := e.executeStep(ctx, exec, node.StepDef, evalCtx)
				if err != nil {
					e.handleStepFailure(ctx, exec, node.StepDef, err)
					return e.transitionExecution(ctx, exec, model.ExecutionStatusFailed)
				}
				completed[nodeID] = *result
				evalCtx.Steps[nodeID] = result.Outputs

			case "approval":
				approved, err := e.handleApproval(ctx, exec, node.Approval, evalCtx)
				if err != nil {
					return fmt.Errorf("approval %s: %w", nodeID, err)
				}
				if !approved {
					return e.transitionExecution(ctx, exec, model.ExecutionStatusCancelled)
				}
				completed[nodeID] = model.StepResult{
					StepID:     nodeID,
					Status:     model.StepStatusSucceeded,
					FinishedAt: time.Now(),
				}
			}

			if err := e.saveCheckpoint(ctx, exec, completed); err != nil {
				e.logger.Error("save checkpoint failed", "error", err)
			}
		}
	}
}

func (e *Executor) executeStep(ctx context.Context, exec *model.Execution, stepDef *model.StepDef, evalCtx *huml.EvalContext) (*model.StepResult, error) {
	if stepDef.When != "" {
		condResult, err := huml.EvaluateString(stepDef.When, evalCtx)
		if err != nil {
			return nil, fmt.Errorf("evaluate condition: %w", err)
		}
		if condResult == "false" || condResult == "" {
			return &model.StepResult{
				StepID:     stepDef.ID,
				Status:     model.StepStatusSkipped,
				FinishedAt: time.Now(),
			}, nil
		}
	}

	command, err := huml.EvaluateString(stepDef.Command, evalCtx)
	if err != nil {
		return nil, fmt.Errorf("evaluate command: %w", err)
	}

	stepID := uuid.New()
	now := time.Now()
	_, err = e.pool.Exec(ctx,
		`INSERT INTO execution_steps (id, execution_id, step_id, status, runner_type, config, attempt, max_retries, timeout_seconds, started_at)
		 VALUES ($1, $2, $3, 'running', $4, $5, 1, $6, $7, $8)`,
		stepID, exec.ID, stepDef.ID, stepDef.Runner, stepDef.Config,
		retryCount(stepDef.Retry), timeoutSeconds(stepDef.Timeout), now)
	if err != nil {
		return nil, fmt.Errorf("insert step: %w", err)
	}

	timeout := parseDuration(stepDef.Timeout, 5*time.Minute)
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r, err := e.runnerFactory.Get(stepDef.Runner)
	if err != nil {
		return nil, fmt.Errorf("get runner %q: %w", stepDef.Runner, err)
	}

	runReq := &runner.Request{
		ExecutionID: exec.ID,
		StepID:      stepDef.ID,
		Command:     command,
		Image:       evaluateStr(stepDef.Image, evalCtx),
		Host:        evaluateStr(stepDef.Host, evalCtx),
		Env:         evaluateEnv(stepDef.Env, evalCtx),
		Config:      stepDef.Config,
	}

	var result *runner.Result
	maxAttempts := 1
	if stepDef.Retry != nil {
		maxAttempts = stepDef.Retry.MaxAttempts
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err = r.Run(stepCtx, runReq)
		if err == nil && result.ExitCode == 0 {
			break
		}

		if attempt < maxAttempts {
			e.logger.Info("step retrying", "step_id", stepDef.ID, "attempt", attempt)
			backoff := calculateBackoff(attempt, stepDef.Retry)
			select {
			case <-stepCtx.Done():
				return nil, stepCtx.Err()
			case <-time.After(backoff):
			}
		}
	}

	finishedAt := time.Now()
	if err != nil || (result != nil && result.ExitCode != 0) {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		} else if result != nil {
			errMsg = fmt.Sprintf("exit code %d", result.ExitCode)
		}

		e.pool.Exec(ctx,
			`UPDATE execution_steps SET status = 'failed', error = $1, finished_at = $2
			 WHERE id = $3`, errMsg, finishedAt, stepID)

		if stepDef.Compensation != nil {
			e.runCompensation(ctx, exec, stepDef, evalCtx)
		}

		return nil, fmt.Errorf("step %s failed: %s", stepDef.ID, errMsg)
	}

	outputs := make(map[string]any)
	if result != nil {
		outputs["stdout"] = result.Stdout
		outputs["stderr"] = result.Stderr
		outputs["exit_code"] = result.ExitCode
	}

	e.pool.Exec(ctx,
		`UPDATE execution_steps SET status = 'succeeded', outputs = $1, finished_at = $2
		 WHERE id = $3`, outputs, finishedAt, stepID)

	return &model.StepResult{
		StepID:     stepDef.ID,
		Status:     model.StepStatusSucceeded,
		Outputs:    outputs,
		FinishedAt: finishedAt,
	}, nil
}

func (e *Executor) handleApproval(ctx context.Context, exec *model.Execution, approval *model.ApprovalDef, evalCtx *huml.EvalContext) (bool, error) {
	if approval.When != "" {
		condResult, err := huml.EvaluateString(approval.When, evalCtx)
		if err != nil {
			return false, err
		}
		if condResult == "false" || condResult == "" {
			return true, nil
		}
	}

	approvalID := uuid.New()
	_, err := e.pool.Exec(ctx,
		`INSERT INTO approvals (id, execution_id, step_id, tenant_id, status, required_roles, requested_at)
		 VALUES ($1, $2, $3, $4, 'pending', $5, now())`,
		approvalID, exec.ID, approval.ID, exec.TenantID, approval.RequiredRoles)
	if err != nil {
		return false, fmt.Errorf("create approval: %w", err)
	}

	e.transitionExecution(ctx, exec, model.ExecutionStatusWaitingApproval)

	listenConn, err := e.pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire listen conn: %w", err)
	}
	defer listenConn.Release()

	_, err = listenConn.Exec(ctx, "LISTEN approval_responded")
	if err != nil {
		return false, fmt.Errorf("listen: %w", err)
	}

	// Check immediately in case approval was already responded before we started listening
	if approved, resolved := e.checkApprovalStatus(ctx, approvalID); resolved {
		if approved {
			e.transitionExecution(ctx, exec, model.ExecutionStatusRunning)
			return true, nil
		}
		return false, nil
	}

	for {
		// Wait for a notification with a timeout as fallback
		waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
		notification, err := listenConn.Conn().WaitForNotification(waitCtx)
		waitCancel()

		if ctx.Err() != nil {
			return false, ctx.Err()
		}

		if err == nil && notification != nil {
			// Parse payload to see if it's for our execution
			e.logger.Debug("approval notification received", "payload", notification.Payload)
		}

		// Check regardless (notification might be for us, or timeout as fallback poll)
		approved, resolved := e.checkApprovalStatus(ctx, approvalID)
		if !resolved {
			continue
		}
		if approved {
			e.transitionExecution(ctx, exec, model.ExecutionStatusRunning)
			return true, nil
		}
		return false, nil
	}
}

func (e *Executor) checkApprovalStatus(ctx context.Context, approvalID uuid.UUID) (approved bool, resolved bool) {
	var status string
	err := e.pool.QueryRow(ctx,
		"SELECT status FROM approvals WHERE id = $1", approvalID).Scan(&status)
	if err != nil {
		return false, false
	}
	switch status {
	case "approved":
		return true, true
	case "rejected":
		return false, true
	default:
		return false, false
	}
}

func (e *Executor) runCompensation(ctx context.Context, exec *model.Execution, stepDef *model.StepDef, evalCtx *huml.EvalContext) {
	comp := stepDef.Compensation
	command, _ := huml.EvaluateString(comp.Command, evalCtx)

	runnerName := comp.Runner
	if runnerName == "" {
		runnerName = stepDef.Runner
	}

	r, err := e.runnerFactory.Get(runnerName)
	if err != nil {
		e.logger.Error("compensation runner not found", "runner", runnerName, "error", err)
		return
	}

	compCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	_, err = r.Run(compCtx, &runner.Request{
		ExecutionID: exec.ID,
		StepID:      stepDef.ID + "_compensation",
		Command:     command,
		Config:      comp.Config,
	})
	if err != nil {
		e.logger.Error("compensation failed", "step_id", stepDef.ID, "error", err)
	}
}

func (e *Executor) loadExecution(ctx context.Context, executionID uuid.UUID) (*model.Execution, *model.WorkflowDefinition, error) {
	exec := &model.Execution{}
	var defJSON []byte
	var checkpointJSON []byte

	err := e.pool.QueryRow(ctx,
		`SELECT e.id, e.tenant_id, e.workflow_id, e.workflow_version_id, e.status,
		        e.inputs, e.context, e.trigger_type, e.checkpoint,
		        wv.definition
		 FROM executions e
		 JOIN workflow_versions wv ON wv.id = e.workflow_version_id
		 WHERE e.id = $1`, executionID).Scan(
		&exec.ID, &exec.TenantID, &exec.WorkflowID, &exec.WorkflowVersionID,
		&exec.Status, &exec.Inputs, &exec.Context, &exec.TriggerType,
		&checkpointJSON, &defJSON)
	if err != nil {
		return nil, nil, err
	}

	var def model.WorkflowDefinition
	if err := json.Unmarshal(defJSON, &def); err != nil {
		return nil, nil, fmt.Errorf("unmarshal definition: %w", err)
	}

	if len(checkpointJSON) > 0 {
		var cp model.Checkpoint
		json.Unmarshal(checkpointJSON, &cp)
		exec.Checkpoint = &cp
	}

	return exec, &def, nil
}

func (e *Executor) transitionExecution(ctx context.Context, exec *model.Execution, to model.ExecutionStatus) error {
	if err := ValidateExecutionTransition(exec.Status, to); err != nil {
		return err
	}

	now := time.Now()
	var finishedAt *time.Time
	switch to {
	case model.ExecutionStatusSucceeded, model.ExecutionStatusFailed, model.ExecutionStatusCancelled:
		finishedAt = &now
	}

	var startedAt *time.Time
	if to == model.ExecutionStatusRunning && exec.StartedAt == nil {
		startedAt = &now
	}

	_, err := e.pool.Exec(ctx,
		`UPDATE executions SET status = $1, started_at = COALESCE($2, started_at), finished_at = $3
		 WHERE id = $4`,
		to, startedAt, finishedAt, exec.ID)
	if err != nil {
		return err
	}

	exec.Status = to
	return nil
}

func (e *Executor) saveCheckpoint(ctx context.Context, exec *model.Execution, completed map[string]model.StepResult) error {
	cp := model.Checkpoint{
		CompletedSteps: completed,
		Context:        exec.Context,
		Version:        1,
	}
	data, _ := json.Marshal(cp)
	_, err := e.pool.Exec(ctx, "UPDATE executions SET checkpoint = $1 WHERE id = $2", data, exec.ID)
	return err
}

func (e *Executor) loadCompletedSteps(exec *model.Execution) map[string]model.StepResult {
	if exec.Checkpoint != nil && exec.Checkpoint.CompletedSteps != nil {
		return exec.Checkpoint.CompletedSteps
	}
	return make(map[string]model.StepResult)
}

func (e *Executor) buildEvalContext(exec *model.Execution, completed map[string]model.StepResult) *huml.EvalContext {
	ctx := huml.NewEvalContext()
	if exec.Inputs != nil {
		ctx.Inputs = exec.Inputs
	}
	for stepID, result := range completed {
		if result.Outputs != nil {
			ctx.Steps[stepID] = result.Outputs
		}
	}
	return ctx
}

func (e *Executor) handleStepFailure(ctx context.Context, exec *model.Execution, stepDef *model.StepDef, err error) {
	e.logger.Error("step failed", "execution_id", exec.ID, "step_id", stepDef.ID, "error", err)

	e.pool.Exec(ctx,
		`INSERT INTO audit_logs (tenant_id, action, resource, details, timestamp)
		 VALUES ($1, 'step.failed', $2, $3, now())`,
		exec.TenantID,
		fmt.Sprintf("execution:%s/step:%s", exec.ID, stepDef.ID),
		map[string]any{"error": err.Error()})
}

// Helpers

func completedSet(completed map[string]model.StepResult) map[string]bool {
	set := make(map[string]bool, len(completed))
	for k := range completed {
		set[k] = true
	}
	return set
}

func retryCount(retry *model.RetryPolicy) int {
	if retry == nil {
		return 0
	}
	return retry.MaxAttempts
}

func timeoutSeconds(timeout string) int {
	d := parseDuration(timeout, 5*time.Minute)
	return int(d.Seconds())
}

func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}

func calculateBackoff(attempt int, retry *model.RetryPolicy) time.Duration {
	if retry == nil {
		return time.Second
	}
	base := parseDuration(retry.InitialWait, time.Second)
	switch retry.Backoff {
	case "exponential":
		mult := time.Duration(1)
		for i := 0; i < attempt; i++ {
			mult *= 2
		}
		d := base * mult
		max := parseDuration(retry.MaxWait, 5*time.Minute)
		if d > max {
			return max
		}
		return d
	case "linear":
		return base * time.Duration(attempt)
	default:
		return base
	}
}

func evaluateStr(s string, ctx *huml.EvalContext) string {
	if s == "" {
		return ""
	}
	result, _ := huml.EvaluateString(s, ctx)
	return result
}

func evaluateEnv(env map[string]string, ctx *huml.EvalContext) map[string]string {
	if env == nil {
		return nil
	}
	result := make(map[string]string, len(env))
	for k, v := range env {
		result[k] = evaluateStr(v, ctx)
	}
	return result
}
