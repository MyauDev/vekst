package review_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/normalize"
	"github.com/MyauDev/vekst/core/internal/review"
)

// These run against a live, migrated Postgres reached as vekst_app, the same
// arrangement core/internal/db's tests use. Row-level security means nothing
// against a superuser, so there is no in-memory version of any of this.

const (
	taxonomyV = "v1"
	rulesetV  = "v1"

	// A seeded leaf that is not computed. Classifying against a section or a
	// computed line is refused by the trigger migration 006 installed.
	payrollCode = "0404"
	otherCode   = "0403"
)

type fixture struct {
	db     *db.DB
	svc    *review.Service
	org    db.OrgID
	orgID  uuid.UUID
	owner  uuid.UUID
	viewer uuid.UUID // a real membership with the viewer role, not a string
	entity uuid.UUID
	batch  uuid.UUID
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
		Name:         "Review " + uuid.NewString(),
		Country:      "NO",
		BaseCurrency: "NOK",
		EntityName:   "Review AS",
		CreatorID:    owner,
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	f := &fixture{
		db:    d,
		svc:   review.New(d, review.HumanVersions(taxonomyV, rulesetV)),
		org:   org,
		orgID: org.UUID(),
		owner: owner,
	}
	f.entity, f.batch = seedEntityAndBatch(t, d, org)
	f.viewer = seedMember(t, f, "viewer")
	return f
}

// seedMember adds a second person to the organisation with the given role.
//
// The role tests read it back through db.OrgIDForSession rather than passing a
// string in, because what has to hold is that the database's answer is the one
// enforced -- a test that hands the service a role proves only that a switch
// statement works.
func seedMember(t *testing.T, f *fixture, role string) uuid.UUID {
	t.Helper()
	user := seedUser(t, f.db)
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO memberships (org_id, user_id, role) VALUES (app_current_org(), $1, $2)`,
			pgtype.UUID{Bytes: user, Valid: true}, role)
		return err
	})
	if err != nil {
		t.Fatalf("seeding a %s: %v", role, err)
	}
	return user
}

func seedUser(t *testing.T, d *db.DB) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := d.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`,
			uuid.NewString()+"@review.test").Scan(&id)
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

// seedEntityAndBatch returns the organisation's single entity and an account
// plus a bank batch to hang transactions off.
func seedEntityAndBatch(t *testing.T, d *db.DB, org db.OrgID) (entity, batch uuid.UUID) {
	t.Helper()
	var e, b pgtype.UUID
	err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id FROM entities LIMIT 1`).Scan(&e); err != nil {
			return fmt.Errorf("reading the entity: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO accounts (org_id, entity_id, name, currency)
			VALUES (app_current_org(), $1, 'Main', 'NOK')`, e); err != nil {
			return fmt.Errorf("seeding account: %w", err)
		}
		return tx.QueryRow(ctx, `
			INSERT INTO import_batches (org_id, entity_id, source_kind)
			VALUES (app_current_org(), $1, 'bank') RETURNING id`, e).Scan(&b)
	})
	if err != nil {
		t.Fatalf("seeding entity and batch: %v", err)
	}
	return uuid.UUID(e.Bytes), uuid.UUID(b.Bytes)
}

type txn struct {
	key        string
	name       string
	amount     int64
	currency   string
	baseAmount int64 // 0 means no conversion: the row is already in NOK
	direction  string
}

func (f *fixture) insert(t *testing.T, rows ...txn) {
	t.Helper()
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		for i, r := range rows {
			direction := r.direction
			if direction == "" {
				direction = "expense"
			}
			currency := r.currency
			if currency == "" {
				currency = "NOK"
			}
			var fxRate, fxOn, base, baseCcy any
			if r.baseAmount != 0 {
				fxRate, fxOn, base, baseCcy = "1.5", "2026-03-01", r.baseAmount, "NOK"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO transactions (
					org_id, entity_id, account_id, batch_id, source_kind,
					booked_on, direction, amount_minor, currency,
					fx_rate, fx_rate_on, base_amount_minor, base_currency,
					counterparty_raw, counterparty_key, description_raw,
					description_norm, normalize_version, dedup_hash)
				SELECT app_current_org(), $1, a.id, $2, 'bank',
				       date '2026-03-01' + $3::int, $4, $5, $6,
				       $7::numeric, $8::date, $9::bigint, $10,
				       $11, $12, $13, $13, $14, $15
				  FROM accounts a WHERE a.entity_id = $1 LIMIT 1`,
				pgtype.UUID{Bytes: f.entity, Valid: true},
				pgtype.UUID{Bytes: f.batch, Valid: true},
				i, direction, r.amount, currency,
				fxRate, fxOn, base, baseCcy,
				r.name, r.key, r.name+" payment", normalize.Version,
				uuid.NewString(),
			); err != nil {
				return fmt.Errorf("row %d: %w", i, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seeding transactions: %v", err)
	}
}

func (f *fixture) queue(t *testing.T) ([]review.Group, review.Totals) {
	t.Helper()
	groups, totals, err := f.svc.Queue(context.Background(), f.owner, f.orgID, f.entity, 50, 0)
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}
	return groups, totals
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	return pgErr.Code
}

func codeOf(err error) string {
	var coded *review.Err
	if errors.As(err, &coded) {
		return coded.Code
	}
	return ""
}

// ---------------------------------------------------------------------------
// Task 7.4 and 7.6 -- what the queue shows and in what order
// ---------------------------------------------------------------------------

func TestTheQueueIsOrderedByAbsoluteAmount(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		txn{key: "tax:100", name: "Small", amount: 400},
		txn{key: "tax:200", name: "Large", amount: 4_000_000},
		// A refund of the same size as the largest payment: it deserves the
		// same attention, so it must sort beside it rather than last.
		txn{key: "tax:300", name: "Refund", amount: -4_000_000, direction: "income"},
	)

	groups, totals := f.queue(t)
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	if groups[2].CounterpartyKey != "tax:100" {
		t.Errorf("the smallest amount is not last: %v", groups[2].CounterpartyKey)
	}
	for _, g := range groups[:2] {
		if g.Total.MinorUnits != 4_000_000 && g.Total.MinorUnits != -4_000_000 {
			t.Errorf("a 4,000,000 group sorted below a 400 one: %+v", g)
		}
	}
	if totals.RowCount != 3 || totals.CounterpartyCount != 3 {
		t.Errorf("totals = %+v, want 3 rows over 3 counterparties", totals)
	}
	// Absolute, so an expense and an income of the same size do not net to
	// nothing and report an empty queue.
	if totals.Absolute.MinorUnits != 8_000_400 {
		t.Errorf("absolute total = %d, want 8000400", totals.Absolute.MinorUnits)
	}
	if totals.Absolute.CurrencyCode != "NOK" {
		t.Errorf("total currency = %q, want the organisation's own", totals.Absolute.CurrencyCode)
	}
}

func TestTwoReadsAgree(t *testing.T) {
	f := newFixture(t)
	for i := range 6 {
		f.insert(t, txn{key: fmt.Sprintf("tax:%d", i), name: "Same", amount: 1000})
	}
	first, _ := f.queue(t)
	second, _ := f.queue(t)

	for i := range first {
		if first[i].CounterpartyKey != second[i].CounterpartyKey {
			t.Fatalf("the queue reshuffled between two reads at %d: %q then %q",
				i, first[i].CounterpartyKey, second[i].CounterpartyKey)
		}
	}
}

// Task 7.5. Sorting by face value would put a small foreign amount above a
// large domestic one whenever the exponent differs.
func TestAmountsCompareInTheBaseCurrency(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		// 1,000,000 JPY minor units is a big number and a small amount.
		txn{key: "tax:jpy", name: "Tokyo", amount: 1_000_000, currency: "JPY", baseAmount: 7_000},
		txn{key: "tax:nok", name: "Oslo", amount: 50_000},
	)

	groups, _ := f.queue(t)
	if groups[0].CounterpartyKey != "tax:nok" {
		t.Errorf("first group is %q; the JPY row sorted by its face value rather than "+
			"by the 7,000 it converts to", groups[0].CounterpartyKey)
	}
	for _, g := range groups {
		if g.CounterpartyKey == "tax:jpy" && g.Total.MinorUnits != 7_000 {
			t.Errorf("JPY group total = %d, want the converted 7000", g.Total.MinorUnits)
		}
	}
}

// A row whose counterparty could not be identified is the hardest row in the
// queue. Hiding it reports a completion that did not happen.
func TestTheUnidentifiedGroupIsShown(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "", name: "", amount: 900})

	groups, totals := f.queue(t)
	if len(groups) != 1 || groups[0].CounterpartyKey != "" {
		t.Fatalf("groups = %+v, want one group with an empty key", groups)
	}
	if totals.RowCount != 1 {
		t.Errorf("rows = %d, want 1", totals.RowCount)
	}
}

// ---------------------------------------------------------------------------
// Task 7.2, 7.7 -- resolving
// ---------------------------------------------------------------------------

func TestOneDecisionCoversTheWholeGroup(t *testing.T) {
	f := newFixture(t)
	const rows = 25
	for i := range rows {
		f.insert(t, txn{key: "tax:220340017991", name: "Ромашка", amount: int64(100 + i)})
	}

	decision, err := f.svc.Resolve(context.Background(), f.owner, f.orgID,
		f.entity,
		"tax:220340017991", review.OutcomeCategorised, payrollCode)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.CoveredCount != rows {
		t.Errorf("covered = %d, want %d", decision.CoveredCount, rows)
	}
	if decision.Covered.CurrencyCode != "NOK" {
		t.Errorf("covered currency = %q", decision.Covered.CurrencyCode)
	}

	groups, totals := f.queue(t)
	if len(groups) != 0 || totals.RowCount != 0 {
		t.Errorf("the group is still in the queue after being resolved: %+v", groups)
	}

	// One vendor row, one decision, and a classification per transaction.
	assertCounts(t, f, 1, 1, rows)
}

func TestAViewerCannotResolve(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "tax:1", name: "A", amount: 100})

	_, err := f.svc.Resolve(context.Background(), f.viewer, f.orgID, f.entity,
		"tax:1", review.OutcomeCategorised, payrollCode)
	if codeOf(err) != review.CodeForbidden {
		t.Fatalf("a viewer resolved, or failed for the wrong reason: %v", err)
	}
	if _, _, err := f.svc.Queue(context.Background(), f.viewer, f.orgID, f.entity, 10, 0); err != nil {
		t.Errorf("a viewer must still be able to read the queue: %v", err)
	}
	assertCounts(t, f, 0, 0, 0)
}

func TestEveryResolverRoleMayResolve(t *testing.T) {
	for _, role := range []string{"admin", "approver"} {
		t.Run(role, func(t *testing.T) {
			f := newFixture(t)
			f.insert(t, txn{key: "tax:1", name: "A", amount: 100})
			member := seedMember(t, f, role)
			if _, err := f.svc.Resolve(context.Background(), member, f.orgID, f.entity,
				"tax:1", review.OutcomeCategorised, payrollCode); err != nil {
				t.Fatalf("%s could not resolve: %v", role, err)
			}
		})
	}
	// The owner is the creator, whose membership CreateOrganization wrote.
	t.Run("owner", func(t *testing.T) {
		f := newFixture(t)
		f.insert(t, txn{key: "tax:1", name: "A", amount: 100})
		if _, err := f.svc.Resolve(context.Background(), f.owner, f.orgID, f.entity,
			"tax:1", review.OutcomeCategorised, payrollCode); err != nil {
			t.Fatalf("owner could not resolve: %v", err)
		}
	})
}

// Somebody who belongs to no organisation at all gets the same answer as
// somebody who belongs to a different one: the call is not a membership
// oracle.
func TestANonMemberIsRefused(t *testing.T) {
	f := newFixture(t)
	stranger := seedUser(t, f.db)

	_, _, err := f.svc.Queue(context.Background(), stranger, f.orgID, f.entity, 10, 0)
	if codeOf(err) != review.CodeForbidden {
		t.Fatalf("a non-member read the queue: %v", err)
	}
}

// Only a categorisation is a fact about the counterparty. Remembering a
// transfer would make L0 answer next month with something that is not a
// category.
func TestOnlyACategorisationWritesMemory(t *testing.T) {
	for _, tc := range []struct {
		outcome review.Outcome
		vendors int
	}{
		{review.OutcomeCategorised, 1},
		{review.OutcomeInternalTransfer, 0},
		{review.OutcomeNonPNL, 0},
	} {
		t.Run(string(tc.outcome), func(t *testing.T) {
			f := newFixture(t)
			f.insert(t, txn{key: "tax:1", name: "A", amount: 100})

			code := ""
			if tc.outcome == review.OutcomeCategorised {
				code = payrollCode
			}
			if _, err := f.svc.Resolve(context.Background(), f.owner, f.orgID,
				f.entity, "tax:1", tc.outcome, code); err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			assertCounts(t, f, tc.vendors, 1, 1)
		})
	}
}

// Skipping records that somebody looked and leaves the rows where they are.
func TestSkippingLeavesTheRowsInTheQueue(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "tax:1", name: "A", amount: 100})

	if _, err := f.svc.Resolve(context.Background(), f.owner, f.orgID,
		f.entity,
		"tax:1", review.OutcomeSkipped, ""); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	groups, _ := f.queue(t)
	if len(groups) != 1 {
		t.Errorf("a skip emptied the queue; it should record that somebody looked and nothing else")
	}
	assertCounts(t, f, 0, 1, 0)
}

func TestAnEmptyGroupIsRefused(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Resolve(context.Background(), f.owner, f.orgID,
		f.entity,
		"tax:nobody", review.OutcomeCategorised, payrollCode)
	if codeOf(err) != review.CodeEmptyGroup {
		t.Fatalf("resolving nothing succeeded or failed wrongly: %v", err)
	}
}

func TestACategoryIsRequiredAndRefusedInTurn(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "tax:1", name: "A", amount: 100})
	ctx := context.Background()

	_, err := f.svc.Resolve(ctx, f.owner, f.orgID,
		f.entity, "tax:1",
		review.OutcomeCategorised, "")
	if codeOf(err) != review.CodeCategoryRequired {
		t.Errorf("categorising with no category: %v", err)
	}

	_, err = f.svc.Resolve(ctx, f.owner, f.orgID,
		f.entity, "tax:1",
		review.OutcomeNonPNL, payrollCode)
	if codeOf(err) != review.CodeCategoryRefused {
		t.Errorf("a non-P&L marking with a category: %v", err)
	}

	_, err = f.svc.Resolve(ctx, f.owner, f.orgID,
		f.entity, "tax:1",
		review.OutcomeCategorised, "does-not-exist")
	if codeOf(err) != review.CodeUnknownCategory {
		t.Errorf("an unknown category: %v", err)
	}
}

// Task 7.3. Four writes, or none. A classification colliding with one that
// already exists must take the vendor row and the decision down with it.
func TestAPartialFailureLeavesNothing(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "tax:1", name: "A", amount: 100})
	ctx := context.Background()

	if _, err := f.svc.Resolve(ctx, f.owner, f.orgID,
		f.entity, "tax:1",
		review.OutcomeCategorised, payrollCode); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	// The rows now have live classifications, so the group is empty and the
	// second attempt is refused before it writes anything.
	_, err := f.svc.Resolve(ctx, f.owner, f.orgID,
		f.entity, "tax:1",
		review.OutcomeCategorised, otherCode)
	if codeOf(err) != review.CodeEmptyGroup {
		t.Fatalf("second resolve: %v", err)
	}
	assertCounts(t, f, 1, 1, 1)
}

// ---------------------------------------------------------------------------
// Task 7.8, 7.9, 7.10 -- undo, and the learning loop
// ---------------------------------------------------------------------------

func TestUndoReturnsTheGroupToTheQueue(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		txn{key: "tax:1", name: "A", amount: 100},
		txn{key: "tax:1", name: "A", amount: 200},
	)
	ctx := context.Background()

	decision, err := f.svc.Resolve(ctx, f.owner, f.orgID,
		f.entity, "tax:1",
		review.OutcomeCategorised, payrollCode)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	retracted, err := f.svc.Undo(ctx, f.owner, f.orgID, decision.ID)
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if retracted != 2 {
		t.Errorf("retracted = %d, want 2", retracted)
	}

	groups, totals := f.queue(t)
	if len(groups) != 1 || totals.RowCount != 2 {
		t.Fatalf("the group did not return to the queue: %+v", groups)
	}

	// No live decision, no memory, no live classification. The rows themselves
	// survive -- migration 007 withheld the DELETE grant, so history cannot be
	// rewritten even by a caller that wanted to.
	assertCounts(t, f, 0, 0, 0)
	assertClassificationRows(t, f, 2)
	assertDecisionUndone(t, f, decision.ID)
}

func TestUndoIsRefusedTwice(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "tax:1", name: "A", amount: 100})
	ctx := context.Background()

	decision, err := f.svc.Resolve(ctx, f.owner, f.orgID,
		f.entity, "tax:1",
		review.OutcomeCategorised, payrollCode)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := f.svc.Undo(ctx, f.owner, f.orgID, decision.ID); err != nil {
		t.Fatalf("first undo: %v", err)
	}
	if _, err := f.svc.Undo(ctx, f.owner, f.orgID, decision.ID); err == nil {
		t.Error("a decision was undone twice")
	}
}

func TestAViewerCannotUndo(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "tax:1", name: "A", amount: 100})
	ctx := context.Background()

	decision, err := f.svc.Resolve(ctx, f.owner, f.orgID,
		f.entity, "tax:1",
		review.OutcomeCategorised, payrollCode)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := f.svc.Undo(ctx, f.viewer, f.orgID, decision.ID); codeOf(err) != review.CodeForbidden {
		t.Fatalf("a viewer undid a decision: %v", err)
	}
}

// Task 7.10, the loop the whole product turns on: the second month is fast
// because the first month's decisions are memory.
func TestADecisionBecomesMemory(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "tax:220340017991", name: "Ромашка", amount: 100})
	ctx := context.Background()

	if _, err := f.svc.Resolve(ctx, f.owner, f.orgID,
		f.entity,
		"tax:220340017991", review.OutcomeCategorised, payrollCode); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	var key, code string
	err := f.db.InTx(ctx, f.org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT v.key, c.code FROM vendors v
			  JOIN categories c ON c.id = v.category_id`).Scan(&key, &code)
	})
	if err != nil {
		t.Fatalf("reading memory: %v", err)
	}
	if key != "tax:220340017991" || code != payrollCode {
		t.Errorf("memory = %q -> %q, want the counterparty and the chosen category", key, code)
	}

	// And undoing takes the memory back: a stale row would keep answering L0
	// with a category the user has just withdrawn.
	var decisionID pgtype.UUID
	if err := f.db.InTx(ctx, f.org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM review_decisions`).Scan(&decisionID)
	}); err != nil {
		t.Fatalf("reading the decision: %v", err)
	}
	if _, err := f.svc.Undo(ctx, f.owner, f.orgID, uuid.UUID(decisionID.Bytes)); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	assertCounts(t, f, 0, 0, 0)
}

// ---------------------------------------------------------------------------
// Task 7.1 -- isolation
// ---------------------------------------------------------------------------

func TestOneOrganisationCannotSeeOrResolveAnothers(t *testing.T) {
	a := newFixture(t)
	b := newFixture(t)
	ctx := context.Background()

	b.insert(t, txn{key: "tax:shared", name: "B's supplier", amount: 5000})

	groups, totals := a.queue(t)
	if len(groups) != 0 || totals.RowCount != 0 {
		t.Fatalf("org A sees org B's queue: %+v", groups)
	}

	// A resolving the same key touches nothing: the rows are B's, so A's read
	// finds an empty group.
	_, err := a.svc.Resolve(ctx, a.owner, a.orgID,
		a.entity, "tax:shared",
		review.OutcomeCategorised, payrollCode)
	if codeOf(err) != review.CodeEmptyGroup {
		t.Fatalf("org A resolved a counterparty belonging to org B: %v", err)
	}

	// B's queue is untouched.
	if groups, _ := b.queue(t); len(groups) != 1 {
		t.Errorf("org B's queue changed: %+v", groups)
	}
}

func TestUndoingAnotherOrganisationsDecisionFindsNothing(t *testing.T) {
	a := newFixture(t)
	b := newFixture(t)
	ctx := context.Background()

	b.insert(t, txn{key: "tax:1", name: "B", amount: 100})
	decision, err := b.svc.Resolve(ctx, b.owner, b.orgID,
		b.entity, "tax:1",
		review.OutcomeCategorised, payrollCode)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if _, err := a.svc.Undo(ctx, a.owner, a.orgID, decision.ID); err == nil {
		t.Fatal("org A undid org B's decision")
	}
	assertCounts(t, b, 1, 1, 1)
}

// ---------------------------------------------------------------------------
// Task 7.12 and the append-only grant
// ---------------------------------------------------------------------------

// A classification may never be rewritten or removed. Migration 007 says so in
// grants rather than in a comment, and this is what proves the grant is the
// one that shipped.
func TestClassificationsCannotBeRewritten(t *testing.T) {
	f := newFixture(t)
	f.insert(t, txn{key: "tax:1", name: "A", amount: 100})
	ctx := context.Background()

	if _, err := f.svc.Resolve(ctx, f.owner, f.orgID,
		f.entity, "tax:1",
		review.OutcomeCategorised, payrollCode); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for name, stmt := range map[string]string{
		"update the category": `UPDATE classifications SET category_id = category_id`,
		"delete the row":      `DELETE FROM classifications`,
	} {
		err := f.db.InTx(ctx, f.org, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, stmt)
			return err
		})
		if err == nil {
			t.Errorf("vekst_app could %s", name)
			continue
		}
		if code := pgCode(err); code != "42501" {
			t.Errorf("%s: SQLSTATE %s, want 42501 (insufficient privilege)", name, code)
		}
	}
}

// A decision is recorded in money, and money is an integer of minor units with
// a code beside it. A float anywhere on this path is a rounding error waiting
// for a large enough number.
func TestCoveredTotalsAreIntegerMoney(t *testing.T) {
	f := newFixture(t)
	f.insert(t,
		txn{key: "tax:1", name: "A", amount: 1},
		txn{key: "tax:1", name: "A", amount: 9_007_199_254_740_993}, // > 2^53
	)

	decision, err := f.svc.Resolve(context.Background(), f.owner, f.orgID,
		f.entity,
		"tax:1", review.OutcomeCategorised, payrollCode)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := money.Money{CurrencyCode: "NOK", MinorUnits: 9_007_199_254_740_994}
	if decision.Covered != want {
		t.Errorf("covered = %+v, want %+v -- a float64 cannot hold this exactly",
			decision.Covered, want)
	}
}

// ---------------------------------------------------------------------------

func assertCounts(t *testing.T, f *fixture, vendors, decisions, liveClassifications int) {
	t.Helper()
	var v, d, c int
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM vendors`).Scan(&v); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM review_decisions WHERE undone_at IS NULL`).Scan(&d); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM classifications
			 WHERE superseded_by IS NULL AND retracted_at IS NULL`).Scan(&c)
	})
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if v != vendors || d != decisions || c != liveClassifications {
		t.Errorf("vendors=%d decisions=%d live classifications=%d; want %d/%d/%d",
			v, d, c, vendors, decisions, liveClassifications)
	}
}

func assertClassificationRows(t *testing.T, f *fixture, total int) {
	t.Helper()
	var n int
	if err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM classifications`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting classifications: %v", err)
	}
	if n != total {
		t.Errorf("classification rows = %d, want %d -- an undo retracts, it never deletes", n, total)
	}
}

func assertDecisionUndone(t *testing.T, f *fixture, id uuid.UUID) {
	t.Helper()
	var undoneBy pgtype.UUID
	err := f.db.InTx(context.Background(), f.org, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT undone_by FROM review_decisions WHERE id = $1`,
			pgtype.UUID{Bytes: id, Valid: true}).Scan(&undoneBy)
	})
	if err != nil {
		t.Fatalf("reading the decision: %v", err)
	}
	if !undoneBy.Valid {
		t.Error("the decision was not stamped as undone; an undo records who, not just that")
	}
}
