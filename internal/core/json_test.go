package core_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
)

// The persisted shape from the spec's Persistence section, with nil
// optionals omitted (an open segment has no endedAt, a running entry no
// worklog) and timestamps in the Jira format.
const specState = `{
  "schemaVersion": 1,
  "trackedTickets": [
    {
      "key": "PUNCH-12",
      "summary": "Wire up the worklog POST",
      "unverified": false,
      "addedAt": "2026-08-09T09:02:41.000-0500",
      "entries": [
        {
          "id": "1E4C9A02-7F3B-4D18-9C51-A0B7E6D24F88",
          "segments": [
            {
              "startedAt": "2026-08-09T09:02:41.000-0500",
              "endedAt": "2026-08-09T09:48:10.000-0500"
            },
            {
              "startedAt": "2026-08-09T10:15:00.000-0500"
            }
          ],
          "held": true,
          "origin": "hand",
          "recovered": false
        }
      ]
    }
  ]
}`

func specValue() core.State {
	ts := func(clock string) core.Timestamp { return at(clock) }
	return core.State{
		SchemaVersion: 1,
		TrackedTickets: []core.TrackedTicket{{
			Key: "PUNCH-12", Summary: "Wire up the worklog POST", AddedAt: ts("09:02:41"),
			Entries: []core.TimeEntry{{
				ID:       "1E4C9A02-7F3B-4D18-9C51-A0B7E6D24F88",
				Segments: []core.Segment{{StartedAt: ts("09:02:41"), EndedAt: ts("09:48:10")}, {StartedAt: ts("10:15")}},
				Held:     true, Origin: core.OriginHand,
			}},
		}},
	}
}

func TestStateMarshalsToTheSpecShape(t *testing.T) {
	got, err := json.MarshalIndent(specValue(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	diff(t, "json", specState, string(got))
}

func TestStateDecodesTheSpecExampleIncludingNulls(t *testing.T) {
	// The spec's example writes nulls; the decoder must accept them too.
	withNulls := `{"schemaVersion":1,"authFailedAt":null,"trackedTickets":[{"key":"PUNCH-12","summary":"Wire up the worklog POST","unverified":false,
	  "addedAt":"2026-08-09T09:02:41.000-0500","entries":[{"id":"1E4C9A02-7F3B-4D18-9C51-A0B7E6D24F88",
	  "segments":[{"startedAt":"2026-08-09T09:02:41.000-0500","endedAt":"2026-08-09T09:48:10.000-0500"},{"startedAt":"2026-08-09T10:15:00.000-0500"}],
	  "held":true,"origin":"hand","recovered":false,"worklog":null}]}]}`
	var got core.State
	if err := json.Unmarshal([]byte(withNulls), &got); err != nil {
		t.Fatal(err)
	}
	diff(t, "state", specValue(), got)
}

func TestWorklogShapes(t *testing.T) {
	cases := []struct {
		name string
		wl   core.Worklog
		want string
	}{
		{
			"pending",
			core.Worklog{State: core.WorklogPending, StartedAt: at("09:00"), TimeSpentSeconds: 1800, Comment: "Fixed the formatter"},
			`{"state":"pending","startedAt":"2026-08-09T09:00:00.000-0500","timeSpentSeconds":1800,"comment":"Fixed the formatter"}`,
		},
		{
			"synced",
			core.Worklog{State: core.WorklogSynced, StartedAt: at("09:00"), TimeSpentSeconds: 1800, WorklogID: "110001", SyncedAt: at("10:00")},
			`{"state":"synced","startedAt":"2026-08-09T09:00:00.000-0500","timeSpentSeconds":1800,"worklogId":"110001","syncedAt":"2026-08-09T10:00:00.000-0500"}`,
		},
		{
			"failed",
			core.Worklog{State: core.WorklogFailed, StartedAt: at("09:00"), TimeSpentSeconds: 1800, Message: "401 Unauthorized", FailedAt: at("10:00")},
			`{"state":"failed","startedAt":"2026-08-09T09:00:00.000-0500","timeSpentSeconds":1800,"message":"401 Unauthorized","failedAt":"2026-08-09T10:00:00.000-0500"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.wl)
			if err != nil {
				t.Fatal(err)
			}
			diff(t, "json", tc.want, string(b))
			var back core.Worklog
			if err := json.Unmarshal(b, &back); err != nil {
				t.Fatal(err)
			}
			diff(t, "round trip", tc.wl, back)
		})
	}
}

func TestHeartbeatShape(t *testing.T) {
	b, err := json.Marshal(core.Heartbeat{SchemaVersion: 1, LastSeen: at("09:00")})
	if err != nil {
		t.Fatal(err)
	}
	diff(t, "json", `{"schemaVersion":1,"lastSeen":"2026-08-09T09:00:00.000-0500"}`, string(b))
}

func TestTimestampsKeepMillisecondsAndOffset(t *testing.T) {
	ts := core.At(time.Date(2026, 8, 9, 9, 0, 0, 123_456_789, time.UTC))
	b, _ := json.Marshal(ts)
	diff(t, "utc", `"2026-08-09T09:00:00.123+0000"`, string(b))
	var back core.Timestamp
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Equal(ts.Time) {
		t.Errorf("round trip %v != %v", back, ts)
	}
	if err := json.Unmarshal([]byte(`"yesterday"`), &back); err == nil {
		t.Error("want an error for a non-timestamp")
	}
}

func TestApplyStoresMillisecondTimes(t *testing.T) {
	now := at("09:00").Add(456_789 * time.Microsecond) // 09:00:00.456789
	got, _, err := core.Apply(world(""), core.Start{Key: "AA-1", Origin: hand, NewEntryID: "E"}, now)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got.State)
	var back core.State
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	diff(t, "lossless round trip", got.State, back)
}
