package report_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/normalize"
	"github.com/MyauDev/vekst/core/internal/report"
	"github.com/MyauDev/vekst/core/internal/review"
)

// These run against a live, migrated Postgres reached as vekst_app, the same
// arrangement core/internal/db and core/internal/review use. Row-level
// security means nothing against a superuser, so there is no in-memory version
// of any of this -- and the isolation test below is the reason the distinction
// matters.
//
// The arithmetic is tested in pnl_test.go, without a database, because it is a
// pure function. These test the things a query gets wrong: which rows it
// reaches, which it must not, and what it reads off them.

const (
	taxonomyV = "v1"
	rulesetV  = "v1"

	// Seeded leaves, each in a section this change names. A classification may
	// target only a leaf that is not computed -- migration 007 reuses 006's
	// trigger to say so.
	revenueCode     = "0101"       // NET SALES
	costOfSalesCode = "02"         // CS
	opexCode        = "0401010204" // OPEX, Bank commission
	payrollCode     = "0404"       // OPEX, and requires_allocation
	capexCode       = "08"         // non-P&L
)

type fixture struct {
	db      *db.DB
	svc     *report.Service
	reviews *review.Service

	org    db.OrgID
	orgID  uuid.UUID
	owner  uuid.UUID
	entity uuid.UUID

	// One batch per source kind: migration 007's constraint trigger holds a
	// transaction's own source_kind equal to its batch's, so a fixture cannot
	// fake a bank row into a ledger import.
	bankBatch   uuid.UUID
	ledgerBatch uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping a test that needs a live, migrated Postgres")
	}
	d, err := db.New(context.Background(), db.Config{URL: url})
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(d.Close)

	owner := seedUser(t, d)
	org, err := d.CreateOrganization(context.Background(), db.NewOrganization{
		Name:         "Report " + uuid.NewString(),
		Country:      "NO",
		BaseCurrency: "NOK",
		EntityName:   "Report AS",
		CreatorID:    owner,
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	f := &fixture{
		db:      d,
		svc:     report.New(d),
		reviews: review.New(d, review.HumanVersions(taxonomyV, rulesetV)),
		org:     org,
		orgID:   org.UUID(),
		owner:   owner,
	}
	f.seed(t)
	return f
}

func seedUser(t *testing.T, d *db.DB) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := d.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`,
			uuid.NewString()+"@report.test").Scan(&id)
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

func (f *fixture) seed(t *testing.T) {
	t.Helper()
	var e pgtype.UUID
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id FROM entities LIMIT 1`).Scan(&e); err != nil {
			return fmt.Errorf("reading the entity: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO accounts (org_id, entity_id, name, currency)
			VALUES (app_current_org(), $1, 'Main', 'NOK')`, e); err != nil {
			return fmt.Errorf("seeding account: %w", err)
		}
		for kind, into := range map[string]*uuid.UUID{
			"bank": &f.bankBatch, "ledger": &f.ledgerBatch,
		} {
			var b pgtype.UUID
			// 'imported' is the terminal state of migration 008's machine:
			// these rows exist as though a file had been through the whole of
			// ingest, which is the only state a reportable transaction can
			// belong to.
			if err := tx.QueryRow(ctx, `
				INSERT INTO import_batches (
					org_id, entity_id, source_kind, status, uploaded_by,
					file_name, declared_bytes, declared_type, file_key,
					upload_expires_at,
					file_sha256, byte_length, content_type)
				VALUES (app_current_org(), $1, $2, 'imported', $3,
					$2 || '-march.csv', 1024, 'text/csv', 'uploads/' || gen_random_uuid(),
					now() + interval '1 hour',
					sha256($4::text::bytea), 1024, 'text/csv')
				RETURNING id`, e, kind, pgtype.UUID{Bytes: f.owner, Valid: true},
				uuid.NewString()).Scan(&b); err != nil {
				return fmt.Errorf("seeding the %s batch: %w", kind, err)
			}
			*into = uuid.UUID(b.Bytes)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	f.entity = uuid.UUID(e.Bytes)
}

// txn is one transaction to plant. Amounts are signed as the ledger stores
// them: money in positive, money out negative.
type txn struct {
	bookedOn string // YYYY-MM-DD; defaults to 2026-03-15
	kind     string // bank | ledger; defaults to bank
	key      string
	amount   int64

	// The leaf to classify it into. Empty leaves the row unclassified, which
	// is not a missing value -- it is the fact that puts it in a bucket.
	code string
}

func (f *fixture) insert(t *testing.T, rows ...txn) []uuid.UUID {
	t.Helper()
	ids := make([]uuid.UUID, 0, len(rows))
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		for i, r := range rows {
			if r.bookedOn == "" {
				r.bookedOn = "2026-03-15"
			}
			if r.kind == "" {
				r.kind = "bank"
			}
			if r.key == "" {
				r.key = fmt.Sprintf("tax:%d", i)
			}
			batch := f.bankBatch
			if r.kind == "ledger" {
				batch = f.ledgerBatch
			}

			var id pgtype.UUID
			if err := tx.QueryRow(ctx, `
				INSERT INTO transactions (
					org_id, entity_id, account_id, batch_id, source_kind,
					booked_on, amount_minor, currency,
					counterparty_raw, counterparty_key, description_raw,
					description_norm, normalize_version, dedup_hash)
				SELECT app_current_org(), $1, a.id, $2, $3,
				       $4::date, $5, 'NOK',
				       $6, $6, $6, $6, $7, $8
				  FROM accounts a WHERE a.entity_id = $1 LIMIT 1
				RETURNING id`,
				pgtype.UUID{Bytes: f.entity, Valid: true},
				pgtype.UUID{Bytes: batch, Valid: true},
				r.kind, r.bookedOn, r.amount, r.key, normalize.Version,
				uuid.NewString(),
			).Scan(&id); err != nil {
				return fmt.Errorf("row %d: %w", i, err)
			}
			ids = append(ids, uuid.UUID(id.Bytes))

			if r.code != "" {
				if err := classify(ctx, tx, uuid.UUID(id.Bytes), r.code, "L1", "engine-1"); err != nil {
					return fmt.Errorf("row %d: %w", i, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seeding transactions: %v", err)
	}
	return ids
}

// classify writes a live classification. Written as SQL rather than through
// core/internal/ledger because these tests need to plant engine versions that
// differ from each other, which is the point of the versions test below.
func classify(ctx context.Context, tx pgx.Tx, txnID uuid.UUID, code, layer, engineVersion string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO classifications (
			org_id, transaction_id, category_id, engine_layer, confidence,
			taxonomy_version, ruleset_version, engine_version, normalize_version)
		SELECT app_current_org(), $1, c.id, $2, 1.000, $3, $4, $5, $6
		  FROM categories c
		 WHERE c.taxonomy_version = $3 AND c.code = $7 AND c.org_id IS NULL`,
		pgtype.UUID{Bytes: txnID, Valid: true}, layer,
		taxonomyV, rulesetV, engineVersion, normalize.Version, code)
	return err
}

func (f *fixture) pnl(t *testing.T, basis report.Basis) report.Report {
	t.Helper()
	return f.pnlAs(t, f.owner, f.entity, basis)
}

func (f *fixture) pnlAs(t *testing.T, user, entity uuid.UUID, basis report.Basis) report.Report {
	t.Helper()
	r, err := f.svc.ManagementPNL(context.Background(), user, f.orgID, report.Request{
		EntityID:    entity,
		From:        "2026-03",
		To:          "2026-03",
		Granularity: report.Monthly,
		Basis:       basis,
	})
	if err != nil {
		t.Fatalf("ManagementPNL: %v", err)
	}
	return r
}

func lineTotal(t *testing.T, r report.Report, code string) money.Money {
	t.Helper()
	for _, l := range r.Lines {
		if l.Code == code {
			return l.Total.Amount
		}
	}
	t.Fatalf("no line %q in the report", code)
	return money.Money{}
}

// ---------------------------------------------------------------------------
// Task 6.1 -- cross-tenant isolation
// ---------------------------------------------------------------------------

// Organisation A's report contains none of B's rows, and asking for B's entity
// answers the same way as asking for an entity that was never created.
//
// The second half is the one worth writing down. Row-level security makes the
// first half almost automatic; what it does not do is stop an error message
// from confirming that somebody else's identifier exists. "Not found" and "not
// yours" have to be the same answer, or the endpoint is a membership oracle
// with a report attached.
func TestOneOrganisationsReportContainsNoOthersRows(t *testing.T) {
	a := newFixture(t)
	b := newFixture(t)

	a.insert(t, txn{amount: 1_000_00, code: revenueCode})
	b.insert(t, txn{amount: 9_999_00, code: revenueCode})

	r := a.pnl(t, report.BasisBank)
	if got := lineTotal(t, r, report.NetSales); got.MinorUnits != 1_000_00 {
		t.Errorf("NET SALES = %d, want 100000 -- the other organisation's 999900 leaked in",
			got.MinorUnits)
	}

	// B's entity, asked for by A.
	foreign := a.pnlAs(t, a.owner, b.entity, report.BasisBank)
	// And an entity that never existed.
	invented := a.pnlAs(t, a.owner, uuid.New(), report.BasisBank)

	for name, r := range map[string]report.Report{"another org's entity": foreign, "no such entity": invented} {
		for _, l := range r.Lines {
			if l.Total.Amount.MinorUnits != 0 {
				t.Errorf("%s: line %s = %d, want an empty report",
					name, l.Code, l.Total.Amount.MinorUnits)
			}
		}
		for bucket, total := range r.Totals {
			if total.MinorUnits != 0 {
				t.Errorf("%s: bucket %s = %d, want an empty report", name, bucket, total.MinorUnits)
			}
		}
	}
}

// A non-member is refused, and refused the same way whether the organisation
// exists or not.
func TestANonMemberGetsNoReport(t *testing.T) {
	f := newFixture(t)
	stranger := seedUser(t, f.db)

	_, err := f.svc.ManagementPNL(context.Background(), stranger, f.orgID, report.Request{
		EntityID: f.entity, From: "2026-03", To: "2026-03",
		Granularity: report.Monthly, Basis: report.BasisBank,
	})
	if got := codeOf(err); got != report.CodeForbidden {
		t.Fatalf("error code = %q, want %q", got, report.CodeForbidden)
	}

	_, err = f.svc.ManagementPNL(context.Background(), stranger, uuid.New(), report.Request{
		EntityID: f.entity, From: "2026-03", To: "2026-03",
		Granularity: report.Monthly, Basis: report.BasisBank,
	})
	if got := codeOf(err); got != report.CodeForbidden {
		t.Errorf("a non-existent organisation answered %q, not %q -- the two must be "+
			"indistinguishable", got, report.CodeForbidden)
	}
}

// Task 5.6. Reading the numbers is the whole reason the viewer role exists.
func TestAViewerMayReadTheReport(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{amount: 500_00, code: revenueCode})

	viewer := seedUser(t, f.db)
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO memberships (org_id, user_id, role) VALUES (app_current_org(), $1, 'viewer')`,
			pgtype.UUID{Bytes: viewer, Valid: true})
		return err
	})
	if err != nil {
		t.Fatalf("seeding a viewer: %v", err)
	}

	r := f.pnlAs(t, viewer, f.entity, report.BasisBank)
	if got := lineTotal(t, r, report.NetSales); got.MinorUnits != 500_00 {
		t.Errorf("a viewer's NET SALES = %d, want 50000", got.MinorUnits)
	}
}

// ---------------------------------------------------------------------------
// Task 6.2 -- only the live classification counts
// ---------------------------------------------------------------------------

// A superseded classification is as though it had not been made, and a
// retracted one puts its money back in the unclassified bucket rather than
// nowhere.
//
// The two are different events and the report has to tell them apart:
// supersession names the classification that replaced this one, so the money
// moves to another line; retraction has no successor, so the money goes back to
// being money nobody has answered for. A report that treated a retraction as a
// supersession would quietly lose it.
func TestOnlyTheLiveClassificationCounts(t *testing.T) {
	f := newFixture(t)
	ids := f.insert(t,
		txn{key: "tax:moved", amount: -300_00, code: opexCode},
		txn{key: "tax:taken-back", amount: -70_00, code: opexCode},
	)

	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		// A correction: the OPEX row was really cost of sales. Written as a
		// second engine pass rather than a human one, because a human
		// classification must name its decider -- migration 007 checks it --
		// and who corrected it is not what this test is about.
		var oldID pgtype.UUID
		if err := tx.QueryRow(ctx,
			`SELECT id FROM classifications WHERE transaction_id = $1`,
			pgtype.UUID{Bytes: ids[0], Valid: true}).Scan(&oldID); err != nil {
			return err
		}
		if err := classify(ctx, tx, ids[0], costOfSalesCode, "L1", "engine-2"); err != nil {
			return err
		}
		var newID pgtype.UUID
		if err := tx.QueryRow(ctx, `
			SELECT id FROM classifications
			 WHERE transaction_id = $1 AND id <> $2`,
			pgtype.UUID{Bytes: ids[0], Valid: true}, oldID).Scan(&newID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE classifications SET superseded_by = $2 WHERE id = $1`, oldID, newID); err != nil {
			return err
		}

		// A retraction: somebody undid the answer and put nothing in its place.
		_, err := tx.Exec(ctx, `
			UPDATE classifications SET retracted_at = now(), retracted_by = $2
			 WHERE transaction_id = $1`,
			pgtype.UUID{Bytes: ids[1], Valid: true},
			pgtype.UUID{Bytes: f.owner, Valid: true})
		return err
	})
	if err != nil {
		t.Fatalf("superseding and retracting: %v", err)
	}

	r := f.pnl(t, report.BasisBank)

	// Costs print positive, so the corrected 300.00 is on CS and not on OPEX.
	if got := lineTotal(t, r, report.CS); got.MinorUnits != 300_00 {
		t.Errorf("CS = %d, want 30000: the correction did not move the money", got.MinorUnits)
	}
	if got := lineTotal(t, r, report.OPEX); got.MinorUnits != 0 {
		t.Errorf("OPEX = %d, want 0: the superseded classification still counts", got.MinorUnits)
	}
	if got := r.Totals[report.BucketUnclassified]; got.MinorUnits != -70_00 {
		t.Errorf("unclassified = %d, want -7000: a retracted row must return to the bucket, "+
			"not vanish", got.MinorUnits)
	}
}

// ---------------------------------------------------------------------------
// Task 6.3 -- one basis
// ---------------------------------------------------------------------------

// An invoice and the payment that settles it are two rows describing one event.
// A line summing both counts the money twice, and nothing about the result
// looks wrong.
func TestALedgerRowAndABankRowDoNotBothReachALine(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		txn{kind: "ledger", key: "tax:acme", amount: 1_000_00, code: revenueCode},
		txn{kind: "bank", key: "tax:acme", amount: 1_000_00, code: revenueCode},
	)

	bank := f.pnl(t, report.BasisBank)
	if got := lineTotal(t, bank, report.NetSales); got.MinorUnits != 1_000_00 {
		t.Errorf("bank basis NET SALES = %d, want 100000 -- the ledger row was summed too",
			got.MinorUnits)
	}
	// The other side is evidence, not silence.
	if got := bank.Totals[report.BucketOtherBasis]; got.MinorUnits != 1_000_00 {
		t.Errorf("other basis = %d, want 100000", got.MinorUnits)
	}

	ledger := f.pnl(t, report.BasisLedger)
	if got := lineTotal(t, ledger, report.NetSales); got.MinorUnits != 1_000_00 {
		t.Errorf("ledger basis NET SALES = %d, want 100000", got.MinorUnits)
	}
	if got := ledger.Totals[report.BucketOtherBasis]; got.MinorUnits != 1_000_00 {
		t.Errorf("other basis = %d, want 100000", got.MinorUnits)
	}
	if ledger.Spec.Basis != report.BasisLedger {
		t.Errorf("the report does not state its basis: %q", ledger.Spec.Basis)
	}
}

// The buckets a category's own flags decide, read off the database rather than
// asserted in Go: payroll awaiting allocation is its own figure, and CAPEX
// reaches no line at all.
func TestTheFlagBucketsComeFromTheTaxonomy(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		txn{key: "tax:wages", amount: -900_00, code: payrollCode},
		txn{key: "tax:lathe", amount: -400_00, code: capexCode},
		txn{key: "tax:rent", amount: -100_00, code: opexCode},
	)

	r := f.pnl(t, report.BasisBank)
	if got := r.Totals[report.BucketUnallocated]; got.MinorUnits != -900_00 {
		t.Errorf("unallocated = %d, want -90000", got.MinorUnits)
	}
	if got := r.Totals[report.BucketNonPNL]; got.MinorUnits != -400_00 {
		t.Errorf("non-P&L = %d, want -40000", got.MinorUnits)
	}
	// Only the rent reached OPEX: payroll is known to be payroll and not known
	// to be any department's, and attributing it would be a guess printed as a
	// figure.
	if got := lineTotal(t, r, report.OPEX); got.MinorUnits != 100_00 {
		t.Errorf("OPEX = %d, want 10000 -- the unallocated payroll was attributed", got.MinorUnits)
	}
}

// ---------------------------------------------------------------------------
// Task 6.4 -- the versions are the rows'
// ---------------------------------------------------------------------------

// The versions on the response are read off the classifications the report
// summed, not off a constant in this binary.
//
// A constant says what this build would classify with today; a report says what
// its figures were classified with, and the two are the same string only until
// the next deploy. The test plants two engine versions precisely because no
// constant could produce that answer.
func TestTheVersionsAreTheOnesOnTheRows(t *testing.T) {
	f := newFixture(t)
	ids := f.insert(t, txn{key: "tax:a", amount: 100_00}, txn{key: "tax:b", amount: 200_00})

	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		if err := classify(ctx, tx, ids[0], revenueCode, "L1", "engine-7"); err != nil {
			return err
		}
		return classify(ctx, tx, ids[1], revenueCode, "L1", "engine-8")
	})
	if err != nil {
		t.Fatalf("classifying: %v", err)
	}

	r := f.pnl(t, report.BasisBank)
	if want := []string{"engine-7", "engine-8"}; !equal(r.Spec.EngineVersions, want) {
		t.Errorf("engine versions = %v, want %v -- a report summing rows classified under "+
			"two engine versions was produced under two, and naming one of them is a claim "+
			"about reproducibility that is not true", r.Spec.EngineVersions, want)
	}
	if want := []string{taxonomyV}; !equal(r.Spec.TaxonomyVersions, want) {
		t.Errorf("taxonomy versions = %v, want %v", r.Spec.TaxonomyVersions, want)
	}
	if want := []string{normalize.Version}; !equal(r.Spec.NormalizeVersions, want) {
		t.Errorf("normalize versions = %v, want %v", r.Spec.NormalizeVersions, want)
	}

	// A report over a range with no classification names no version, rather
	// than naming this binary's.
	empty, err := f.svc.ManagementPNL(context.Background(), f.owner, f.orgID, report.Request{
		EntityID: f.entity, From: "2025-01", To: "2025-01",
		Granularity: report.Monthly, Basis: report.BasisBank,
	})
	if err != nil {
		t.Fatalf("ManagementPNL: %v", err)
	}
	if len(empty.Spec.EngineVersions) != 0 {
		t.Errorf("a report summing nothing named versions %v", empty.Spec.EngineVersions)
	}
}

// ---------------------------------------------------------------------------
// Task 6.5 -- end to end, through the queue that produced the answer
// ---------------------------------------------------------------------------

// Settle a counterparty in the review queue, then print the report: the amount
// the person was shown is on the line the category they chose belongs to.
//
// This is the seam the two changes share, and the only test that crosses it.
// Everything between -- the decision row, the vendor memory, the classification
// per covered transaction, the section the category hangs under -- is written by
// one change and read by the other, and a disagreement anywhere along it shows
// up here as money on the wrong line rather than as a failure either side
// could see alone.
func TestADecisionInTheQueueLandsOnTheReport(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		txn{key: "tax:landlord", amount: -250_00},
		txn{key: "tax:landlord", amount: -250_00, bookedOn: "2026-03-20"},
	)

	// Before: nothing is on a line, and the whole amount is visible as
	// unclassified rather than absent.
	before := f.pnl(t, report.BasisBank)
	if got := lineTotal(t, before, report.OPEX); got.MinorUnits != 0 {
		t.Fatalf("OPEX before the decision = %d, want 0", got.MinorUnits)
	}
	if got := before.Totals[report.BucketUnclassified]; got.MinorUnits != -500_00 {
		t.Fatalf("unclassified before the decision = %d, want -50000", got.MinorUnits)
	}

	decision, err := f.reviews.Resolve(context.Background(), f.owner, f.orgID, f.entity,
		"tax:landlord", review.OutcomeCategorised, opexCode)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.CoveredCount != 2 {
		t.Fatalf("the decision covered %d rows, want 2", decision.CoveredCount)
	}

	after := f.pnl(t, report.BasisBank)
	// Costs print positive: the owner reads "OPEX 500.00", and the store holds
	// -50000.
	if got := lineTotal(t, after, report.OPEX); got.MinorUnits != 500_00 {
		t.Errorf("OPEX after the decision = %d, want 50000", got.MinorUnits)
	}
	if got := after.Totals[report.BucketUnclassified]; got.MinorUnits != 0 {
		t.Errorf("unclassified after the decision = %d, want 0", got.MinorUnits)
	}
	// The decision's own figure and the report's agree, in opposite signs,
	// because one is the store's and the other is the page's.
	if decision.Covered.MinorUnits != -500_00 {
		t.Errorf("the decision covered %d, the report shows 50000", decision.Covered.MinorUnits)
	}

	// And undoing it puts the money back where it was, which is the property
	// that makes the queue safe to work quickly.
	if _, err := f.reviews.Undo(context.Background(), f.owner, f.orgID, decision.ID); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	undone := f.pnl(t, report.BasisBank)
	if got := lineTotal(t, undone, report.OPEX); got.MinorUnits != 0 {
		t.Errorf("OPEX after the undo = %d, want 0", got.MinorUnits)
	}
	if got := undone.Totals[report.BucketUnclassified]; got.MinorUnits != -500_00 {
		t.Errorf("unclassified after the undo = %d, want -50000", got.MinorUnits)
	}
}

// ---------------------------------------------------------------------------
// The range, read off the database rather than asserted in Go
// ---------------------------------------------------------------------------

// A closed interval on booked_on, and the columns of an empty month are zeros
// rather than absent.
func TestTheRangeIsClosedAndEveryMonthHasAColumn(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		// The first and last days of the range: an exclusive end, or a range
		// built from the first of the month at both ends, would drop one.
		txn{key: "tax:first", bookedOn: "2026-02-01", amount: 100_00, code: revenueCode},
		txn{key: "tax:last", bookedOn: "2026-04-30", amount: 300_00, code: revenueCode},
		txn{key: "tax:outside", bookedOn: "2026-05-01", amount: 999_00, code: revenueCode},
	)

	r, err := f.svc.ManagementPNL(context.Background(), f.owner, f.orgID, report.Request{
		EntityID: f.entity, From: "2026-02", To: "2026-04",
		Granularity: report.Monthly, Basis: report.BasisBank,
	})
	if err != nil {
		t.Fatalf("ManagementPNL: %v", err)
	}

	if want := []string{"2026-02", "2026-03", "2026-04"}; !equal(r.Spec.Periods, want) {
		t.Fatalf("periods = %v, want %v", r.Spec.Periods, want)
	}
	if got := lineTotal(t, r, report.NetSales); got.MinorUnits != 400_00 {
		t.Errorf("NET SALES = %d, want 40000: either an edge of the range was dropped or "+
			"May was included", got.MinorUnits)
	}
	for _, l := range r.Lines {
		if l.Code != report.NetSales {
			continue
		}
		// March traded nothing. Its column is a zero, not a gap: a month
		// missing from a report is indistinguishable from a month with no
		// trade, and only one of those is a business fact.
		if len(l.ByPeriod) != 3 {
			t.Fatalf("NET SALES has %d columns, want 3", len(l.ByPeriod))
		}
		if l.ByPeriod[1].Amount.MinorUnits != 0 {
			t.Errorf("March = %d, want 0", l.ByPeriod[1].Amount.MinorUnits)
		}
		if l.ByPeriod[1].Amount.CurrencyCode != "NOK" {
			t.Errorf("an empty column carries no currency: %q", l.ByPeriod[1].Amount.CurrencyCode)
		}
	}
}

func codeOf(err error) string {
	var coded *report.Err
	if !errors.As(err, &coded) {
		return ""
	}
	return coded.Code
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
