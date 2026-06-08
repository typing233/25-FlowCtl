package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var executionCmd = &cobra.Command{
	Use:     "execution",
	Aliases: []string{"exec"},
	Short:   "Manage workflow executions",
}

func init() {
	// execution list
	listExecCmd := &cobra.Command{
		Use:   "list",
		Short: "List executions",
		RunE:  runExecutionList,
	}
	listExecCmd.Flags().String("status", "", "Filter by status (pending, running, succeeded, failed, cancelled)")

	// execution status
	statusCmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Show execution status with step details",
		Args:  cobra.ExactArgs(1),
		RunE:  runExecutionStatus,
	}

	// execution logs
	logsCmd := &cobra.Command{
		Use:   "logs <id>",
		Short: "Stream execution logs",
		Args:  cobra.ExactArgs(1),
		RunE:  runExecutionLogs,
	}
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output (poll for updates)")

	// execution cancel
	cancelCmd := &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a running execution",
		Args:  cobra.ExactArgs(1),
		RunE:  runExecutionCancel,
	}

	// execution retry
	retryCmd := &cobra.Command{
		Use:   "retry <id>",
		Short: "Retry a failed execution",
		Args:  cobra.ExactArgs(1),
		RunE:  runExecutionRetry,
	}

	executionCmd.AddCommand(listExecCmd, statusCmd, logsCmd, cancelCmd, retryCmd)
}

func runExecutionList(cmd *cobra.Command, args []string) error {
	client := NewAPIClient()
	path := "/api/v1/executions"

	status, _ := cmd.Flags().GetString("status")
	if status != "" {
		path += "?status=" + status
	}

	var executions []map[string]interface{}
	if err := client.Get(path, &executions); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(executions)
	}

	headers := []string{"ID", "WORKFLOW", "STATUS", "TRIGGER", "STARTED", "FINISHED"}
	rows := make([][]string, 0, len(executions))
	for _, e := range executions {
		started := ""
		if e["started_at"] != nil {
			started = fmt.Sprintf("%v", e["started_at"])
		}
		finished := ""
		if e["finished_at"] != nil {
			finished = fmt.Sprintf("%v", e["finished_at"])
		}
		rows = append(rows, []string{
			fmt.Sprintf("%v", e["id"]),
			fmt.Sprintf("%v", e["workflow_id"]),
			fmt.Sprintf("%v", e["status"]),
			fmt.Sprintf("%v", e["trigger_type"]),
			started,
			finished,
		})
	}
	out.PrintTable(headers, rows)
	return nil
}

func runExecutionStatus(cmd *cobra.Command, args []string) error {
	id := args[0]
	client := NewAPIClient()

	var execution map[string]interface{}
	if err := client.Get(fmt.Sprintf("/api/v1/executions/%s", id), &execution); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(execution)
	}

	fmt.Printf("Execution: %v\n", execution["id"])
	fmt.Printf("Status:    %v\n", execution["status"])
	fmt.Printf("Workflow:  %v\n", execution["workflow_id"])
	fmt.Printf("Trigger:   %v\n", execution["trigger_type"])
	fmt.Printf("Started:   %v\n", execution["started_at"])
	fmt.Printf("Finished:  %v\n", execution["finished_at"])

	// Print steps if available
	if steps, ok := execution["steps"]; ok {
		if stepList, ok := steps.([]interface{}); ok {
			fmt.Printf("\nSteps:\n")
			headers := []string{"STEP", "STATUS", "RUNNER", "ATTEMPT", "STARTED", "FINISHED"}
			rows := make([][]string, 0, len(stepList))
			for _, s := range stepList {
				step, ok := s.(map[string]interface{})
				if !ok {
					continue
				}
				started := ""
				if step["started_at"] != nil {
					started = fmt.Sprintf("%v", step["started_at"])
				}
				finished := ""
				if step["finished_at"] != nil {
					finished = fmt.Sprintf("%v", step["finished_at"])
				}
				rows = append(rows, []string{
					fmt.Sprintf("%v", step["step_id"]),
					fmt.Sprintf("%v", step["status"]),
					fmt.Sprintf("%v", step["runner_type"]),
					fmt.Sprintf("%v", step["attempt"]),
					started,
					finished,
				})
			}
			out.PrintTable(headers, rows)
		}
	}
	return nil
}

func runExecutionLogs(cmd *cobra.Command, args []string) error {
	id := args[0]
	follow, _ := cmd.Flags().GetBool("follow")
	client := NewAPIClient()

	// Try WebSocket streaming first by attempting an upgrade
	wsURL := strings.Replace(client.BaseURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL += fmt.Sprintf("/api/v1/executions/%s/logs/stream", id)

	// Attempt SSE/streaming endpoint
	resp, err := client.Do(http.MethodGet, fmt.Sprintf("/api/v1/executions/%s/logs", id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseAPIError(resp)
	}

	// Check if streaming response (content-type text/event-stream or chunked)
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") || follow {
		// Stream logs line by line
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				fmt.Println(strings.TrimPrefix(line, "data: "))
			} else if line != "" {
				fmt.Println(line)
			}
		}
		return scanner.Err()
	}

	// Non-streaming: JSON array of log lines
	var logs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return fmt.Errorf("decoding logs: %w", err)
	}

	for _, log := range logs {
		ts := ""
		if log["timestamp"] != nil {
			ts = fmt.Sprintf("[%v]", log["timestamp"])
		}
		stream := ""
		if log["stream"] != nil {
			stream = fmt.Sprintf("[%v]", log["stream"])
		}
		fmt.Printf("%s %s %v: %v\n", ts, stream, log["step_id"], log["line"])
	}

	// If follow mode and non-streaming response, poll
	if follow {
		lastID := ""
		if len(logs) > 0 {
			if id, ok := logs[len(logs)-1]["id"]; ok {
				lastID = fmt.Sprintf("%v", id)
			}
		}
		for {
			time.Sleep(2 * time.Second)
			path := fmt.Sprintf("/api/v1/executions/%s/logs", id)
			if lastID != "" {
				path += "?after=" + lastID
			}
			var newLogs []map[string]interface{}
			if err := client.Get(path, &newLogs); err != nil {
				return err
			}
			for _, log := range newLogs {
				ts := ""
				if log["timestamp"] != nil {
					ts = fmt.Sprintf("[%v]", log["timestamp"])
				}
				stream := ""
				if log["stream"] != nil {
					stream = fmt.Sprintf("[%v]", log["stream"])
				}
				fmt.Printf("%s %s %v: %v\n", ts, stream, log["step_id"], log["line"])
				if logID, ok := log["id"]; ok {
					lastID = fmt.Sprintf("%v", logID)
				}
			}
		}
	}

	return nil
}

func runExecutionCancel(cmd *cobra.Command, args []string) error {
	id := args[0]
	client := NewAPIClient()

	var result map[string]interface{}
	if err := client.Post(fmt.Sprintf("/api/v1/executions/%s/cancel", id), nil, &result); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(result)
	}

	fmt.Printf("Execution %s cancelled.\n", id)
	return nil
}

func runExecutionRetry(cmd *cobra.Command, args []string) error {
	id := args[0]
	client := NewAPIClient()

	var result map[string]interface{}
	if err := client.Post(fmt.Sprintf("/api/v1/executions/%s/retry", id), nil, &result); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(result)
	}

	fmt.Printf("Execution %s retried.\n", id)
	if newID, ok := result["id"]; ok {
		fmt.Printf("New execution ID: %v\n", newID)
	}
	return nil
}
