package claudecli

// EffortLevel controls reasoning intensity. On Opus 4.7+ this drives adaptive
// thinking (the model decides when and how much to think per step). On earlier
// models it maps to extended thinking with a fixed budget.
type EffortLevel string

const (
	EffortLow    EffortLevel = "low"
	EffortMedium EffortLevel = "medium"
	EffortHigh   EffortLevel = "high"
	EffortXHigh  EffortLevel = "xhigh"
	EffortMax    EffortLevel = "max"

	// DefaultEffort is the Claude Code default since Opus 4.7.
	DefaultEffort = EffortXHigh
)

// Model represents a Claude model identifier.
type Model string

const (
	ModelHaiku  Model = "haiku"
	ModelSonnet Model = "sonnet"
	ModelOpus   Model = "opus"

	// DefaultModel is used when no model is specified.
	DefaultModel = ModelSonnet
)
