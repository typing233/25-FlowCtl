package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler manages WebSocket connections for streaming execution logs.
type Handler struct {
	pool *pgxpool.Pool
}

// NewHandler creates a new WebSocket Handler.
func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

type logEntry struct {
	ID          int64     `json:"id"`
	ExecutionID uuid.UUID `json:"execution_id"`
	StepID      string    `json:"step_id"`
	Stream      string    `json:"stream"`
	Line        string    `json:"line"`
	Timestamp   time.Time `json:"timestamp"`
}

// StreamLogs upgrades the HTTP connection to a WebSocket and streams execution logs.
// It first sends all historical logs, then subscribes to pg_notify for new ones.
func (h *Handler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	executionIDStr := chi.URLParam(r, "executionID")
	executionID, err := uuid.Parse(executionIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid execution ID"}`, http.StatusBadRequest)
		return
	}

	// Accept WebSocket connection
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow any origin for development
	})
	if err != nil {
		slog.Error("failed to accept websocket", "error", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Send historical logs first
	lastID, err := h.sendHistoricalLogs(ctx, conn, executionID)
	if err != nil {
		slog.Error("failed to send historical logs", "error", err)
		conn.Close(websocket.StatusInternalError, "failed to send historical logs")
		return
	}

	// Subscribe to pg_notify channel for new logs
	pgConn, err := h.pool.Acquire(ctx)
	if err != nil {
		slog.Error("failed to acquire connection for listen", "error", err)
		conn.Close(websocket.StatusInternalError, "failed to subscribe")
		return
	}
	defer pgConn.Release()

	_, err = pgConn.Exec(ctx, "LISTEN step_log_inserted")
	if err != nil {
		slog.Error("failed to listen", "error", err)
		conn.Close(websocket.StatusInternalError, "failed to subscribe")
		return
	}

	// Stream new logs as they arrive
	for {
		notification, err := pgConn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// Client disconnected
				conn.Close(websocket.StatusNormalClosure, "client disconnected")
				return
			}
			slog.Error("error waiting for notification", "error", err)
			conn.Close(websocket.StatusInternalError, "notification error")
			return
		}

		// The payload could be the execution_id - check if it matches
		if notification.Payload != "" && notification.Payload != executionID.String() {
			continue
		}

		// Fetch new logs since lastID
		newLastID, err := h.sendNewLogs(ctx, conn, executionID, lastID)
		if err != nil {
			slog.Error("failed to send new logs", "error", err)
			conn.Close(websocket.StatusInternalError, "failed to send logs")
			return
		}
		if newLastID > lastID {
			lastID = newLastID
		}
	}
}

// sendHistoricalLogs sends all existing logs for the execution and returns the last log ID.
func (h *Handler) sendHistoricalLogs(ctx context.Context, conn *websocket.Conn, executionID uuid.UUID) (int64, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT id, execution_id, step_id, stream, line, timestamp
		 FROM step_logs WHERE execution_id = $1 ORDER BY id ASC`,
		executionID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var lastID int64
	for rows.Next() {
		var entry logEntry
		if err := rows.Scan(&entry.ID, &entry.ExecutionID, &entry.StepID, &entry.Stream, &entry.Line, &entry.Timestamp); err != nil {
			return lastID, err
		}
		lastID = entry.ID

		data, err := json.Marshal(entry)
		if err != nil {
			return lastID, err
		}
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			return lastID, err
		}
	}

	return lastID, rows.Err()
}

// sendNewLogs sends logs newer than lastID and returns the new last ID.
func (h *Handler) sendNewLogs(ctx context.Context, conn *websocket.Conn, executionID uuid.UUID, lastID int64) (int64, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT id, execution_id, step_id, stream, line, timestamp
		 FROM step_logs WHERE execution_id = $1 AND id > $2 ORDER BY id ASC`,
		executionID, lastID)
	if err != nil {
		return lastID, err
	}
	defer rows.Close()

	newLastID := lastID
	for rows.Next() {
		var entry logEntry
		if err := rows.Scan(&entry.ID, &entry.ExecutionID, &entry.StepID, &entry.Stream, &entry.Line, &entry.Timestamp); err != nil {
			return newLastID, err
		}
		newLastID = entry.ID

		data, err := json.Marshal(entry)
		if err != nil {
			return newLastID, err
		}
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			return newLastID, err
		}
	}

	return newLastID, rows.Err()
}
