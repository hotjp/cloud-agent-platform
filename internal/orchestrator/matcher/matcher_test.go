package matcher

import (
	"context"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentProfile_SuccessRate(t *testing.T) {
	tests := []struct {
		name     string
		profile  AgentProfile
		expected float64
	}{
		{
			name: "all success",
			profile: AgentProfile{
				HistoricalSuccessCount:  10,
				HistoricalFailureCount: 0,
			},
			expected: 1.0,
		},
		{
			name: "all failure",
			profile: AgentProfile{
				HistoricalSuccessCount:  0,
				HistoricalFailureCount: 10,
			},
			expected: 0.0,
		},
		{
			name: "half success",
			profile: AgentProfile{
				HistoricalSuccessCount:  5,
				HistoricalFailureCount: 5,
			},
			expected: 0.5,
		},
		{
			name: "cold start - no history",
			profile: AgentProfile{
				HistoricalSuccessCount:  0,
				HistoricalFailureCount: 0,
			},
			expected: DefaultSuccessRate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.profile.SuccessRate()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatcher_Score(t *testing.T) {
	m := New()

	tests := []struct {
		name           string
		requirements   TaskRequirements
		profile        AgentProfile
		expectNil      bool
		expectedScore  float64
	}{
		{
			name: "exact capability match",
			requirements: TaskRequirements{
				RequiredCapabilities: []string{"coding", "testing"},
				TaskType:            domain.SubtaskTypeCoding,
			},
			profile: AgentProfile{
				AgentTemplateID:      "executor",
				Role:                 domain.AgentRoleExecutor,
				CapabilityTags:       []string{"coding", "testing", "review"},
				HistoricalSuccessCount: 8,
				HistoricalFailureCount: 2,
				CostPerExecution:     0.10,
			},
			expectNil:     false,
			expectedScore: 0.94, // capability: 1.0, success: 0.8, cost: high
		},
		{
			name: "partial capability match",
			requirements: TaskRequirements{
				RequiredCapabilities: []string{"coding", "testing", "security"},
				TaskType:            domain.SubtaskTypeCoding,
			},
			profile: AgentProfile{
				AgentTemplateID:      "executor",
				Role:                 domain.AgentRoleExecutor,
				CapabilityTags:       []string{"coding", "review"},
				HistoricalSuccessCount: 5,
				HistoricalFailureCount: 5,
				CostPerExecution:     0.10,
			},
			expectNil: false,
			// capability: 1/3 = 0.333, success: 0.5, cost: high
			// 0.333*0.5 + 0.5*0.3 + high*0.2
			expectedScore: 0.46,
		},
		{
			name: "incompatible role",
			requirements: TaskRequirements{
				RequiredCapabilities: []string{"analysis"},
				TaskType:            domain.SubtaskTypeTesting,
			},
			profile: AgentProfile{
				AgentTemplateID: "executor",
				Role:            domain.AgentRoleExecutor, // Tester needed for testing
				CapabilityTags:  []string{"analysis", "coding"},
			},
			expectNil: true,
		},
		{
			name: "cold start agent",
			requirements: TaskRequirements{
				RequiredCapabilities: []string{"coding"},
				TaskType:            domain.SubtaskTypeCoding,
			},
			profile: AgentProfile{
				AgentTemplateID:      "executor",
				Role:                 domain.AgentRoleExecutor,
				CapabilityTags:       []string{"coding", "testing"},
				HistoricalSuccessCount: 0, // Cold start
				HistoricalFailureCount: 0,
				CostPerExecution:     0.10,
			},
			expectNil: false,
			// capability: 1.0, success: 0.5 (default), cost: 0.75
			// 1.0*0.5 + 0.5*0.3 + 0.75*0.2 = 0.8
			expectedScore: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result := m.Score(ctx, &tt.requirements, &tt.profile)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.InDelta(t, tt.expectedScore, result.TotalScore, 0.1)
				assert.Equal(t, tt.profile.AgentTemplateID, result.Agent.AgentTemplateID)
			}
		})
	}
}

func TestMatcher_ScoreAll(t *testing.T) {
	m := New()
	ctx := context.Background()

	requirements := &TaskRequirements{
		RequiredCapabilities: []string{"coding"},
		TaskType:            domain.SubtaskTypeCoding,
		Budget:              0.15,
	}

	profiles := []*AgentProfile{
		{
			AgentTemplateID:      "executor1",
			Role:                 domain.AgentRoleExecutor,
			CapabilityTags:       []string{"coding"},
			HistoricalSuccessCount: 9,
			HistoricalFailureCount: 1,
			CostPerExecution:     0.08,
		},
		{
			AgentTemplateID:      "executor2",
			Role:                 domain.AgentRoleExecutor,
			CapabilityTags:       []string{"coding", "testing"},
			HistoricalSuccessCount: 5,
			HistoricalFailureCount: 5,
			CostPerExecution:     0.12,
		},
		{
			AgentTemplateID: "observer",
			Role:            domain.AgentRoleObserver, // Incompatible for coding
			CapabilityTags:  []string{"analysis"},
		},
	}

	results := m.ScoreAll(ctx, requirements, profiles)

	// Should only return 2 compatible agents
	assert.Len(t, results, 2)

	// Should be sorted by score descending
	if len(results) == 2 {
		assert.GreaterOrEqual(t, results[0].TotalScore, results[1].TotalScore)
	}
}

func TestMatcher_FindBestAgent(t *testing.T) {
	m := New()
	ctx := context.Background()

	t.Run("returns best agent", func(t *testing.T) {
		requirements := &TaskRequirements{
			TaskType: domain.SubtaskTypeCoding,
			Budget:    0.20,
		}

		profiles := []*AgentProfile{
			{
				AgentTemplateID:      "executor1",
				Role:                 domain.AgentRoleExecutor,
				CapabilityTags:       []string{"coding"},
				HistoricalSuccessCount: 5,
				HistoricalFailureCount: 5,
				CostPerExecution:     0.15,
			},
			{
				AgentTemplateID:      "executor2",
				Role:                 domain.AgentRoleExecutor,
				CapabilityTags:       []string{"coding", "testing"},
				HistoricalSuccessCount: 9,
				HistoricalFailureCount: 1,
				CostPerExecution:     0.10,
			},
		}

		best := m.FindBestAgent(ctx, requirements, profiles)
		require.NotNil(t, best)
		assert.Equal(t, "executor2", best.AgentTemplateID)
	})

	t.Run("returns nil for empty profiles", func(t *testing.T) {
		requirements := &TaskRequirements{
			TaskType: domain.SubtaskTypeCoding,
		}

		best := m.FindBestAgent(ctx, requirements, nil)
		assert.Nil(t, best)

		best = m.FindBestAgent(ctx, requirements, []*AgentProfile{})
		assert.Nil(t, best)
	})
}

func TestMatcher_FindTopKAgents(t *testing.T) {
	m := New()
	ctx := context.Background()

	profiles := []*AgentProfile{
		{AgentTemplateID: "agent1", Role: domain.AgentRoleExecutor, CapabilityTags: []string{"coding"}},
		{AgentTemplateID: "agent2", Role: domain.AgentRoleExecutor, CapabilityTags: []string{"coding", "testing"}},
		{AgentTemplateID: "agent3", Role: domain.AgentRoleExecutor, CapabilityTags: []string{"coding"}},
		{AgentTemplateID: "agent4", Role: domain.AgentRoleExecutor, CapabilityTags: []string{"coding", "review"}},
	}

	t.Run("returns top K", func(t *testing.T) {
		requirements := &TaskRequirements{TaskType: domain.SubtaskTypeCoding}

		top2 := m.FindTopKAgents(ctx, requirements, profiles, 2)
		require.NotNil(t, top2)
		assert.Len(t, top2, 2)
	})

	t.Run("K larger than available", func(t *testing.T) {
		requirements := &TaskRequirements{TaskType: domain.SubtaskTypeCoding}

		top10 := m.FindTopKAgents(ctx, requirements, profiles, 10)
		require.NotNil(t, top10)
		assert.Len(t, top10, 4) // Only 4 available
	})

	t.Run("K <= 0 returns nil", func(t *testing.T) {
		requirements := &TaskRequirements{TaskType: domain.SubtaskTypeCoding}

		top0 := m.FindTopKAgents(ctx, requirements, profiles, 0)
		assert.Nil(t, top0)

		topNeg := m.FindTopKAgents(ctx, requirements, profiles, -1)
		assert.Nil(t, topNeg)
	})

	t.Run("empty profiles returns nil", func(t *testing.T) {
		requirements := &TaskRequirements{TaskType: domain.SubtaskTypeCoding}

		result := m.FindTopKAgents(ctx, requirements, []*AgentProfile{}, 3)
		assert.Nil(t, result)
	})
}

func TestMatcher_ScoringFormula(t *testing.T) {
	m := New()
	ctx := context.Background()

	// Test that the scoring formula follows: capability×0.5 + success×0.3 + cost×0.2
	t.Run("high capability dominates with high success rate", func(t *testing.T) {
		requirements := &TaskRequirements{
			RequiredCapabilities: []string{"coding", "testing"},
			TaskType:            domain.SubtaskTypeCoding,
			Budget:              0.20,
		}

		// Perfect capability match, 100% success rate, within budget
		profile := &AgentProfile{
			AgentTemplateID:      "executor",
			Role:                 domain.AgentRoleExecutor,
			CapabilityTags:       []string{"coding", "testing"},
			HistoricalSuccessCount: 10,
			HistoricalFailureCount: 0,
			CostPerExecution:     0.10,
		}

		result := m.Score(ctx, requirements, profile)
		require.NotNil(t, result)

		// Verify the formula: capability*0.5 + success*0.3 + cost*0.2
		expected := result.CapabilityScore*WeightCapability +
			result.SuccessRateScore*WeightSuccessRate +
			result.CostEfficiencyScore*WeightCostEfficiency

		assert.InDelta(t, expected, result.TotalScore, 0.001)
		assert.InDelta(t, 1.0, result.CapabilityScore, 0.001)
		assert.InDelta(t, 1.0, result.SuccessRateScore, 0.001)
	})

	t.Run("cold start uses default success rate", func(t *testing.T) {
		requirements := &TaskRequirements{
			TaskType: domain.SubtaskTypeCoding,
		}

		profile := &AgentProfile{
			AgentTemplateID:      "executor",
			Role:                 domain.AgentRoleExecutor,
			CapabilityTags:       []string{"coding"},
			HistoricalSuccessCount: 0,
			HistoricalFailureCount: 0,
			CostPerExecution:     0.10,
		}

		result := m.Score(ctx, requirements, profile)
		require.NotNil(t, result)
		assert.InDelta(t, DefaultSuccessRate, result.SuccessRateScore, 0.001)
	})
}

func TestMatcher_RoleCompatibility(t *testing.T) {
	m := New()

	tests := []struct {
		role     domain.AgentRole
		taskType domain.SubtaskType
		expected bool
	}{
		{domain.AgentRoleExecutor, domain.SubtaskTypeCoding, true},
		{domain.AgentRoleTester, domain.SubtaskTypeTesting, true},
		{domain.AgentRoleGuardian, domain.SubtaskTypeReview, true},
		{domain.AgentRoleStrategist, domain.SubtaskTypeAnalysis, true},
		{domain.AgentRoleResearcher, domain.SubtaskTypeResearch, true},
		{domain.AgentRoleObserver, domain.SubtaskTypeAnalysis, true},
		{domain.AgentRoleExecutor, domain.SubtaskTypeTesting, false},  // Wrong role
		{domain.AgentRoleTester, domain.SubtaskTypeCoding, false},    // Wrong role
	}

	for _, tt := range tests {
		t.Run(string(tt.role)+"_vs_"+string(tt.taskType), func(t *testing.T) {
			result := m.isRoleCompatible(tt.role, tt.taskType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatcher_SameScoreSorting(t *testing.T) {
	m := New()
	ctx := context.Background()

	requirements := &TaskRequirements{
		TaskType: domain.SubtaskTypeCoding,
		Budget:    0.20,
	}

	// Two agents with identical profiles
	profiles := []*AgentProfile{
		{
			AgentTemplateID:      "executor1",
			Role:                 domain.AgentRoleExecutor,
			CapabilityTags:       []string{"coding"},
			HistoricalSuccessCount: 5,
			HistoricalFailureCount: 5,
			CostPerExecution:     0.10,
		},
		{
			AgentTemplateID:      "executor2",
			Role:                 domain.AgentRoleExecutor,
			CapabilityTags:       []string{"coding"},
			HistoricalSuccessCount: 5,
			HistoricalFailureCount: 5,
			CostPerExecution:     0.10,
		},
	}

	results := m.ScoreAll(ctx, requirements, profiles)

	// Both should have same score
	require.Len(t, results, 2)
	assert.Equal(t, results[0].TotalScore, results[1].TotalScore)

	// FindBestAgent should return one of them (deterministic due to sort)
	best := m.FindBestAgent(ctx, requirements, profiles)
	require.NotNil(t, best)
	// Either is acceptable since scores are equal
	assert.True(t, best.AgentTemplateID == "executor1" || best.AgentTemplateID == "executor2")
}

func TestMatcher_CostEfficiency(t *testing.T) {
	m := New()

	tests := []struct {
		name           string
		cost           float64
		budget         float64
		expectedMin    float64
		expectedMax    float64
	}{
		{"at budget", 0.10, 0.10, 0.4, 0.6},
		{"below budget", 0.05, 0.10, 0.7, 1.0},
		{"above budget", 0.15, 0.10, 0.0, 0.3},
		{"no budget limit", 0.50, 0, DefaultCostEfficiency, DefaultCostEfficiency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &AgentProfile{CostPerExecution: tt.cost}
			reqs := &TaskRequirements{Budget: tt.budget}

			score := m.calculateCostEfficiency(profile, reqs)

			if tt.budget == 0 {
				assert.Equal(t, DefaultCostEfficiency, score)
			} else {
				assert.GreaterOrEqual(t, score, tt.expectedMin)
				assert.LessOrEqual(t, score, tt.expectedMax)
			}
		})
	}
}

func TestMatcher_CapabilityScore(t *testing.T) {
	m := New()

	tests := []struct {
		name          string
		capabilities  []string
		requirements  []string
		tags          []string
		expected      float64
	}{
		{"exact match", []string{"coding", "testing"}, []string{"coding", "testing"}, nil, 1.0},
		{"partial match", []string{"coding"}, []string{"coding", "testing"}, nil, 0.5},
		{"no match", []string{"review"}, []string{"coding", "testing"}, nil, 0.0},
		{"with tags", []string{"coding"}, []string{"coding"}, []string{"testing"}, 0.5},
		{"empty requirements", []string{"coding"}, nil, nil, 0.8}, // Default for matching role
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &AgentProfile{CapabilityTags: tt.capabilities}
			reqs := &TaskRequirements{
				RequiredCapabilities: tt.requirements,
				Tags:                tt.tags,
				TaskType:            domain.SubtaskTypeCoding,
			}

			score := m.calculateCapabilityScore(profile, reqs)
			assert.InDelta(t, tt.expected, score, 0.1)
		})
	}
}