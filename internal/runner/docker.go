package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type DockerRunner struct{}

func NewDockerRunner() *DockerRunner {
	return &DockerRunner{}
}

func (r *DockerRunner) Name() string { return "docker" }

func (r *DockerRunner) Run(ctx context.Context, req *Request) (*Result, error) {
	image := req.Image
	if image == "" {
		return nil, fmt.Errorf("docker runner requires 'image' field")
	}

	pullCmd := exec.CommandContext(ctx, "docker", "pull", image)
	pullCmd.Run()

	args := []string{"run", "--rm"}

	args = append(args, "--name", fmt.Sprintf("flowctl-%s-%s", req.ExecutionID.String()[:8], req.StepID))

	if mem, ok := req.Config["memory_limit"]; ok {
		if memStr, ok := mem.(string); ok {
			args = append(args, "--memory", memStr)
		}
	}

	if cpus, ok := req.Config["cpu_limit"]; ok {
		if cpuFloat, ok := cpus.(float64); ok {
			args = append(args, "--cpus", fmt.Sprintf("%.2f", cpuFloat))
		}
	}

	network := "none"
	if n, ok := req.Config["network"]; ok {
		if netStr, ok := n.(string); ok {
			network = netStr
		}
	}
	args = append(args, "--network", network)

	for k, v := range req.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	for k, v := range req.Secrets {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	if workDir, ok := req.Config["workdir"]; ok {
		if wd, ok := workDir.(string); ok {
			args = append(args, "-w", wd)
		}
	}

	if volumes, ok := req.Config["volumes"]; ok {
		if vols, ok := volumes.([]any); ok {
			for _, vol := range vols {
				if v, ok := vol.(string); ok {
					args = append(args, "-v", v)
				}
			}
		}
	}

	args = append(args, image, "sh", "-c", req.Command)

	cmd := exec.CommandContext(ctx, "docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("docker run: %w", err)
		}
	}

	return &Result{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

func parseMemoryLimit(s string) int64 {
	s = strings.TrimSpace(s)
	multiplier := int64(1)
	if strings.HasSuffix(s, "g") || strings.HasSuffix(s, "G") {
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "m") || strings.HasSuffix(s, "M") {
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	}
	val := int64(0)
	for _, c := range s {
		if c >= '0' && c <= '9' {
			val = val*10 + int64(c-'0')
		}
	}
	return val * multiplier
}
