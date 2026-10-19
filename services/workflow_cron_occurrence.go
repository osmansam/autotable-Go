package services

import "time"

// Preserve shared interval phase across replicas/restarts; cron's default
// ConstantDelaySchedule would otherwise anchor @every to each process startup.
type workflowCronInterval struct{ delay time.Duration }

func (s workflowCronInterval) Next(now time.Time) time.Time {
	seconds := int64(s.delay / time.Second)
	remainder := now.Unix() % seconds
	if remainder < 0 {
		remainder += seconds
	}
	return time.Unix(now.Unix()-remainder+seconds, 0).UTC()
}
