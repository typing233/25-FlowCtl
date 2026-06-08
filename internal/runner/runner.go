package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type Request struct {
	ExecutionID uuid.UUID
	StepID      string
	Command     string
	Image       string
	Host        string
	Env         map[string]string
	Config      map[string]any
	Secrets     map[string]string
	WorkDir     string
}

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type Runner interface {
	Run(ctx context.Context, req *Request) (*Result, error)
	Name() string
}

type Factory struct {
	runners map[string]Runner
}

func NewFactory() *Factory {
	return &Factory{
		runners: make(map[string]Runner),
	}
}

func (f *Factory) Register(name string, r Runner) {
	f.runners[name] = r
}

func (f *Factory) Get(name string) (Runner, error) {
	r, ok := f.runners[name]
	if !ok {
		return nil, fmt.Errorf("runner %q not registered", name)
	}
	return r, nil
}

func DefaultFactory() *Factory {
	f := NewFactory()
	f.Register("local", NewLocalRunner())
	f.Register("docker", NewDockerRunner())
	f.Register("ssh", NewSSHRunner())
	return f
}
