package migrate

import "testing"

// RequiredVersion must see both the SQL migration (00001) and the Go
// migration (00002_river.go, registered via init()) -- design Q5's whole
// point is that this number is derived, never declared, so it cannot
// silently disagree with the files it is supposed to describe.
func TestRequiredVersionSeesBothSQLAndGoMigrations(t *testing.T) {
	got, err := RequiredVersion()
	if err != nil {
		t.Fatalf("RequiredVersion: %v", err)
	}
	if got != 2 {
		t.Fatalf("RequiredVersion() = %d, want 2 (00001 SQL + 00002 Go)", got)
	}
}
