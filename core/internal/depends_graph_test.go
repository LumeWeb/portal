package internal_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/core/internal"
)

func TestAddNode(t *testing.T) {
	graph := internal.NewDependsGraph()

	graph.AddNode("A")
	assert.Contains(t, graph, "A")
	assert.NotNil(t, graph["A"])
	assert.Empty(t, graph["A"].Dependencies)
	assert.Equal(t, "A", graph["A"].ID)

	graph.AddNode("B", "A")
	assert.Contains(t, graph, "B")
	assert.NotNil(t, graph["B"])
	assert.Len(t, graph["B"].Dependencies, 1)
	assert.Equal(t, "A", graph["B"].Dependencies[0])
	assert.Equal(t, "B", graph["B"].ID)

	graph.AddNode("C", "A", "B")
	assert.Contains(t, graph, "C")
	assert.NotNil(t, graph["C"])
	assert.Len(t, graph["C"].Dependencies, 2)
	assert.Contains(t, graph["C"].Dependencies, "A")
	assert.Contains(t, graph["C"].Dependencies, "B")
	assert.Equal(t, "C", graph["C"].ID)
}

func TestBuild_SimpleOrdering(t *testing.T) {
	graph := internal.NewDependsGraph()
	graph.AddNode("A")
	graph.AddNode("B", "A")
	graph.AddNode("C", "B")

	order, err := graph.Build()
	assert.NoError(t, err)
	assert.Len(t, order, 3)

	// Check topological order: A must come before B, B before C
	aIndex := -1
	bIndex := -1
	cIndex := -1
	for i, id := range order {
		if id == "A" {
			aIndex = i
		} else if id == "B" {
			bIndex = i
		} else if id == "C" {
			cIndex = i
		}
	}

	assert.True(t, aIndex != -1 && bIndex != -1 && cIndex != -1, "All nodes should be in the order")
	assert.True(t, aIndex < bIndex, "A should come before B")
	assert.True(t, bIndex < cIndex, "B should come before C")
}

func TestBuild_ComplexOrdering(t *testing.T) {
	graph := internal.NewDependsGraph()
	graph.AddNode("A")
	graph.AddNode("B", "A")
	graph.AddNode("C", "A")
	graph.AddNode("D", "B", "C")
	graph.AddNode("E", "D")

	order, err := graph.Build()
	assert.NoError(t, err)
	assert.Len(t, order, 5)

	// Check topological order
	aIndex := -1
	bIndex := -1
	cIndex := -1
	dIndex := -1
	eIndex := -1
	for i, id := range order {
		if id == "A" {
			aIndex = i
		} else if id == "B" {
			bIndex = i
		} else if id == "C" {
			cIndex = i
		} else if id == "D" {
			dIndex = i
		} else if id == "E" {
			eIndex = i
		}
	}

	assert.True(t, aIndex != -1 && bIndex != -1 && cIndex != -1 && dIndex != -1 && eIndex != -1, "All nodes should be in the order")
	assert.True(t, aIndex < bIndex, "A should come before B")
	assert.True(t, aIndex < cIndex, "A should come before C")
	assert.True(t, bIndex < dIndex, "B should come before D")
	assert.True(t, cIndex < dIndex, "C should come before D")
	assert.True(t, dIndex < eIndex, "D should come before E")
}

func TestBuild_NoDependencies(t *testing.T) {
	graph := internal.NewDependsGraph()
	graph.AddNode("A")
	graph.AddNode("B")
	graph.AddNode("C")

	order, err := graph.Build()
	assert.NoError(t, err)
	assert.Len(t, order, 3)

	// Order can be arbitrary, just check all nodes are present
	assert.Contains(t, order, "A")
	assert.Contains(t, order, "B")
	assert.Contains(t, order, "C")
}

func TestBuild_CycleDetection(t *testing.T) {
	graph := internal.NewDependsGraph()
	graph.AddNode("A", "C") // A depends on C
	graph.AddNode("B", "A") // B depends on A
	graph.AddNode("C", "B") // C depends on B (cycle: A -> B -> C -> A)

	order, err := graph.Build()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle detected")
	assert.Empty(t, order)

	// Test self-cycle
	graph = internal.NewDependsGraph()
	graph.AddNode("A", "A")
	order, err = graph.Build()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle detected")
	assert.Empty(t, order)
}

func TestBuild_MissingDependency(t *testing.T) {
	graph := internal.NewDependsGraph()
	graph.AddNode("A", "NonExistent") // A depends on a non-existent node
	graph.AddNode("B", "A")

	order, err := graph.Build()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dependency not found: NonExistent")
	assert.Empty(t, order)
}
