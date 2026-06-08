package runner

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHRunner struct{}

func NewSSHRunner() *SSHRunner {
	return &SSHRunner{}
}

func (r *SSHRunner) Name() string { return "ssh" }

func (r *SSHRunner) Run(ctx context.Context, req *Request) (*Result, error) {
	host := req.Host
	if host == "" {
		return nil, fmt.Errorf("ssh runner requires 'host' field")
	}

	if !hasPort(host) {
		host = host + ":22"
	}

	cfg, err := r.buildSSHConfig(req)
	if err != nil {
		return nil, fmt.Errorf("ssh config: %w", err)
	}

	conn, err := ssh.Dial("tcp", host, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", host, err)
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	for k, v := range req.Env {
		session.Setenv(k, v)
	}

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(req.Command)
	}()

	select {
	case err := <-done:
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				return nil, fmt.Errorf("ssh run: %w", err)
			}
		}
		return &Result{
			ExitCode: exitCode,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
		}, nil

	case <-ctx.Done():
		session.Signal(ssh.SIGTERM)
		return nil, ctx.Err()
	}
}

func (r *SSHRunner) buildSSHConfig(req *Request) (*ssh.ClientConfig, error) {
	user := "root"
	if u, ok := req.Config["user"]; ok {
		if s, ok := u.(string); ok {
			user = s
		}
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	if key, ok := req.Secrets["ssh_private_key"]; ok {
		signer, err := ssh.ParsePrivateKey([]byte(key))
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		cfg.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else if pass, ok := req.Secrets["ssh_password"]; ok {
		cfg.Auth = []ssh.AuthMethod{ssh.Password(pass)}
	} else {
		return nil, fmt.Errorf("ssh runner requires either 'ssh_private_key' or 'ssh_password' secret")
	}

	return cfg, nil
}

func hasPort(host string) bool {
	_, _, err := net.SplitHostPort(host)
	return err == nil
}
