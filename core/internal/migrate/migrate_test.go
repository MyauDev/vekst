package migrate

import "testing"

// RequiredVersion must see the SQL migrations (00001, 00003, 00004) and the Go
// migration (00002_river.go, registered via init()) alike -- design Q5's
// whole point is that this number is derived, never declared, so it cannot
// silently disagree with the files it is supposed to describe.
//
// This number is expected to move with every migration added. It is asserted
// rather than computed because the failure it catches is a migration the
// embed or the init() registration does not see: /readyz compares this
// against what goose has applied, so a version that is silently too low
// reports a stale schema as ready.
func TestRequiredVersionSeesEverySQLAndGoMigration(t *testing.T) {
	got, err := RequiredVersion()
	if err != nil {
		t.Fatalf("RequiredVersion: %v", err)
	}
	if got != 5 {
		t.Fatalf("RequiredVersion() = %d, want 5 (00001 + 00003 + 00004 + 00005 SQL, 00002 Go)", got)
	}
}
