package sfg_utils

import (
	"fmt"

	"gitlab.com/earthscope/gnsstools/core/gnss/observation"
)

// ContinuousEpochProcessor merges adjacent equal-time epochs and applies one
// lock-time tracker across source-file and output-batch boundaries. Inputs must
// arrive in chronological order.
type ContinuousEpochProcessor struct {
	tracker observation.LockTimeTracker
	pending *observation.Epoch
	emit    func(observation.Epoch) error
}

func NewContinuousEpochProcessor(emit func(observation.Epoch) error) *ContinuousEpochProcessor {
	return &ContinuousEpochProcessor{emit: emit}
}

func (p *ContinuousEpochProcessor) Add(epoch observation.Epoch) error {
	if p.pending == nil {
		p.pending = &epoch
		return nil
	}
	if epoch.Time.Before(p.pending.Time) {
		return fmt.Errorf("epochs out of order: %s follows %s", epoch.Time, p.pending.Time)
	}
	if epoch.Time.Equal(p.pending.Time) {
		if epoch.AntennaIndex != p.pending.AntennaIndex {
			return fmt.Errorf("cannot merge antenna %d and %d at %s", p.pending.AntennaIndex, epoch.AntennaIndex, epoch.Time)
		}
		merged, err := observation.CombineEpochs(*p.pending, epoch)
		if err != nil {
			return err
		}
		merged.AntennaIndex = epoch.AntennaIndex
		p.pending = &merged
		return nil
	}
	if err := p.emitPending(); err != nil {
		return err
	}
	p.pending = &epoch
	return nil
}

func (p *ContinuousEpochProcessor) Flush() error {
	return p.emitPending()
}

func (p *ContinuousEpochProcessor) emitPending() error {
	if p.pending == nil {
		return nil
	}
	p.tracker.Apply(p.pending)
	epoch := *p.pending
	p.pending = nil
	return p.emit(epoch)
}
