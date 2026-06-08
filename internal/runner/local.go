package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

type LocalRunner struct{}

func NewLocalRunner() *LocalRunner {
	return &LocalRunner{}
}

func (r *LocalRunner) Name() string { return "local" }

func (r *LocalRunner) Run(ctx context.Context, req *Request) (*Result, error) {
	shell := "bash"
	if cfg, ok := req.Config["shell"]; ok {
		if s, ok := cfg.(string); ok {
			shell = s
		}
	}

	cmd := exec.CommandContext(ctx, shell, "-c", req.Command)

	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}

	cmd.Env = buildEnv(req.Env)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	err := cmd.Wait()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("wait command: %w", err)
		}
	}

	return &Result{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

func buildEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

func splitCommand(cmd string) (string, []string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}
