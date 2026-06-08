package engine

import (
	"fmt"

	"github.com/flowctl/flowctl/internal/model"
)

type DAG struct {
	Nodes    map[string]*DAGNode
	Edges    map[string][]string // from -> [to]
	InEdges  map[string][]string // to -> [from]
}

type DAGNode struct {
	ID       string
	Type     string // "step" or "approval"
	StepDef  *model.StepDef
	Approval *model.ApprovalDef
}

func BuildDAG(def *model.WorkflowDefinition) (*DAG, error) {
	dag := &DAG{
		Nodes:   make(map[string]*DAGNode),
		Edges:   make(map[string][]string),
		InEdges: make(map[string][]string),
	}

	for i := range def.Steps {
		step := &def.Steps[i]
		dag.Nodes[step.ID] = &DAGNode{
			ID:      step.ID,
			Type:    "step",
			StepDef: step,
		}
	}

	for i := range def.Approvals {
		approval := &def.Approvals[i]
		dag.Nodes[approval.ID] = &DAGNode{
			ID:       approval.ID,
			Type:     "approval",
			Approval: approval,
		}
	}

	for _, step := range def.Steps {
		for _, dep := range step.DependsOn {
			if _, exists := dag.Nodes[dep]; !exists {
				return nil, fmt.Errorf("step %q depends on unknown node %q", step.ID, dep)
			}
			dag.Edges[dep] = append(dag.Edges[dep], step.ID)
			dag.InEdges[step.ID] = append(dag.InEdges[step.ID], dep)
		}
	}

	for _, approval := range def.Approvals {
		for _, dep := range approval.DependsOn {
			if _, exists := dag.Nodes[dep]; !exists {
				return nil, fmt.Errorf("approval %q depends on unknown node %q", approval.ID, dep)
			}
			dag.Edges[dep] = append(dag.Edges[dep], approval.ID)
			dag.InEdges[approval.ID] = append(dag.InEdges[approval.ID], dep)
		}
	}

	return dag, nil
}

func (d *DAG) TopologicalSort() ([][]string, error) {
	inDegree := make(map[string]int)
	for id := range d.Nodes {
		inDegree[id] = len(d.InEdges[id])
	}

	var layers [][]string
	visited := make(map[string]bool)

	for len(visited) < len(d.Nodes) {
		var layer []string
		for id, deg := range inDegree {
			if deg == 0 && !visited[id] {
				layer = append(layer, id)
			}
		}

		if len(layer) == 0 {
			var remaining []string
			for id := range d.Nodes {
				if !visited[id] {
					remaining = append(remaining, id)
				}
			}
			return nil, fmt.Errorf("cycle detected involving nodes: %v", remaining)
		}

		for _, id := range layer {
			visited[id] = true
			for _, downstream := range d.Edges[id] {
				inDegree[downstream]--
			}
		}

		layers = append(layers, layer)
	}

	return layers, nil
}

func (d *DAG) DetectCycles() error {
	_, err := d.TopologicalSort()
	return err
}

func (d *DAG) GetReadyNodes(completed map[string]bool) []string {
	var ready []string
	for id := range d.Nodes {
		if completed[id] {
			continue
		}
		allDepsComplete := true
		for _, dep := range d.InEdges[id] {
			if !completed[dep] {
				allDepsComplete = false
				break
			}
		}
		if allDepsComplete {
			ready = append(ready, id)
		}
	}
	return ready
}

func (d *DAG) GetDownstream(nodeID string) []string {
	return d.Edges[nodeID]
}

func (d *DAG) GetUpstream(nodeID string) []string {
	return d.InEdges[nodeID]
}
