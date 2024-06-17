package internal

import (
	"fmt"
)

type Node struct {
	ID           string
	Dependencies []string
}

type Graph map[string]*Node

func NewDependsGraph() Graph {
	return make(Graph)
}

func (g Graph) AddNode(id string, dependencies ...string) {
	node := &Node{
		ID:           id,
		Dependencies: dependencies,
	}
	g[id] = node
}

func (g Graph) Build() ([]string, error) {
	visited := make(map[string]bool)
	visitedStack := make(map[string]bool)
	result := make([]string, 0)

	for id := range g {
		if err := g.process(id, visited, visitedStack, &result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (g Graph) process(id string, visited, visitedStack map[string]bool, result *[]string) error {
	if visitedStack[id] {
		return fmt.Errorf("cycle detected for node: %s", id)
	}

	if !visited[id] {
		visitedStack[id] = true
		if _, ok := g[id]; !ok {
			return fmt.Errorf("dependency not found: %s", id)
		}
		node := g[id]
		for _, dep := range node.Dependencies {
			if err := g.process(dep, visited, visitedStack, result); err != nil {
				return err
			}
		}
		visited[id] = true
		visitedStack[id] = false
		*result = append(*result, id)
	}

	return nil
}
