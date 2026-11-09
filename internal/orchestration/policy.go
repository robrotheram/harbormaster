package orchestration

import (
	"agent-workbench/internal/shared"
	"time"
)

// AdvanceDue coalesces missed intervals into one durable occurrence and moves
// the next trigger strictly beyond now. It does not replay missed side effects.
func AdvanceDue(schedule shared.Schedule, now time.Time) (shared.Schedule, *shared.Run) {
	if !schedule.Enabled || schedule.NextRunAt.After(now) {
		return schedule, nil
	}
	run := Occurrence(schedule, "due:"+schedule.NextRunAt.Format(time.RFC3339Nano), now)
	if schedule.IntervalSeconds == 0 {
		schedule.Enabled = false
	} else {
		step := time.Duration(schedule.IntervalSeconds) * time.Second
		missed := now.Sub(schedule.NextRunAt) / step
		schedule.NextRunAt = schedule.NextRunAt.Add((missed + 1) * step)
	}
	return schedule, &run
}
