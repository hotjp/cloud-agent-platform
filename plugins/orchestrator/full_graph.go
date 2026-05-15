// Package orchestrator implements Eino-based task orchestration graph.
// Responsible for task decomposition, agent matching, and execution scheduling.
package orchestrator

import (
	"context"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// FullTaskGraph represents the complete task orchestration graph.
type FullTaskGraph struct {
	graph *compose.Graph[*TaskInput, *TaskResult]
	deps  *Dependencies
}

// NewFullTaskGraph creates a new FullTaskGraph with the given dependencies.
func NewFullTaskGraph(deps *Dependencies) (*FullTaskGraph, error) {
	g := compose.NewGraph[*TaskInput, *TaskResult]()

	// Create router condition that examines complexity and returns node name
	routerCondition := func(ctx context.Context, in *TaskContext) (string, error) {
		switch in.Complexity {
		case ComplexitySimple:
			return "simple_executor", nil
		case ComplexityMedium:
			return "medium_path", nil
		case ComplexityComplex:
			return "complex_path", nil
		default:
			return "simple_executor", nil
		}
	}

	// Create the router branch
	routerBranch := compose.NewGraphBranch(routerCondition, map[string]bool{
		"simple_executor": true,
		"medium_path":     true,
		"complex_path":    true,
	})

	// Create chains for each path
	simpleChain, err := buildSimplePath(deps)
	if err != nil {
		return nil, err
	}

	mediumChain, err := buildMediumPath(deps)
	if err != nil {
		return nil, err
	}

	complexChain, err := buildComplexPath(deps)
	if err != nil {
		return nil, err
	}

	// Add chains as graph nodes
	if err := g.AddGraphNode("simple_executor", simpleChain); err != nil {
		return nil, err
	}
	if err := g.AddGraphNode("medium_path", mediumChain); err != nil {
		return nil, err
	}
	if err := g.AddGraphNode("complex_path", complexChain); err != nil {
		return nil, err
	}

	// Add the router branch from analyzer
	// Note: analyzer will be added first as a lambda node
	analyzerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskInput) (*TaskContext, error) {
		node := NewAnalyzerNode(deps)
		return node.Invoke(ctx, in)
	})
	if err := g.AddLambdaNode("analyzer", analyzerLambda); err != nil {
		return nil, err
	}

	// Set entry point: START -> analyzer
	if err := g.AddEdge("start", "analyzer"); err != nil {
		return nil, err
	}

	// Add branch from analyzer using router condition
	if err := g.AddBranch("analyzer", routerBranch); err != nil {
		return nil, err
	}

	// Mark paths as end nodes by connecting to END
	if err := g.AddEdge("simple_executor", "end"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("medium_path", "end"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("complex_path", "end"); err != nil {
		return nil, err
	}

	return &FullTaskGraph{
		graph: g,
		deps:  deps,
	}, nil
}

// buildSimplePath builds the simple execution path: simple_executor -> merger
func buildSimplePath(deps *Dependencies) (*compose.Chain[*TaskContext, *TaskResult], error) {
	c := compose.NewChain[*TaskContext, *TaskResult]()

	simpleLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewSimpleExecutorNode(deps)
		return node.Invoke(ctx, in)
	})

	mergerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskResult, error) {
		node := NewResultMerger(deps)
		return node.Invoke(ctx, in)
	})

	c.AppendLambda(simpleLambda)
	c.AppendLambda(mergerLambda)

	return c, nil
}

// buildMediumPath builds the medium execution path: medium_decomposer -> medium_executor -> merger
func buildMediumPath(deps *Dependencies) (*compose.Chain[*TaskContext, *TaskResult], error) {
	c := compose.NewChain[*TaskContext, *TaskResult]()

	decomposerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewMediumDecomposerNode(deps)
		return node.Invoke(ctx, in)
	})

	executorLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewMediumExecutorNode(deps)
		return node.Invoke(ctx, in)
	})

	mergerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskResult, error) {
		node := NewResultMerger(deps)
		return node.Invoke(ctx, in)
	})

	c.AppendLambda(decomposerLambda)
	c.AppendLambda(executorLambda)
	c.AppendLambda(mergerLambda)

	return c, nil
}

// buildComplexPath builds the complex execution path:
// observer -> strategist -> executor -> guardian -> tester -> merger
func buildComplexPath(deps *Dependencies) (*compose.Chain[*TaskContext, *TaskResult], error) {
	c := compose.NewChain[*TaskContext, *TaskResult]()

	observerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewObserverNode(deps)
		return node.Invoke(ctx, in)
	})

	strategistLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewStrategistNode(deps)
		return node.Invoke(ctx, in)
	})

	executorLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewExecutorNode(deps)
		return node.Invoke(ctx, in)
	})

	guardianLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewGuardianNode(deps)
		return node.Invoke(ctx, in)
	})

	testerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewTesterNode(deps)
		return node.Invoke(ctx, in)
	})

	mergerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskResult, error) {
		node := NewResultMerger(deps)
		return node.Invoke(ctx, in)
	})

	c.AppendLambda(observerLambda)
	c.AppendLambda(strategistLambda)
	c.AppendLambda(executorLambda)
	c.AppendLambda(guardianLambda)
	c.AppendLambda(testerLambda)
	c.AppendLambda(mergerLambda)

	return c, nil
}

// Compile compiles the graph and returns a runnable graph.
func (ftg *FullTaskGraph) Compile(ctx context.Context) (compose.Runnable[*TaskInput, *TaskResult], error) {
	return ftg.graph.Compile(ctx)
}

// Invoke runs the graph with the given input.
func (ftg *FullTaskGraph) Invoke(ctx context.Context, input *TaskInput) (*TaskResult, error) {
	rg, err := ftg.Compile(ctx)
	if err != nil {
		return nil, err
	}

	// Invoke the graph with the input
	output, err := rg.Invoke(ctx, input)
	if err != nil {
		return nil, err
	}

	return output, nil
}

// BuildFullTaskGraph is a convenience function to build and compile the graph.
func BuildFullTaskGraph(ctx context.Context, deps *Dependencies) (compose.Runnable[*TaskInput, *TaskResult], error) {
	ftg, err := NewFullTaskGraph(deps)
	if err != nil {
		return nil, err
	}
	return ftg.Compile(ctx)
}

// ParallelExecutorNode creates a node that executes multiple nodes in parallel.
type ParallelExecutorNode struct {
	deps   *Dependencies
	name   string
	nodes  []func(ctx context.Context, in *TaskContext) (*TaskContext, error)
}

// NewParallelExecutorNode creates a new parallel executor node.
func NewParallelExecutorNode(deps *Dependencies, name string, nodes ...func(ctx context.Context, in *TaskContext) (*TaskContext, error)) *ParallelExecutorNode {
	return &ParallelExecutorNode{
		deps:  deps,
		name:  name,
		nodes: nodes,
	}
}

// Invoke executes the parallel nodes.
func (pe *ParallelExecutorNode) Invoke(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if len(pe.nodes) == 0 {
		return input, nil
	}

	type result struct {
		ctx *TaskContext
		err error
	}

	results := make(chan result, len(pe.nodes))

	for _, nodeFn := range pe.nodes {
		go func(fn func(ctx context.Context, in *TaskContext) (*TaskContext, error)) {
			out, err := fn(ctx, input)
			if err != nil {
				results <- result{err: err}
				return
			}
			results <- result{ctx: out}
		}(nodeFn)
	}

	var mergedCtx *TaskContext
	for i := 0; i < len(pe.nodes); i++ {
		r := <-results
		if r.err != nil {
			return input, r.err
		}
		if mergedCtx == nil {
			mergedCtx = r.ctx
		} else {
			// Merge summaries
			if mergedCtx.Summary == "" {
				mergedCtx.Summary = r.ctx.Summary
			} else if r.ctx.Summary != "" {
				mergedCtx.Summary = mergedCtx.Summary + "\n" + r.ctx.Summary
			}
			mergedCtx.TokensUsed += r.ctx.TokensUsed
		}
	}

	if mergedCtx != nil {
		return mergedCtx, nil
	}
	return input, nil
}

// StreamResult wraps a result for streaming.
type StreamResult struct {
	TaskResult *TaskResult
	Stream     *schema.StreamReader[*TaskResult]
}
