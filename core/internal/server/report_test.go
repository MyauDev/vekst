package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	vektv1 "github.com/MyauDev/vekst/core/gen/vekst/v1"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/money"
	"github.com/MyauDev/vekst/core/internal/normalize"
	"github.com/MyauDev/vekst/core/internal/report"
	"github.com/MyauDev/vekst/core/internal/review"
)

// The translation layer, run for real.
//
// core/internal/report tests the arithmetic and the queries; this tests the
// part between them and a browser, which is where a field quietly stops being
// filled in. A handler is not interesting code and that is exactly why nobody
// notices when `line_no` stops reaching the wire, or when a percentage of no
// revenue starts arriving as a confident zero.
//
// Driven through the handler rather than over HTTP because the wire itself is
// Connect's to get right; what is this package's is what goes into the message.

const (
	reportTaxonomyV = "v1"
	reportRulesetV  = "v1"

	revenueLeaf = "0101"       // NET SALES
	opexLeaf    = "0401010204" // OPEX, Bank commission
)

type reportEnv struct {
	report *reportHandler
	review *reviewHandler
	db     *db.DB

	org    db.OrgID
	owner  uuid.UUID
	entity uuid.UUID
	batch  uuid.UUID
}

func testReportEnv(t *testing.T) *reportEnv {
	t.Helper()
	d := testDB(t)
	owner := testUser(t, d)
	org, entity := testOrgAndEntity(t, d, owner)

	e := &reportEnv{
		report: &reportHandler{svc: report.New(d)},
		review: &reviewHandler{svc: review.New(d,
			review.HumanVersions(reportTaxonomyV, reportRulesetV))},
		db:     d,
		org:    org,
		owner:  owner,
		entity: entity,
	}

	err := d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO accounts (org_id, entity_id, name, currency)
			VALUES (app_current_org(), $1, 'Main', 'NOK')`,
			pgtype.UUID{Bytes: entity, Valid: true}); err != nil {
			return fmt.Errorf("account: %w", err)
		}
		var b pgtype.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO import_batches (
				org_id, entity_id, source_kind, status, uploaded_by,
				file_name, declared_bytes, declared_type, file_key, upload_expires_at,
				file_sha256, byte_length, content_type)
			VALUES (app_current_org(), $1, 'bank', 'imported', $2,
				'march.csv', 1024, 'text/csv', 'uploads/' || gen_random_uuid(),
				now() + interval '1 hour',
				sha256(gen_random_uuid()::text::bytea), 1024, 'text/csv')
			RETURNING id`,
			pgtype.UUID{Bytes: entity, Valid: true},
			pgtype.UUID{Bytes: owner, Valid: true}).Scan(&b); err != nil {
			return fmt.Errorf("batch: %w", err)
		}
		e.batch = uuid.UUID(b.Bytes)
		return nil
	})
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	return e
}

// seed plants one transaction, classified into code when code is non-empty.
func (e *reportEnv) seed(t *testing.T, lineNo int32, amount int64, code string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := e.db.InTx(context.Background(), e.org, func(ctx context.Context, tx pgx.Tx) error {
		var got pgtype.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO transactions (
				org_id, entity_id, account_id, batch_id, line_no, source_kind,
				booked_on, amount_minor, currency,
				counterparty_raw, counterparty_key, description_raw,
				description_norm, normalize_version, dedup_hash)
			SELECT app_current_org(), $1, a.id, $2, $3, 'bank',
			       date '2026-03-10', $4, 'NOK',
			       'Acme AS', 'tax:acme', 'Payment', 'payment', $5, gen_random_uuid()::text
			  FROM accounts a WHERE a.entity_id = $1 LIMIT 1
			RETURNING id`,
			pgtype.UUID{Bytes: e.entity, Valid: true},
			pgtype.UUID{Bytes: e.batch, Valid: true},
			lineNo, amount, normalize.Version).Scan(&got); err != nil {
			return err
		}
		id = uuid.UUID(got.Bytes)
		if code == "" {
			return nil
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO classifications (
				org_id, transaction_id, category_id, engine_layer, confidence, evidence,
				taxonomy_version, ruleset_version, engine_version, normalize_version)
			SELECT app_current_org(), $1, c.id, 'L1', 0.950, 'rule:test', $2, $3, 'engine-1', $4
			  FROM categories c
			 WHERE c.taxonomy_version = $2 AND c.code = $5 AND c.org_id IS NULL`,
			got, reportTaxonomyV, reportRulesetV, normalize.Version, code)
		return err
	})
	if err != nil {
		t.Fatalf("seeding a transaction: %v", err)
	}
	return id
}

func (e *reportEnv) pnlRequest() *vektv1.GetManagementPNLRequest {
	return &vektv1.GetManagementPNLRequest{
		OrganizationId: e.org.UUID().String(),
		EntityId:       e.entity.String(),
		From:           "2026-03",
		To:             "2026-03",
		Granularity:    vektv1.ReportGranularity_REPORT_GRANULARITY_MONTH,
		Basis:          vektv1.ReportBasis_REPORT_BASIS_BANK,
	}
}

// ---------------------------------------------------------------------------
// GetManagementPNL
// ---------------------------------------------------------------------------

func TestGetManagementPNLHandler(t *testing.T) {
	e := testReportEnv(t)
	e.seed(t, 12, 1_000_00, revenueLeaf)
	e.seed(t, 13, -400_00, opexLeaf)
	e.seed(t, 14, -25_00, "")

	resp, err := e.report.GetManagementPNL(authedContext(t, e.owner),
		connect.NewRequest(e.pnlRequest()))
	if err != nil {
		t.Fatalf("GetManagementPNL: %v", err)
	}
	msg := resp.Msg

	if msg.GetBaseCurrency() != "NOK" {
		t.Errorf("base_currency = %q, want NOK -- every figure below it is meaningless "+
			"without it", msg.GetBaseCurrency())
	}
	if msg.GetBasis() != vektv1.ReportBasis_REPORT_BASIS_BANK {
		t.Errorf("basis = %v; it is a statement at the top of the table, not a footnote",
			msg.GetBasis())
	}
	if got := msg.GetPeriods(); len(got) != 1 || got[0] != "2026-03" {
		t.Errorf("periods = %v, want [2026-03]", got)
	}
	if msg.GetFrom() != "2026-03" || msg.GetTo() != "2026-03" {
		t.Errorf("range echoed back as %s..%s", msg.GetFrom(), msg.GetTo())
	}

	// Twelve lines in reading order, each computed line after its operands.
	if len(msg.GetLines()) != len(report.Order) {
		t.Fatalf("%d lines, want %d", len(msg.GetLines()), len(report.Order))
	}
	for i, l := range msg.GetLines() {
		if l.GetCode() != report.Order[i] {
			t.Errorf("line %d is %s, want %s", i, l.GetCode(), report.Order[i])
		}
		if l.GetLabel() == "" {
			t.Errorf("line %s has no label", l.GetCode())
		}
		if len(l.GetByPeriod()) != 1 {
			t.Errorf("line %s has %d columns, want 1", l.GetCode(), len(l.GetByPeriod()))
		}
		if l.GetTotal().GetAmount().GetCurrencyCode() != "NOK" {
			t.Errorf("line %s total carries no currency", l.GetCode())
		}
		if l.GetComputed() != (l.GetFormula() != "") {
			t.Errorf("line %s: computed=%v formula=%q", l.GetCode(), l.GetComputed(), l.GetFormula())
		}
	}

	byCode := map[string]*vektv1.ReportLine{}
	for _, l := range msg.GetLines() {
		byCode[l.GetCode()] = l
	}
	if got := byCode[report.NetSales].GetTotal().GetAmount().GetMinorUnits(); got != 1_000_00 {
		t.Errorf("NET SALES = %d, want 100000", got)
	}
	// Costs print positive: an owner reading "OPEX -400.00" beside
	// "NET SALES 1,000.00" is reading a spreadsheet, not a report.
	if got := byCode[report.OPEX].GetTotal().GetAmount().GetMinorUnits(); got != 400_00 {
		t.Errorf("OPEX = %d, want 40000 -- costs print positive", got)
	}
	// A percentage is present where there is revenue to take a share of.
	if byCode[report.OPEX].GetTotal().PercentOfRevenue == nil {
		t.Error("OPEX has no percent_of_revenue and the period has revenue")
	}

	// The four buckets, in the package's own order rather than a map's.
	if len(msg.GetBuckets()) != len(report.BucketOrder) {
		t.Fatalf("%d buckets, want %d", len(msg.GetBuckets()), len(report.BucketOrder))
	}
	if msg.GetBuckets()[0].GetKind() != vektv1.ReportBucketKind_REPORT_BUCKET_KIND_UNCLASSIFIED {
		t.Errorf("the first bucket is %v; unclassified comes first because it is the one "+
			"that decides whether the table above can be trusted", msg.GetBuckets()[0].GetKind())
	}
	if got := msg.GetBuckets()[0].GetTotal().GetMinorUnits(); got != -25_00 {
		t.Errorf("unclassified = %d, want -2500", got)
	}
	for _, b := range msg.GetBuckets() {
		if b.GetKind() == vektv1.ReportBucketKind_REPORT_BUCKET_KIND_UNSPECIFIED {
			t.Error("a bucket arrived unspecified")
		}
		if len(b.GetByPeriod()) != 1 {
			t.Errorf("bucket %v has %d columns, want 1", b.GetKind(), len(b.GetByPeriod()))
		}
	}

	// The versions are the rows', and they are what make a March report
	// reproduce in June.
	v := msg.GetVersions()
	if len(v.GetEngine()) != 1 || v.GetEngine()[0] != "engine-1" {
		t.Errorf("engine versions = %v, want [engine-1]", v.GetEngine())
	}
	if len(v.GetTaxonomy()) != 1 || len(v.GetNormalize()) != 1 {
		t.Errorf("versions = %+v", v)
	}

	// The strip at the foot of the table.
	if len(msg.GetReconciliation()) != 1 {
		t.Fatalf("%d reconciliation lines, want 1", len(msg.GetReconciliation()))
	}
	strip := msg.GetReconciliation()[0]
	if !strip.GetBalances() {
		t.Errorf("the strip does not balance: %d + %d - %d - %d <> %d",
			strip.GetOpening().GetMinorUnits(), strip.GetIn().GetMinorUnits(),
			strip.GetOut().GetMinorUnits(), strip.GetTransfers().GetMinorUnits(),
			strip.GetClosing().GetMinorUnits())
	}
	if !strip.GetDerived() {
		t.Error("the strip does not say its balances are derived; nothing stores what a " +
			"statement declared yet, and a client would render this as reconciled with a bank")
	}
	if strip.GetIn().GetMinorUnits() != 1_000_00 || strip.GetOut().GetMinorUnits() != 425_00 {
		t.Errorf("in/out = %d/%d, want 100000/42500 as positive magnitudes",
			strip.GetIn().GetMinorUnits(), strip.GetOut().GetMinorUnits())
	}
}

func TestGetManagementPNLRefusals(t *testing.T) {
	e := testReportEnv(t)
	ctx := authedContext(t, e.owner)

	t.Run("unauthenticated", func(t *testing.T) {
		_, err := e.report.GetManagementPNL(context.Background(),
			connect.NewRequest(e.pnlRequest()))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("unset basis", func(t *testing.T) {
		req := e.pnlRequest()
		req.Basis = vektv1.ReportBasis_REPORT_BASIS_UNSPECIFIED
		_, err := e.report.GetManagementPNL(ctx, connect.NewRequest(req))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
		// Proto3 cannot tell an unset enum from its zero value, and defaulting
		// this one would pick which half of the business the owner is looking
		// at, silently.
		if err.Error() != "invalid_argument: "+report.CodeUnknownBasis {
			t.Errorf("error = %q, want the %s code", err, report.CodeUnknownBasis)
		}
	})

	t.Run("unset granularity", func(t *testing.T) {
		req := e.pnlRequest()
		req.Granularity = vektv1.ReportGranularity_REPORT_GRANULARITY_UNSPECIFIED
		_, err := e.report.GetManagementPNL(ctx, connect.NewRequest(req))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("unparseable ids", func(t *testing.T) {
		for _, mutate := range []func(*vektv1.GetManagementPNLRequest){
			func(r *vektv1.GetManagementPNLRequest) { r.OrganizationId = "not-a-uuid" },
			func(r *vektv1.GetManagementPNLRequest) { r.EntityId = "not-a-uuid" },
		} {
			req := e.pnlRequest()
			mutate(req)
			_, err := e.report.GetManagementPNL(ctx, connect.NewRequest(req))
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
			}
		}
	})

	t.Run("a range running backwards", func(t *testing.T) {
		req := e.pnlRequest()
		req.From, req.To = "2026-05", "2026-01"
		_, err := e.report.GetManagementPNL(ctx, connect.NewRequest(req))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("a non-member", func(t *testing.T) {
		stranger := testUser(t, e.db)
		_, err := e.report.GetManagementPNL(authedContext(t, stranger),
			connect.NewRequest(e.pnlRequest()))
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("code = %v, want PermissionDenied", connect.CodeOf(err))
		}
	})
}

// A quarterly report, because the granularity switch has three arms and two of
// them would otherwise never run.
func TestGetManagementPNLGranularities(t *testing.T) {
	e := testReportEnv(t)
	e.seed(t, 1, 500_00, revenueLeaf)

	for _, c := range []struct {
		g    vektv1.ReportGranularity
		want string
	}{
		{vektv1.ReportGranularity_REPORT_GRANULARITY_MONTH, "2026-03"},
		{vektv1.ReportGranularity_REPORT_GRANULARITY_QUARTER, "2026-Q1"},
		{vektv1.ReportGranularity_REPORT_GRANULARITY_YEAR, "2026"},
	} {
		req := e.pnlRequest()
		req.Granularity = c.g
		resp, err := e.report.GetManagementPNL(authedContext(t, e.owner), connect.NewRequest(req))
		if err != nil {
			t.Fatalf("%v: %v", c.g, err)
		}
		if got := resp.Msg.GetPeriods(); len(got) != 1 || got[0] != c.want {
			t.Errorf("%v: periods = %v, want [%s]", c.g, got, c.want)
		}
		if got := resp.Msg.GetReconciliation(); len(got) != 1 || got[0].GetPeriod() != c.want {
			t.Errorf("%v: the strip does not follow the columns: %v", c.g, got)
		}
	}
}

// ---------------------------------------------------------------------------
// ListLineTransactions
// ---------------------------------------------------------------------------

func (e *reportEnv) drillRequest(line string) *vektv1.ListLineTransactionsRequest {
	return &vektv1.ListLineTransactionsRequest{
		OrganizationId: e.org.UUID().String(),
		EntityId:       e.entity.String(),
		Basis:          vektv1.ReportBasis_REPORT_BASIS_BANK,
		Granularity:    vektv1.ReportGranularity_REPORT_GRANULARITY_MONTH,
		From:           "2026-03",
		To:             "2026-03",
		Period:         "2026-03",
		Line:           line,
	}
}

func TestListLineTransactionsHandler(t *testing.T) {
	e := testReportEnv(t)
	e.seed(t, 12, 1_000_00, revenueLeaf)
	e.seed(t, 13, 250_00, revenueLeaf)
	e.seed(t, 14, -25_00, "")
	ctx := authedContext(t, e.owner)

	resp, err := e.report.ListLineTransactions(ctx,
		connect.NewRequest(e.drillRequest(report.NetSales)))
	if err != nil {
		t.Fatalf("ListLineTransactions: %v", err)
	}
	msg := resp.Msg

	if msg.GetKind() != vektv1.ReportAnswerKind_REPORT_ANSWER_KIND_TRANSACTIONS {
		t.Errorf("kind = %v, want transactions", msg.GetKind())
	}
	if msg.GetRowCount() != 2 || len(msg.GetTransactions()) != 2 {
		t.Fatalf("row_count %d over %d transactions, want 2",
			msg.GetRowCount(), len(msg.GetTransactions()))
	}
	if msg.GetTotal().GetMinorUnits() != 1_250_00 {
		t.Errorf("total = %d, want 125000", msg.GetTotal().GetMinorUnits())
	}
	if msg.GetNextCursor() != "" {
		t.Errorf("next_cursor = %q on a page that is the whole answer", msg.GetNextCursor())
	}

	for _, r := range msg.GetTransactions() {
		if r.GetLineNo() <= 0 {
			t.Errorf("row %s claims line %d; the batch says which file and the line says "+
				"where in it", r.GetId(), r.GetLineNo())
		}
		if r.GetBatchId() != e.batch.String() {
			t.Errorf("row %s names batch %s, want %s", r.GetId(), r.GetBatchId(), e.batch)
		}
		if r.GetBookedOn() != "2026-03-10" {
			t.Errorf("booked_on = %q", r.GetBookedOn())
		}
		if r.GetAmount().GetCurrencyCode() != "NOK" {
			t.Errorf("row %s carries an amount with no currency", r.GetId())
		}
		// Already in the base currency: "no conversion happened" is a
		// different claim from "converted at a rate of one".
		if r.GetBaseAmount() != nil {
			t.Errorf("a NOK row claims a conversion to %v", r.GetBaseAmount())
		}
		if r.GetEngineLayer() != "L1" || r.GetEvidence() != "rule:test" {
			t.Errorf("row %s says layer %q evidence %q -- these are what answer 'why is "+
				"this row on this line'", r.GetId(), r.GetEngineLayer(), r.GetEvidence())
		}
		if r.Confidence == nil {
			t.Errorf("row %s carries no confidence", r.GetId())
		}
		if r.GetDecidedBy() != "" {
			t.Errorf("a rule named a decider (%s); a machine decision has none", r.GetDecidedBy())
		}
		if r.GetCategoryCode() != revenueLeaf || r.GetCategoryName() == "" {
			t.Errorf("row %s: category %q/%q", r.GetId(), r.GetCategoryCode(), r.GetCategoryName())
		}
		if r.GetSourceKind() != "bank" {
			t.Errorf("row %s is of kind %q", r.GetId(), r.GetSourceKind())
		}
	}

	t.Run("a bucket opens too", func(t *testing.T) {
		resp, err := e.report.ListLineTransactions(ctx,
			connect.NewRequest(e.drillRequest(string(report.BucketUnclassified))))
		if err != nil {
			t.Fatalf("opening the unclassified bucket: %v", err)
		}
		if len(resp.Msg.GetTransactions()) != 1 {
			t.Fatalf("%d rows, want 1", len(resp.Msg.GetTransactions()))
		}
		r := resp.Msg.GetTransactions()[0]
		if r.Confidence != nil || r.GetEngineLayer() != "" || r.GetCategoryCode() != "" {
			t.Errorf("an unclassified row claims layer %q category %q confidence %v",
				r.GetEngineLayer(), r.GetCategoryCode(), r.GetConfidence())
		}
	})

	t.Run("a computed line answers with operands", func(t *testing.T) {
		resp, err := e.report.ListLineTransactions(ctx,
			connect.NewRequest(e.drillRequest(report.GM)))
		if err != nil {
			t.Fatalf("opening GM: %v", err)
		}
		if resp.Msg.GetKind() != vektv1.ReportAnswerKind_REPORT_ANSWER_KIND_OPERANDS {
			t.Fatalf("kind = %v, want operands", resp.Msg.GetKind())
		}
		if len(resp.Msg.GetTransactions()) != 0 {
			t.Error("GM returned transactions; it has none of its own")
		}
		ops := resp.Msg.GetOperands()
		if len(ops) != 2 {
			t.Fatalf("%d operands, want 2", len(ops))
		}
		if ops[0].GetCode() != report.NetSales || ops[0].GetSubtracted() {
			t.Errorf("first operand = %+v, want NET SALES added", ops[0])
		}
		if ops[1].GetCode() != report.CS || !ops[1].GetSubtracted() {
			t.Errorf("second operand = %+v, want CS subtracted", ops[1])
		}
		if ops[0].GetLabel() == "" || ops[1].GetLabel() == "" {
			t.Error("an operand with no label is a code a person has to look up")
		}
	})

	t.Run("paging", func(t *testing.T) {
		req := e.drillRequest(report.NetSales)
		req.Limit = 1
		first, err := e.report.ListLineTransactions(ctx, connect.NewRequest(req))
		if err != nil {
			t.Fatalf("first page: %v", err)
		}
		if len(first.Msg.GetTransactions()) != 1 || first.Msg.GetNextCursor() == "" {
			t.Fatalf("first page has %d rows and cursor %q",
				len(first.Msg.GetTransactions()), first.Msg.GetNextCursor())
		}
		req.Cursor = first.Msg.GetNextCursor()
		second, err := e.report.ListLineTransactions(ctx, connect.NewRequest(req))
		if err != nil {
			t.Fatalf("second page: %v", err)
		}
		if len(second.Msg.GetTransactions()) != 1 {
			t.Fatalf("second page has %d rows", len(second.Msg.GetTransactions()))
		}
		if second.Msg.GetTransactions()[0].GetId() == first.Msg.GetTransactions()[0].GetId() {
			t.Error("the cursor returned the same row twice")
		}
		if second.Msg.GetNextCursor() != "" {
			t.Error("the last page claims another")
		}
	})
}

func TestListLineTransactionsRefusals(t *testing.T) {
	e := testReportEnv(t)
	ctx := authedContext(t, e.owner)

	for name, mutate := range map[string]func(*vektv1.ListLineTransactionsRequest){
		"unparseable entity": func(r *vektv1.ListLineTransactionsRequest) { r.EntityId = "x" },
		"unset basis": func(r *vektv1.ListLineTransactionsRequest) {
			r.Basis = vektv1.ReportBasis_REPORT_BASIS_UNSPECIFIED
		},
		"unset granularity": func(r *vektv1.ListLineTransactionsRequest) {
			r.Granularity = vektv1.ReportGranularity_REPORT_GRANULARITY_UNSPECIFIED
		},
		// A line this report has not got is a bad request, never an empty
		// page: an empty drill-down of a figure of 412,000 reads as "these
		// rows went missing".
		"a line that is not a line": func(r *vektv1.ListLineTransactionsRequest) { r.Line = "99" },
		"a category name as a line": func(r *vektv1.ListLineTransactionsRequest) {
			r.Line = "NET SALES"
		},
		"a period outside the report": func(r *vektv1.ListLineTransactionsRequest) {
			r.Period = "2026-04"
		},
		"a cursor that is not one": func(r *vektv1.ListLineTransactionsRequest) {
			r.Cursor = "yesterday"
		},
		"a cursor with an unparseable id": func(r *vektv1.ListLineTransactionsRequest) {
			r.Cursor = "2026-03-10:not-a-uuid"
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := e.drillRequest(report.NetSales)
			mutate(req)
			_, err := e.report.ListLineTransactions(ctx, connect.NewRequest(req))
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("code = %v, want InvalidArgument (err %v)", connect.CodeOf(err), err)
			}
		})
	}

	t.Run("unauthenticated", func(t *testing.T) {
		_, err := e.report.ListLineTransactions(context.Background(),
			connect.NewRequest(e.drillRequest(report.NetSales)))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})
}

// ---------------------------------------------------------------------------
// The review handlers, which are this change's other untested translation
// ---------------------------------------------------------------------------

func TestReviewHandlers(t *testing.T) {
	e := testReportEnv(t)
	e.seed(t, 20, -300_00, "")
	e.seed(t, 21, -200_00, "")
	ctx := authedContext(t, e.owner)

	groups, err := e.review.ListReviewGroups(ctx, connect.NewRequest(&vektv1.ListReviewGroupsRequest{
		OrganizationId: e.org.UUID().String(), EntityId: e.entity.String(),
	}))
	if err != nil {
		t.Fatalf("ListReviewGroups: %v", err)
	}
	if len(groups.Msg.GetGroups()) != 1 {
		t.Fatalf("%d groups, want 1", len(groups.Msg.GetGroups()))
	}
	g := groups.Msg.GetGroups()[0]
	if g.GetCounterpartyKey() != "tax:acme" || g.GetRowCount() != 2 {
		t.Errorf("group = %+v", g)
	}
	if g.GetTotal().GetMinorUnits() != -500_00 {
		t.Errorf("group total = %d, want -50000", g.GetTotal().GetMinorUnits())
	}
	if groups.Msg.GetTotalAbsolute().GetMinorUnits() != 500_00 {
		t.Errorf("absolute total = %d, want 50000: an expense and an income of the same "+
			"size must not net to an empty queue",
			groups.Msg.GetTotalAbsolute().GetMinorUnits())
	}

	rows, err := e.review.ListGroupTransactions(ctx,
		connect.NewRequest(&vektv1.ListGroupTransactionsRequest{
			OrganizationId: e.org.UUID().String(), EntityId: e.entity.String(),
			CounterpartyKey: "tax:acme",
		}))
	if err != nil {
		t.Fatalf("ListGroupTransactions: %v", err)
	}
	if len(rows.Msg.GetTransactions()) != 2 {
		t.Fatalf("%d rows, want 2", len(rows.Msg.GetTransactions()))
	}
	if rows.Msg.GetTransactions()[0].GetDirection() != "expense" {
		t.Errorf("direction = %q, want expense -- it is generated from the sign",
			rows.Msg.GetTransactions()[0].GetDirection())
	}

	resolved, err := e.review.ResolveGroup(ctx, connect.NewRequest(&vektv1.ResolveGroupRequest{
		OrganizationId: e.org.UUID().String(), EntityId: e.entity.String(),
		CounterpartyKey: "tax:acme",
		Outcome:         vektv1.ReviewOutcome_REVIEW_OUTCOME_CATEGORISED,
		CategoryCode:    opexLeaf,
	}))
	if err != nil {
		t.Fatalf("ResolveGroup: %v", err)
	}
	if resolved.Msg.GetCoveredCount() != 2 {
		t.Errorf("covered %d rows, want 2", resolved.Msg.GetCoveredCount())
	}

	// And the decision reaches the report, which is the seam the two changes
	// share.
	pnl, err := e.report.GetManagementPNL(ctx, connect.NewRequest(e.pnlRequest()))
	if err != nil {
		t.Fatalf("GetManagementPNL: %v", err)
	}
	for _, l := range pnl.Msg.GetLines() {
		if l.GetCode() == report.OPEX && l.GetTotal().GetAmount().GetMinorUnits() != 500_00 {
			t.Errorf("OPEX = %d after the decision, want 50000",
				l.GetTotal().GetAmount().GetMinorUnits())
		}
	}

	undone, err := e.review.UndoDecision(ctx, connect.NewRequest(&vektv1.UndoDecisionRequest{
		OrganizationId: e.org.UUID().String(), DecisionId: resolved.Msg.GetDecisionId(),
	}))
	if err != nil {
		t.Fatalf("UndoDecision: %v", err)
	}
	if undone.Msg.GetRetractedCount() != 2 {
		t.Errorf("retracted %d, want 2", undone.Msg.GetRetractedCount())
	}
}

func TestReviewHandlerRefusals(t *testing.T) {
	e := testReportEnv(t)
	e.seed(t, 30, -100_00, "")
	ctx := authedContext(t, e.owner)
	orgID := e.org.UUID().String()

	t.Run("an unset outcome", func(t *testing.T) {
		_, err := e.review.ResolveGroup(ctx, connect.NewRequest(&vektv1.ResolveGroupRequest{
			OrganizationId: orgID, EntityId: e.entity.String(),
			CounterpartyKey: "tax:acme",
			Outcome:         vektv1.ReviewOutcome_REVIEW_OUTCOME_UNSPECIFIED,
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("a viewer may not resolve", func(t *testing.T) {
		viewer := testUser(t, e.db)
		err := e.db.InTx(context.Background(), e.org, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO memberships (org_id, user_id, role)
				VALUES (app_current_org(), $1, 'viewer')`,
				pgtype.UUID{Bytes: viewer, Valid: true})
			return err
		})
		if err != nil {
			t.Fatalf("seeding a viewer: %v", err)
		}

		// Reading is the whole reason the role exists.
		if _, err := e.review.ListReviewGroups(authedContext(t, viewer),
			connect.NewRequest(&vektv1.ListReviewGroupsRequest{
				OrganizationId: orgID, EntityId: e.entity.String(),
			})); err != nil {
			t.Errorf("a viewer could not read the queue: %v", err)
		}
		if _, err := e.report.GetManagementPNL(authedContext(t, viewer),
			connect.NewRequest(e.pnlRequest())); err != nil {
			t.Errorf("a viewer could not read the report: %v", err)
		}

		_, err = e.review.ResolveGroup(authedContext(t, viewer),
			connect.NewRequest(&vektv1.ResolveGroupRequest{
				OrganizationId: orgID, EntityId: e.entity.String(),
				CounterpartyKey: "tax:acme",
				Outcome:         vektv1.ReviewOutcome_REVIEW_OUTCOME_CATEGORISED,
				CategoryCode:    opexLeaf,
			}))
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("code = %v, want PermissionDenied", connect.CodeOf(err))
		}
	})

	t.Run("an empty group", func(t *testing.T) {
		_, err := e.review.ResolveGroup(ctx, connect.NewRequest(&vektv1.ResolveGroupRequest{
			OrganizationId: orgID, EntityId: e.entity.String(),
			CounterpartyKey: "tax:nobody",
			Outcome:         vektv1.ReviewOutcome_REVIEW_OUTCOME_SKIPPED,
		}))
		// Not an empty success: a decision covering nothing is a double submit
		// or somebody else's counterparty, and both deserve to be visible.
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
		}
	})

	t.Run("unparseable ids", func(t *testing.T) {
		_, err := e.review.UndoDecision(ctx, connect.NewRequest(&vektv1.UndoDecisionRequest{
			OrganizationId: orgID, DecisionId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
		_, err = e.review.ListGroupTransactions(ctx,
			connect.NewRequest(&vektv1.ListGroupTransactionsRequest{
				OrganizationId: orgID, EntityId: "not-a-uuid",
			}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("unauthenticated", func(t *testing.T) {
		for name, call := range map[string]func() error{
			"list": func() error {
				_, err := e.review.ListReviewGroups(context.Background(),
					connect.NewRequest(&vektv1.ListReviewGroupsRequest{OrganizationId: orgID}))
				return err
			},
			"rows": func() error {
				_, err := e.review.ListGroupTransactions(context.Background(),
					connect.NewRequest(&vektv1.ListGroupTransactionsRequest{OrganizationId: orgID}))
				return err
			},
			"resolve": func() error {
				_, err := e.review.ResolveGroup(context.Background(),
					connect.NewRequest(&vektv1.ResolveGroupRequest{OrganizationId: orgID}))
				return err
			},
			"undo": func() error {
				_, err := e.review.UndoDecision(context.Background(),
					connect.NewRequest(&vektv1.UndoDecisionRequest{OrganizationId: orgID}))
				return err
			},
		} {
			if got := connect.CodeOf(call()); got != connect.CodeUnauthenticated {
				t.Errorf("%s: code = %v, want Unauthenticated", name, got)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// The translation itself, where a live database says nothing useful
// ---------------------------------------------------------------------------

// An error the service did not give a code to must not reach a client as one.
// A coded failure is something a screen can say out loud; anything else is a
// bug, and dressing it as InvalidArgument would tell the user to fix their
// request.
func TestAnUncodedFailureIsInternal(t *testing.T) {
	boom := errors.New("the connection went away")
	if got := connect.CodeOf(reportErr(boom)); got != connect.CodeInternal {
		t.Errorf("reportErr: code = %v, want Internal", got)
	}
	if got := connect.CodeOf(connectErr(boom)); got != connect.CodeInternal {
		t.Errorf("connectErr: code = %v, want Internal", got)
	}
}

// Each coded failure reaches a client as the Connect code that says what the
// caller can do about it: fix the request, ask somebody for access, or wait.
// Collapsing them onto one code would make every one of them read as the
// caller's fault.
func TestEveryCodedFailureKeepsItsMeaning(t *testing.T) {
	for _, c := range []struct {
		code string
		want connect.Code
	}{
		{report.CodeForbidden, connect.CodePermissionDenied},
		// An organisation with no reporting currency is a state somebody has
		// to fix, not a request somebody got wrong.
		{report.CodeNoBaseCurrency, connect.CodeFailedPrecondition},
		{report.CodeBadRange, connect.CodeInvalidArgument},
		{report.CodeUnknownLine, connect.CodeInvalidArgument},
		{report.CodeUnknownPeriod, connect.CodeInvalidArgument},
	} {
		err := reportErr(&report.Err{Code: c.code})
		if got := connect.CodeOf(err); got != c.want {
			t.Errorf("%s -> %v, want %v", c.code, got, c.want)
		}
		// The code is the message: translation is the client's.
		if err.Error() != c.want.String()+": "+c.code {
			t.Errorf("%s reached the wire as %q", c.code, err)
		}
	}

	for _, c := range []struct {
		code string
		want connect.Code
	}{
		{review.CodeForbidden, connect.CodePermissionDenied},
		{review.CodeEmptyGroup, connect.CodeFailedPrecondition},
		{review.CodeAlreadyUndone, connect.CodeFailedPrecondition},
		// Two people working the queue at once is the normal case, not a
		// malformed request and not an internal error.
		{review.CodeAlreadyDecided, connect.CodeFailedPrecondition},
		{review.CodeCategoryRequired, connect.CodeInvalidArgument},
	} {
		if got := connect.CodeOf(connectErr(&review.Err{Code: c.code})); got != c.want {
			t.Errorf("%s -> %v, want %v", c.code, got, c.want)
		}
	}
}

// Both halves of the business cross the wire. A basis that translated to the
// wrong half would produce a plausible report of the wrong thing.
func TestBothBasesCrossTheWire(t *testing.T) {
	for proto, want := range map[vektv1.ReportBasis]report.Basis{
		vektv1.ReportBasis_REPORT_BASIS_LEDGER: report.BasisLedger,
		vektv1.ReportBasis_REPORT_BASIS_BANK:   report.BasisBank,
	} {
		got, err := fromProtoBasis(proto)
		if err != nil || got != want {
			t.Errorf("%v translated to %q (%v), want %q", proto, got, err, want)
		}
	}
	if _, err := fromProtoBasis(vektv1.ReportBasis_REPORT_BASIS_UNSPECIFIED); err == nil {
		t.Error("an unset basis was accepted")
	}
}

// A figure of zero is a fact -- "this section had no movement" -- and must
// arrive as a zero with a currency beside it, not as an absent field a client
// renders as a dash.
func TestAZeroFigureIsAZeroAndAnAbsentAmountIsAbsent(t *testing.T) {
	zero := toFigure(report.Figure{Amount: money.Money{CurrencyCode: "NOK"}})
	if zero.GetAmount() == nil || zero.GetAmount().GetCurrencyCode() != "NOK" {
		t.Errorf("a zero figure arrived as %v", zero)
	}
	if zero.PercentOfRevenue != nil {
		t.Error("a figure with no revenue behind it reported a percentage; neither zero " +
			"nor infinity is the answer, and a client would render both as a fact")
	}

	withPct := toFigure(report.Figure{
		Amount:              money.Money{CurrencyCode: "NOK", MinorUnits: 25},
		PercentOfRevenue:    12.5,
		HasPercentOfRevenue: true,
	})
	if withPct.PercentOfRevenue == nil ||
		math.Abs(withPct.GetPercentOfRevenue()-12.5) > 1e-9 {
		t.Errorf("percent_of_revenue = %v", withPct.GetPercentOfRevenue())
	}

	if got := toReportMoney(money.Money{CurrencyCode: "NOK"}); got.GetCurrencyCode() != "NOK" {
		t.Errorf("toReportMoney dropped the currency of a zero: %v", got)
	}
	// toMoney is the other one, and it elides deliberately: a row that needed
	// no conversion has no converted amount.
	if got := toMoney(money.Money{}); got != nil {
		t.Errorf("toMoney(zero) = %v, want nil", got)
	}
	if got := toMoney(money.Money{CurrencyCode: "JPY", MinorUnits: 700}); got == nil ||
		got.GetMinorUnits() != 700 {
		t.Errorf("toMoney dropped a real amount: %v", got)
	}
}

// Every bucket the report computes has a name on the wire. One falling through
// to UNSPECIFIED would reach a client as a total it cannot label, which is a
// figure nobody can act on.
func TestEveryBucketAndAnswerHasAWireName(t *testing.T) {
	for _, b := range report.BucketOrder {
		if toProtoBucket(b) == vektv1.ReportBucketKind_REPORT_BUCKET_KIND_UNSPECIFIED {
			t.Errorf("bucket %s has no wire name", b)
		}
	}
	if toProtoBucket(report.Bucket("invented")) !=
		vektv1.ReportBucketKind_REPORT_BUCKET_KIND_UNSPECIFIED {
		t.Error("an unknown bucket was given a name it does not have")
	}
	if toProtoAnswer(report.AnswerOperands) !=
		vektv1.ReportAnswerKind_REPORT_ANSWER_KIND_OPERANDS {
		t.Error("operands did not survive the translation")
	}
	if toProtoAnswer(report.AnswerTransactions) !=
		vektv1.ReportAnswerKind_REPORT_ANSWER_KIND_TRANSACTIONS {
		t.Error("transactions did not survive the translation")
	}
}

// The outcome vocabulary, both ways. Proto3 cannot tell an unset enum from its
// zero value, so the zero is refused rather than defaulted -- defaulting it
// would pick what a person decided about a counterparty on their behalf.
func TestEveryOutcomeCrossesTheWire(t *testing.T) {
	for proto, want := range map[vektv1.ReviewOutcome]review.Outcome{
		vektv1.ReviewOutcome_REVIEW_OUTCOME_CATEGORISED:       review.OutcomeCategorised,
		vektv1.ReviewOutcome_REVIEW_OUTCOME_INTERNAL_TRANSFER: review.OutcomeInternalTransfer,
		vektv1.ReviewOutcome_REVIEW_OUTCOME_NON_PNL:           review.OutcomeNonPNL,
		vektv1.ReviewOutcome_REVIEW_OUTCOME_SKIPPED:           review.OutcomeSkipped,
	} {
		got, err := fromProtoOutcome(proto)
		if err != nil || got != want {
			t.Errorf("%v translated to %q (%v), want %q", proto, got, err, want)
		}
	}
	if _, err := fromProtoOutcome(vektv1.ReviewOutcome_REVIEW_OUTCOME_UNSPECIFIED); err == nil {
		t.Error("an unset outcome was accepted")
	}
}
