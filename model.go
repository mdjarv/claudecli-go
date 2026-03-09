package claudecli

// Model represents a Claude model identifier.
type Model string

const (
	ModelHaiku  Model = "haiku"
	ModelSonnet Model = "sonnet"
	ModelOpus   Model = "opus"
)

// DefaultModel is used when no model is specified.
var DefaultModel = ModelSonnet
