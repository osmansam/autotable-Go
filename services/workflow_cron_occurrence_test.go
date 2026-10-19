package services

import (
	"github.com/osmansam/autotableGo/models"
	"testing"
	"time"
)

func TestCronEveryRemainsEpochAnchoredAcrossRestart(t *testing.T) {
	for _, test := range []struct {
		spec string
		now  time.Time
		want time.Time
	}{
		{"@every 90s", time.Unix(1000, 700000000), time.Unix(1080, 0)},
		{"@every 90s", time.Unix(1010, 0), time.Unix(1080, 0)},
		{"@every 90s", time.Unix(1085, 0), time.Unix(1170, 0)},
		{"@every 1500ms", time.Unix(1000, 700000000), time.Unix(1001, 0)},
	} {
		schedule, err := parseCronSchedule(models.DynamicWorkflow{Schedule: test.spec})
		if err != nil {
			t.Fatal(err)
		}
		if at := schedule.Next(test.now); !at.Equal(test.want) {
			t.Fatalf("%s next=%v want=%v", test.spec, at, test.want)
		}
	}
}
func TestCronTimezoneAndDSTUseDistinctUTCInstants(t *testing.T) {
	for _, test := range []struct {
		spec                 string
		start, first, second time.Time
	}{
		{"30 1 * * *", time.Date(2026, 11, 1, 6, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC), time.Date(2026, 11, 1, 7, 30, 0, 0, time.UTC)},
		{"30 2 * * *", time.Date(2026, 3, 7, 8, 0, 0, 0, time.UTC), time.Date(2026, 3, 7, 8, 30, 0, 0, time.UTC), time.Date(2026, 3, 9, 7, 30, 0, 0, time.UTC)},
	} {
		schedule, err := parseCronSchedule(models.DynamicWorkflow{Schedule: test.spec, Timezone: "America/Chicago"})
		if err != nil {
			t.Fatal(err)
		}
		first := schedule.Next(test.start)
		second := schedule.Next(first)
		if !first.Equal(test.first) || !second.Equal(test.second) {
			t.Fatalf("DST instants %v/%v want %v/%v", first, second, test.first, test.second)
		}
	}
}
