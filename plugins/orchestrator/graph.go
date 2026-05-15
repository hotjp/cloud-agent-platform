// Package orchestrator implements Eino-based task orchestration graph.
// Responsible for task decomposition, agent matching, and execution scheduling.
package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/compose"
)

// NodeRegistry maintains a registry of available graph nodes.
type NodeRegistry struct {
	mu    sync.RWMutex
	nodes map[string]NodeFactory
}

// NodeFactory creates a node instance with the given dependencies.
type NodeFactory func(deps *Dependencies) (interface{}, error)

// GraphConfig holds configuration for the TaskGraph.
type GraphConfig struct {
	// Analyzer is the complexity analyzer to use.
	Analyzer *ComplexityAnalyzer
	// NodeRegistry is the registry of available nodes.
	NodeRegistry *NodeRegistry
	// EnableTracing enables OpenTelemetry tracing.
	EnableTracing bool
	// MaxConcurrentSubtasks is the max parallel subtasks for medium path.
	MaxConcurrentSubtasks int
}

// DefaultGraphConfig returns the default graph configuration.
func DefaultGraphConfig() *GraphConfig {
	return &GraphConfig{
		Analyzer:              NewComplexityAnalyzer(),
		NodeRegistry:          NewNodeRegistry(),
		EnableTracing:        true,
		MaxConcurrentSubtasks: 3,
	}
}

// NewNodeRegistry creates a new empty node registry.
func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes: make(map[string]NodeFactory),
	}
}

// RegisterNode registers a node factory under a name.
func (nr *NodeRegistry) RegisterNode(name string, factory NodeFactory) {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	nr.nodes[name] = factory
}

// GetNode retrieves a node factory by name.
func (nr *NodeRegistry) GetNode(name string) (NodeFactory, bool) {
	nr.mu.RLock()
	defer nr.mu.RUnlock()
	factory, ok := nr.nodes[name]
	return factory, ok
}

// ListNodes returns all registered node names.
func (nr *NodeRegistry) ListNodes() []string {
	nr.mu.RLock()
	defer nr.mu.RUnlock()
	names := make([]string, 0, len(nr.nodes))
	for name := range nr.nodes {
		names = append(names, name)
	}
	return names
}

// TaskGraph is a wrapper around eino's compose.Graph with orchestration-specific features.
type TaskGraph struct {
	graph *compose.Graph[*TaskInput, *TaskResult]
	deps  *Dependencies
	cfg   *GraphConfig
	mu    sync.RWMutex
}

// NewTaskGraph creates a new TaskGraph with the given configuration.
func NewTaskGraph(deps *Dependencies, cfg *GraphConfig) (*TaskGraph, error) {
	if deps == nil {
		return nil, fmt.Errorf("dependencies cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultGraphConfig()
	}
	if cfg.Analyzer == nil {
		cfg.Analyzer = NewComplexityAnalyzer()
	}
	if cfg.NodeRegistry == nil {
		cfg.NodeRegistry = NewNodeRegistry()
	}

	// Register default nodes
	registerDefaultNodes(cfg.NodeRegistry)

	g := compose.NewGraph[*TaskInput, *TaskResult]()

	tg := &TaskGraph{
		graph: g,
		deps:  deps,
		cfg:   cfg,
	}

	// Build the graph structure
	if err := tg.buildGraph(); err != nil {
		return nil, err
	}

	return tg, nil
}

// registerDefaultNodes registers the default orchestration nodes.
func registerDefaultNodes(registry *NodeRegistry) {
	registry.RegisterNode("analyzer", func(deps *Dependencies) (interface{}, error) {
		return NewAnalyzerNode(deps), nil
	})
	registry.RegisterNode("simple_executor", func(deps *Dependencies) (interface{}, error) {
		return NewSimpleExecutorNode(deps), nil
	})
	registry.RegisterNode("medium_decomposer", func(deps *Dependencies) (interface{}, error) {
		return NewMediumDecomposerNode(deps), nil
	})
	registry.RegisterNode("medium_executor", func(deps *Dependencies) (interface{}, error) {
		return NewMediumExecutorNode(deps), nil
	})
	registry.RegisterNode("observer", func(deps *Dependencies) (interface{}, error) {
		return NewObserverNode(deps), nil
	})
	registry.RegisterNode("strategist", func(deps *Dependencies) (interface{}, error) {
		return NewStrategistNode(deps), nil
	})
	registry.RegisterNode("executor", func(deps *Dependencies) (interface{}, error) {
		return NewExecutorNode(deps), nil
	})
	registry.RegisterNode("guardian", func(deps *Dependencies) (interface{}, error) {
		return NewGuardianNode(deps), nil
	})
	registry.RegisterNode("tester", func(deps *Dependencies) (interface{}, error) {
		return NewTesterNode(deps), nil
	})
	registry.RegisterNode("result_merger", func(deps *Dependencies) (interface{}, error) {
		return NewResultMerger(deps), nil
	})
}

// buildGraph constructs the orchestration graph with complexity-based routing.
// Graph structure follows the Eino pattern used in full_graph.go:
//   START -> analyzer -> [router branch] -> {simple_path | medium_path | complex_path} -> END
func (tg *TaskGraph) buildGraph() error {
	// Create router condition that examines complexity and returns node name
	analyzer := tg.cfg.Analyzer
	routerCondition := func(ctx context.Context, in *TaskContext) (string, error) {
		// Use the ComplexityAnalyzer to determine routing
		result := analyzer.Analyze(ctx, &TaskInput{
			TaskID:               in.TaskID,
			Goal:                 in.Goal,
			Constraints:          in.Constraints,
			RepoURL:              in.RepoURL,
			BaseBranch:           in.BaseBranch,
			HistoricalComplexity: "", // Could be populated from task history
		})
		return result, nil
	}

	// Create the router branch
	routerBranch := compose.NewGraphBranch(routerCondition, map[string]bool{
		"simple":  true,
		"medium":  true,
		"complex": true,
	})

	// Build each execution path
	simpleChain, err := tg.buildSimplePath()
	if err != nil {
		return fmt.Errorf("failed to build simple path: %w", err)
	}

	mediumChain, err := tg.buildMediumPath()
	if err != nil {
		return fmt.Errorf("failed to build medium path: %w", err)
	}

	complexChain, err := tg.buildComplexPath()
	if err != nil {
		return fmt.Errorf("failed to build complex path: %w", err)
	}

	// Add chains as graph nodes
	if err := tg.graph.AddGraphNode("simple", simpleChain); err != nil {
		return fmt.Errorf("failed to add simple path: %w", err)
	}
	if err := tg.graph.AddGraphNode("medium", mediumChain); err != nil {
		return fmt.Errorf("failed to add medium path: %w", err)
	}
	if err := tg.graph.AddGraphNode("complex", complexChain); err != nil {
		return fmt.Errorf("failed to add complex path: %w", err)
	}

	// Add analyzer lambda node
	analyzerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskInput) (*TaskContext, error) {
		node := NewAnalyzerNode(tg.deps)
		return node.Invoke(ctx, in)
	})
	if err := tg.graph.AddLambdaNode("analyzer", analyzerLambda); err != nil {
		return fmt.Errorf("failed to add analyzer node: %w", err)
	}

	// Set entry point: START -> analyzer
	if err := tg.graph.AddEdge("start", "analyzer"); err != nil {
		return fmt.Errorf("failed to add start->analyzer edge: %w", err)
	}

	// Add branch from analyzer using router condition
	if err := tg.graph.AddBranch("analyzer", routerBranch); err != nil {
		return fmt.Errorf("failed to add router branch: %w", err)
	}

	// Mark paths as end nodes by connecting to END
	if err := tg.graph.AddEdge("simple", "end"); err != nil {
		return fmt.Errorf("failed to add simple->end edge: %w", err)
	}
	if err := tg.graph.AddEdge("medium", "end"); err != nil {
		return fmt.Errorf("failed to add medium->end edge: %w", err)
	}
	if err := tg.graph.AddEdge("complex", "end"); err != nil {
		return fmt.Errorf("failed to add complex->end edge: %w", err)
	}

	return nil
}

// buildSimplePath builds the simple execution path: simple_executor -> result_merger
func (tg *TaskGraph) buildSimplePath() (*compose.Chain[*TaskContext, *TaskResult], error) {
	c := compose.NewChain[*TaskContext, *TaskResult]()

	simpleLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewSimpleExecutorNode(tg.deps)
		return node.Invoke(ctx, in)
	})

	mergerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskResult, error) {
		node := NewResultMerger(tg.deps)
		return node.Invoke(ctx, in)
	})

	c.AppendLambda(simpleLambda)
	c.AppendLambda(mergerLambda)

	return c, nil
}

// buildMediumPath builds the medium execution path: medium_decomposer -> medium_executor -> result_merger
func (tg *TaskGraph) buildMediumPath() (*compose.Chain[*TaskContext, *TaskResult], error) {
	c := compose.NewChain[*TaskContext, *TaskResult]()

	decomposerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewMediumDecomposerNode(tg.deps)
		return node.Invoke(ctx, in)
	})

	executorLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewMediumExecutorNode(tg.deps)
		return node.Invoke(ctx, in)
	})

	mergerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskResult, error) {
		node := NewResultMerger(tg.deps)
		return node.Invoke(ctx, in)
	})

	c.AppendLambda(decomposerLambda)
	c.AppendLambda(executorLambda)
	c.AppendLambda(mergerLambda)

	return c, nil
}

// buildComplexPath builds the complex execution path:
// observer -> strategist -> executor -> guardian -> tester -> result_merger
func (tg *TaskGraph) buildComplexPath() (*compose.Chain[*TaskContext, *TaskResult], error) {
	c := compose.NewChain[*TaskContext, *TaskResult]()

	observerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewObserverNode(tg.deps)
		return node.Invoke(ctx, in)
	})

	strategistLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewStrategistNode(tg.deps)
		return node.Invoke(ctx, in)
	})

	executorLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewExecutorNode(tg.deps)
		return node.Invoke(ctx, in)
	})

	guardianLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewGuardianNode(tg.deps)
		return node.Invoke(ctx, in)
	})

	testerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
		node := NewTesterNode(tg.deps)
		return node.Invoke(ctx, in)
	})

	mergerLambda := compose.InvokableLambda(func(ctx context.Context, in *TaskContext) (*TaskResult, error) {
		node := NewResultMerger(tg.deps)
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
func (tg *TaskGraph) Compile(ctx context.Context) (compose.Runnable[*TaskInput, *TaskResult], error) {
	tg.mu.RLock()
	defer tg.mu.RUnlock()
	return tg.graph.Compile(ctx)
}

// Invoke runs the graph with the given input.
func (tg *TaskGraph) Invoke(ctx context.Context, input *TaskInput) (*TaskResult, error) {
	tg.mu.RLock()
	rg, err := tg.graph.Compile(ctx)
	tg.mu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("failed to compile graph: %w", err)
	}

	result, err := rg.Invoke(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("graph invocation failed: %w", err)
	}

	return result, nil
}

// GetConfig returns the graph configuration.
func (tg *TaskGraph) GetConfig() *GraphConfig {
	tg.mu.RLock()
	defer tg.mu.RUnlock()
	return tg.cfg
}

// GetAnalyzer returns the complexity analyzer.
func (tg *TaskGraph) GetAnalyzer() *ComplexityAnalyzer {
	tg.mu.RLock()
	defer tg.mu.RUnlock()
	return tg.cfg.Analyzer
}

// RegisterNode dynamically registers a new node type.
func (tg *TaskGraph) RegisterNode(name string, factory NodeFactory) error {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	tg.cfg.NodeRegistry.RegisterNode(name, factory)
	return nil
}
