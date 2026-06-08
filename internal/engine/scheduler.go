package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowctl/flowctl/internal/config"
)

type Scheduler struct {
	pool   *pgxpool.Pool
	cfg    config.SchedulerConfig
	nodeID string
	logger *slog.Logger

	executor *Executor

	mu       sync.Mutex
	running  map[uuid.UUID]context.CancelFunc
	shutdown chan struct{}
}

func NewScheduler(pool *pgxpool.Pool, cfg config.SchedulerConfig, executor *Executor, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		pool:     pool,
		cfg:      cfg,
		nodeID:   cfg.NodeID,
		logger:   logger,
		executor: executor,
		running:  make(map[uuid.UUID]context.CancelFunc),
		shutdown: make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info("scheduler starting", "node_id", s.nodeID)

	if err := s.registerNode(ctx); err != nil {
		return fmt.Errorf("register node: %w", err)
	}

	go s.heartbeatLoop(ctx)
	go s.takeoverLoop(ctx)
	go s.cronLoop(ctx)

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire listener conn: %w", err)
	}

	_, err = conn.Exec(ctx, "LISTEN execution_queued")
	if err != nil {
		conn.Release()
		return fmt.Errorf("listen execution_queued: %w", err)
	}
	_, err = conn.Exec(ctx, "LISTEN approval_responded")
	if err != nil {
		conn.Release()
		return fmt.Errorf("listen approval_responded: %w", err)
	}

	go s.listenLoop(ctx, conn)
	go s.pollLoop(ctx)

	s.logger.Info("scheduler started", "node_id", s.nodeID)
	return nil
}

func (s *Scheduler) Stop() {
	close(s.shutdown)
	s.mu.Lock()
	for id, cancel := range s.running {
		cancel()
		delete(s.running, id)
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.deregisterNode(ctx)
}

func (s *Scheduler) listenLoop(ctx context.Context, conn *pgxpool.Conn) {
	defer conn.Release()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		default:
		}

		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Error("notification error", "error", err)
			time.Sleep(time.Second)
			continue
		}

		switch notification.Channel {
		case "execution_queued":
			go s.handleNewExecution(ctx, notification.Payload)
		case "approval_responded":
			go s.handleApprovalResponse(ctx, notification.Payload)
		}
	}
}

func (s *Scheduler) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.pollForWork(ctx)
		}
	}
}

func (s *Scheduler) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-ticker.C:
			_, err := s.pool.Exec(ctx,
				"UPDATE scheduler_nodes SET heartbeat_at = now() WHERE id = $1", s.nodeID)
			if err != nil {
				s.logger.Error("heartbeat failed", "error", err)
			}
		}
	}
}

func (s *Scheduler) takeoverLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.performTakeover(ctx)
		}
	}
}

func (s *Scheduler) cronLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.processCronSchedules(ctx)
		}
	}
}

func (s *Scheduler) handleNewExecution(ctx context.Context, payload string) {
	execID, err := uuid.Parse(payload)
	if err != nil {
		s.logger.Error("invalid execution_queued payload", "payload", payload)
		return
	}

	acquired, err := s.tryAcquireExecution(ctx, execID)
	if err != nil {
		s.logger.Error("acquire execution failed", "execution_id", execID, "error", err)
		return
	}
	if !acquired {
		return
	}

	s.runExecution(ctx, execID)
}

func (s *Scheduler) handleApprovalResponse(ctx context.Context, payload string) {
	execID, err := uuid.Parse(payload)
	if err != nil {
		return
	}

	s.mu.Lock()
	_, owns := s.running[execID]
	s.mu.Unlock()

	if owns {
		s.logger.Info("approval responded, resuming execution", "execution_id", execID)
	}
}

func (s *Scheduler) pollForWork(ctx context.Context) {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM executions
		 WHERE status = 'queued' AND scheduler_node IS NULL
		 ORDER BY created_at ASC LIMIT 10`)
	if err != nil {
		s.logger.Error("poll for work failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var execID uuid.UUID
		if err := rows.Scan(&execID); err != nil {
			continue
		}

		acquired, err := s.tryAcquireExecution(ctx, execID)
		if err != nil || !acquired {
			continue
		}

		go s.runExecution(ctx, execID)
	}
}

func (s *Scheduler) performTakeover(ctx context.Context) {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM scheduler_nodes
		 WHERE heartbeat_at < now() - $1::interval AND id != $2`,
		s.cfg.TakeoverThreshold.String(), s.nodeID)
	if err != nil {
		s.logger.Error("takeover query failed", "error", err)
		return
	}
	defer rows.Close()

	var deadNodes []string
	for rows.Next() {
		var nodeID string
		if err := rows.Scan(&nodeID); err == nil {
			deadNodes = append(deadNodes, nodeID)
		}
	}

	for _, deadNode := range deadNodes {
		s.logger.Warn("taking over from dead node", "dead_node", deadNode)

		_, err := s.pool.Exec(ctx,
			`UPDATE executions SET scheduler_node = NULL, status = 'queued'
			 WHERE scheduler_node = $1 AND status IN ('running', 'queued')`,
			deadNode)
		if err != nil {
			s.logger.Error("reassign executions failed", "dead_node", deadNode, "error", err)
			continue
		}

		s.pool.Exec(ctx, "DELETE FROM scheduler_nodes WHERE id = $1", deadNode)
	}
}

func (s *Scheduler) processCronSchedules(ctx context.Context) {
	rows, err := s.pool.Query(ctx,
		`UPDATE cron_schedules
		 SET last_run_at = now()
		 WHERE enabled = true AND next_run_at <= now()
		 RETURNING id, tenant_id, workflow_id, inputs`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			schedID    uuid.UUID
			tenantID   uuid.UUID
			workflowID uuid.UUID
			inputs     map[string]any
		)
		if err := rows.Scan(&schedID, &tenantID, &workflowID, &inputs); err != nil {
			continue
		}

		s.logger.Info("cron trigger", "schedule_id", schedID, "workflow_id", workflowID)
		s.triggerCronExecution(ctx, tenantID, workflowID, inputs)
	}
}

func (s *Scheduler) triggerCronExecution(ctx context.Context, tenantID, workflowID uuid.UUID, inputs map[string]any) {
	var versionID uuid.UUID
	err := s.pool.QueryRow(ctx,
		"SELECT active_version_id FROM workflows WHERE id = $1 AND tenant_id = $2",
		workflowID, tenantID).Scan(&versionID)
	if err != nil {
		s.logger.Error("cron: get active version failed", "error", err)
		return
	}

	execID := uuid.New()
	_, err = s.pool.Exec(ctx,
		`INSERT INTO executions (id, tenant_id, workflow_id, workflow_version_id, status, inputs, trigger_type, created_at)
		 VALUES ($1, $2, $3, $4, 'queued', $5, 'cron', now())`,
		execID, tenantID, workflowID, versionID, inputs)
	if err != nil {
		s.logger.Error("cron: create execution failed", "error", err)
	}
}

func (s *Scheduler) tryAcquireExecution(ctx context.Context, executionID uuid.UUID) (bool, error) {
	lockKey := int64(binary.BigEndian.Uint64(executionID[:8]))
	var acquired bool
	err := s.pool.QueryRow(ctx,
		"SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE executions SET scheduler_node = $1, lock_acquired_at = now()
		 WHERE id = $2 AND (scheduler_node IS NULL OR scheduler_node = $1)`,
		s.nodeID, executionID)
	if err != nil || tag.RowsAffected() == 0 {
		s.releaseAdvisoryLock(ctx, lockKey)
		return false, err
	}

	return true, nil
}

func (s *Scheduler) releaseAdvisoryLock(ctx context.Context, key int64) {
	s.pool.Exec(ctx, "SELECT pg_advisory_unlock($1)", key)
}

func (s *Scheduler) runExecution(ctx context.Context, execID uuid.UUID) {
	execCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	s.running[execID] = cancel
	s.mu.Unlock()

	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.running, execID)
		s.mu.Unlock()

		lockKey := int64(binary.BigEndian.Uint64(execID[:8]))
		s.releaseAdvisoryLock(context.Background(), lockKey)
	}()

	err := s.executor.Execute(execCtx, execID)
	if err != nil {
		s.logger.Error("execution failed", "execution_id", execID, "error", err)
	}
}

func (s *Scheduler) registerNode(ctx context.Context) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO scheduler_nodes (id, heartbeat_at, started_at, metadata)
		 VALUES ($1, now(), now(), '{}')
		 ON CONFLICT (id) DO UPDATE SET heartbeat_at = now(), started_at = now()`,
		s.nodeID)
	return err
}

func (s *Scheduler) deregisterNode(ctx context.Context) {
	s.pool.Exec(ctx, "DELETE FROM scheduler_nodes WHERE id = $1", s.nodeID)
}

func (s *Scheduler) CheckTenantConcurrency(ctx context.Context, tenantID uuid.UUID, limit int) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM executions
		 WHERE tenant_id = $1 AND status = 'running'`, tenantID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count < limit, nil
}

// GetRunningCount returns the number of active executions on this node
func (s *Scheduler) GetRunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running)
}
