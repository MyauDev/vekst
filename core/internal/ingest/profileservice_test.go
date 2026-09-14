package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
)

// Task 6.4: a bad canonical field is a write error, caught by migration
// 011's own constraint trigger at the moment the profile is written -- not
// discovered later when a file using it is parsed.
func TestABadCanonicalFieldIsAWriteError(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, _ := testOrgAndEntity(t, env.db, owner)

	_, err := env.svc.CreateImportProfile(ctx, owner, org.UUID(), ProfileInput{
		Name:       "typo profile",
		SourceKind: SourceKindBank,
		ColumnMap:  ColumnMap{"Назначение": "descrption"},
	})
	var badField *ErrInvalidCanonicalField
	if !errors.As(err, &badField) {
		t.Fatalf("CreateImportProfile with a bad canonical field = %v, want *ErrInvalidCanonicalField", err)
	}
	if badField.Field != "descrption" {
		t.Errorf("ErrInvalidCanonicalField.Field = %q, want %q", badField.Field, "descrption")
	}
}

// Task 6.5: source kinds must agree. A profile created for one source_kind
// is refused on a batch of the other, so the amount column of a ledger
// export can never be resolved against a bank profile's column_map.
func TestSourceKindsMustAgree(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	profile, err := env.svc.CreateImportProfile(ctx, owner, org.UUID(), ProfileInput{
		Name:       "ledger profile",
		SourceKind: SourceKindLedger,
		ColumnMap:  ColumnMap{"Назначение": "description"},
	})
	if err != nil {
		t.Fatalf("CreateImportProfile: %v", err)
	}

	_, _, err = env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID:        entityID,
		SourceKind:      SourceKindBank,
		FileName:        "x.csv",
		DeclaredBytes:   10,
		DeclaredType:    "text/csv",
		ImportProfileID: profile.ID,
	})
	if !errors.Is(err, ErrProfileSourceKindMismatch) {
		t.Errorf("CreateImportBatch with a ledger profile on a bank batch = %v, want ErrProfileSourceKindMismatch", err)
	}
}

// Task 5.1 (the other half of it): a profile belonging to another
// organisation is refused identically to one that does not exist at all --
// row-level security makes GetImportProfile return pgx.ErrNoRows either way,
// so CreateImportBatch cannot and does not distinguish them.
func TestForeignProfileIsRefusedLikeANonexistentOne(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	ownerA := testUser(t, env.db)
	ownerB := testUser(t, env.db)
	orgA, _ := testOrgAndEntity(t, env.db, ownerA)
	orgB, entityB := testOrgAndEntity(t, env.db, ownerB)

	profileA, err := env.svc.CreateImportProfile(ctx, ownerA, orgA.UUID(), ProfileInput{
		Name:       "org A's profile",
		SourceKind: SourceKindBank,
		ColumnMap:  ColumnMap{"Назначение": "description"},
	})
	if err != nil {
		t.Fatalf("CreateImportProfile: %v", err)
	}

	_, _, err = env.svc.CreateImportBatch(ctx, ownerB, orgB.UUID(), CreateBatchInput{
		EntityID:        entityB,
		SourceKind:      SourceKindBank,
		FileName:        "x.csv",
		DeclaredBytes:   10,
		DeclaredType:    "text/csv",
		ImportProfileID: profileA.ID,
	})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("CreateImportBatch naming org A's profile from org B = %v, want ErrProfileNotFound", err)
	}

	_, _, errNonexistent := env.svc.CreateImportBatch(ctx, ownerB, orgB.UUID(), CreateBatchInput{
		EntityID:        entityB,
		SourceKind:      SourceKindBank,
		FileName:        "x.csv",
		DeclaredBytes:   10,
		DeclaredType:    "text/csv",
		ImportProfileID: uuid.New(),
	})
	if !errors.Is(errNonexistent, ErrProfileNotFound) {
		t.Errorf("CreateImportBatch naming a profile that never existed = %v, want ErrProfileNotFound", errNonexistent)
	}
}

// Task 6.7: cross-tenant isolation. B can neither read, name, update nor
// delete A's profile, and the failure is the same ErrProfileNotFound a
// nonexistent id produces -- a policy denial, not a distinguishable
// not-found.
func TestProfileCrossTenantIsolation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	ownerA := testUser(t, env.db)
	ownerB := testUser(t, env.db)
	orgA, _ := testOrgAndEntity(t, env.db, ownerA)
	orgB, _ := testOrgAndEntity(t, env.db, ownerB)

	profileA, err := env.svc.CreateImportProfile(ctx, ownerA, orgA.UUID(), ProfileInput{
		Name:       "org A's profile",
		SourceKind: SourceKindBank,
		ColumnMap:  ColumnMap{"Назначение": "description"},
	})
	if err != nil {
		t.Fatalf("CreateImportProfile: %v", err)
	}

	if _, err := env.svc.GetImportProfile(ctx, ownerB, orgB.UUID(), profileA.ID); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("B reading A's profile = %v, want ErrProfileNotFound", err)
	}
	if _, err := env.svc.UpdateImportProfile(ctx, ownerB, orgB.UUID(), profileA.ID, ProfileInput{
		Name:      "renamed by B",
		ColumnMap: ColumnMap{"x": "description"},
	}); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("B updating A's profile = %v, want ErrProfileNotFound", err)
	}
	if err := env.svc.DeleteImportProfile(ctx, ownerB, orgB.UUID(), profileA.ID); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("B deleting A's profile = %v, want ErrProfileNotFound", err)
	}

	profiles, err := env.svc.ListImportProfiles(ctx, ownerB, orgB.UUID())
	if err != nil {
		t.Fatalf("ListImportProfiles: %v", err)
	}
	for _, p := range profiles {
		if p.ID == profileA.ID {
			t.Errorf("B's profile list names A's profile %s", p.ID)
		}
	}
}

// Task 6.8: fail-closed. Every query in profiles.sql raises 42704 outside a
// tenant transaction, the same as every other tenant table in this schema
// (CLAUDE.md: tenant context is fail-closed).
func TestProfileQueriesFailClosedOutsideATenantTransaction(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	someID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	someText := pgtype.Text{String: "x", Valid: true}

	cases := map[string]func(tx pgx.Tx) error{
		"InsertImportProfile": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).InsertImportProfile(ctx, gendb.InsertImportProfileParams{
				OrgID: someID, Name: "x", SourceKind: SourceKindBank,
				ColumnMap: []byte(`{"a":"description"}`),
			})
			return err
		},
		"GetImportProfile": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).GetImportProfile(ctx, someID)
			return err
		},
		"ListImportProfiles": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).ListImportProfiles(ctx)
			return err
		},
		"UpdateImportProfile": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).UpdateImportProfile(ctx, gendb.UpdateImportProfileParams{
				ID: someID, Name: "x", ColumnMap: []byte(`{"a":"description"}`),
				Charset: someText,
			})
			return err
		},
		"DeleteImportProfile": func(tx pgx.Tx) error {
			_, err := gendb.New(tx).DeleteImportProfile(ctx, someID)
			return err
		},
	}

	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			err := env.db.InSystemTx(context.Background(), func(_ context.Context, tx pgx.Tx) error {
				return run(tx)
			})
			if err == nil {
				t.Fatalf("%s succeeded with no tenant context", name)
			}
			if code := pgCode(err); code != "42704" {
				t.Fatalf("%s: SQLSTATE = %q (%v), want 42704", name, code, err)
			}
		})
	}
}

// Task 6.9: a used profile cannot be deleted. import_batches.import_profile_id's
// own ON DELETE RESTRICT is what enforces this; DeleteImportProfile
// translates the resulting foreign-key violation into a named code.
func TestAUsedProfileCannotBeDeleted(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	profile, err := env.svc.CreateImportProfile(ctx, owner, org.UUID(), ProfileInput{
		Name:       "in-use profile",
		SourceKind: SourceKindBank,
		ColumnMap:  ColumnMap{"Назначение": "description"},
	})
	if err != nil {
		t.Fatalf("CreateImportProfile: %v", err)
	}

	body := realFixtureBody(t)
	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID:        entityID,
		SourceKind:      SourceKindBank,
		FileName:        "x.csv",
		DeclaredBytes:   int64(len(body)),
		DeclaredType:    "text/csv",
		ImportProfileID: profile.ID,
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	putBytes(t, upload.URL, upload.Headers, body)
	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload: %v", err)
	}
	// validated is transient now: add-dedup's persist job (change 2.6)
	// chains automatically, and this fixture has nothing to skip or fail
	// on, so it settles at imported.
	env.waitForStatus(t, org, batch.ID, StatusImported)

	if err := env.svc.DeleteImportProfile(ctx, owner, org.UUID(), profile.ID); !errors.Is(err, ErrProfileInUse) {
		t.Errorf("DeleteImportProfile on a profile a batch used = %v, want ErrProfileInUse", err)
	}
}

// Task 6.10: resolved parameters survive. What a completed batch is
// recorded as having been parsed with is a snapshot taken when it was
// parsed (task 5.3) -- editing the profile afterward does not reach back
// into that snapshot.
func TestResolvedParametersSurviveAProfileEdit(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	owner := testUser(t, env.db)
	org, entityID := testOrgAndEntity(t, env.db, owner)

	profile, err := env.svc.CreateImportProfile(ctx, owner, org.UUID(), ProfileInput{
		Name:       "resolved-params profile",
		SourceKind: SourceKindBank,
		ColumnMap:  ColumnMap{"Назначение": "description"},
		DateFormat: "02.01.2006",
	})
	if err != nil {
		t.Fatalf("CreateImportProfile: %v", err)
	}

	body := realFixtureBody(t)
	batch, upload, err := env.svc.CreateImportBatch(ctx, owner, org.UUID(), CreateBatchInput{
		EntityID:        entityID,
		SourceKind:      SourceKindBank,
		FileName:        "x.csv",
		DeclaredBytes:   int64(len(body)),
		DeclaredType:    "text/csv",
		ImportProfileID: profile.ID,
	})
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	putBytes(t, upload.URL, upload.Headers, body)
	if _, err := env.svc.ConfirmImportUpload(ctx, owner, org.UUID(), batch.ID); err != nil {
		t.Fatalf("ConfirmImportUpload: %v", err)
	}
	// validated is transient now: add-dedup's persist job (change 2.6)
	// chains automatically, and this fixture has nothing to skip or fail
	// on, so it settles at imported.
	env.waitForStatus(t, org, batch.ID, StatusImported)

	before := env.rawBatch(t, org, batch.ID)
	if len(before.ResolvedParameters) == 0 {
		t.Fatal("a batch parsed with a profile has no resolved_parameters recorded")
	}
	var resolvedBefore resolvedParameters
	if err := json.Unmarshal(before.ResolvedParameters, &resolvedBefore); err != nil {
		t.Fatalf("unmarshaling resolved_parameters: %v", err)
	}
	if resolvedBefore.DateFormat != "02.01.2006" {
		t.Fatalf("resolved date_fmt = %q, want %q", resolvedBefore.DateFormat, "02.01.2006")
	}

	if _, err := env.svc.UpdateImportProfile(ctx, owner, org.UUID(), profile.ID, ProfileInput{
		Name:       "resolved-params profile",
		ColumnMap:  ColumnMap{"Назначение": "description"},
		DateFormat: "2006-01-02",
	}); err != nil {
		t.Fatalf("UpdateImportProfile: %v", err)
	}

	after := env.rawBatch(t, org, batch.ID)
	var resolvedAfter resolvedParameters
	if err := json.Unmarshal(after.ResolvedParameters, &resolvedAfter); err != nil {
		t.Fatalf("unmarshaling resolved_parameters: %v", err)
	}
	if resolvedAfter.DateFormat != "02.01.2006" {
		t.Errorf("resolved date_fmt after editing the profile = %q, want the original %q", resolvedAfter.DateFormat, "02.01.2006")
	}
}
