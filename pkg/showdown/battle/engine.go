package battle

// battleengine provides a unified interface for selecting battle decisions.
type BattleEngine interface {
	Decide(b *Battle, req BattleRequest) BattleDecision
}

// defaultengine wraps the simultaneous minimax engine for standard battle play.
type DefaultEngine struct {
	minimax *MinimaxEngine
}

// newdefaultengine creates a default battle engine powered by minimax search.
func NewDefaultEngine() *DefaultEngine {
	return &DefaultEngine{
		minimax: NewMinimaxEngine(),
	}
}

// decide delegates decision-making to the minimax decision engine.
func (e *DefaultEngine) Decide(b *Battle, req BattleRequest) BattleDecision {
	if e.minimax != nil {
		return e.minimax.Decide(b, req)
	}
	return NewMinimaxEngine().Decide(b, req)
}
