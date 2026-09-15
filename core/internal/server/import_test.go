package server

import (
	"context"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gendb "github.com/MyauDev/vekst/core/gen/db"
	vektv1 "github.com/MyauDev/vekst/core/gen/vekst/v1"
	"github.com/MyauDev/vekst/core/internal/blob"
	"github.com/MyauDev/vekst/core/internal/db"
	"github.com/MyauDev/vekst/core/internal/identity"
	"github.com/MyauDev/vekst/core/internal/ingest"
	"github.com/MyauDev/vekst/core/internal/jobs"
)

// The handler-level test harness below mirrors core/internal/ingest's own
// (testEnv, testUser, testOrgAndEntity): Go does not let a _test.go file in
// one package reuse another's unexported helpers, and this package is the
// one place vekst.v1.ImportService's translation from proto to
// *ingest.Service actually runs -- untested since add-file-upload, because
// nothing before this needed an authenticated context outside a real
// browser request.

func testDB(t *testing.T) *db.DB {
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
	return d
}

func testStore(t *testing.T) blob.ObjectStore {
	t.Helper()
	endpoint := os.Getenv("OBJECT_STORE_ENDPOINT")
	if endpoint == "" {
		t.Skip("OBJECT_STORE_ENDPOINT not set; skipping a test that needs a live object store")
	}
	bucket := os.Getenv("OBJECT_STORE_BUCKET")
	if bucket == "" {
		bucket = "vekst"
	}
	return blob.New(blob.Config{
		Endpoint:    endpoint,
		Bucket:      bucket,
		Region:      "us-east-1",
		AccessKeyID: envOr("OBJECT_STORE_ACCESS_KEY_ID", "minioadmin"),
		SecretKey:   envOr("OBJECT_STORE_SECRET_KEY", "minioadmin"),
		PathStyle:   true,
	})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func testUser(t *testing.T, d *db.DB) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := d.InSystemTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO users (email) VALUES ($1) RETURNING id`,
			uuid.NewString()+"@server.test").Scan(&id)
	})
	if err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return uuid.UUID(id.Bytes)
}

func testOrgAndEntity(t *testing.T, d *db.DB, owner uuid.UUID) (db.OrgID, uuid.UUID) {
	t.Helper()
	org, err := d.CreateOrganization(context.Background(), db.NewOrganization{
		Name:         "Test " + uuid.NewString()[:8],
		Country:      "NO",
		BaseCurrency: "NOK",
		EntityName:   "Test AS",
		CreatorID:    owner,
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	var entityID uuid.UUID
	err = d.InTx(context.Background(), org, func(ctx context.Context, tx pgx.Tx) error {
		entities, err := gendb.New(tx).ListEntities(ctx)
		if err != nil {
			return err
		}
		entityID = uuid.UUID(entities[0].ID.Bytes)
		return nil
	})
	if err != nil {
		t.Fatalf("reading the organisation's entity: %v", err)
	}
	return org, entityID
}

// testHandler builds a real *importHandler behind a real *ingest.Service,
// with its own River client started -- the same wiring core/cmd/vekst-core
// does, minus the HTTP mux.
func testHandler(t *testing.T) (*importHandler, *db.DB) {
	t.Helper()
	d := testDB(t)
	store := testStore(t)
	cfg := ingest.Config{UploadMaxBytes: 26_214_400, UploadURLLifetime: 15 * time.Minute}
	workers := ingest.NewWorkers(d, store, cfg)
	jobsClient, err := jobs.New(d, workers)
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	if err := jobsClient.Start(context.Background()); err != nil {
		t.Fatalf("jobs Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = jobsClient.Stop(stopCtx)
	})
	return &importHandler{svc: ingest.NewService(d, store, jobsClient, cfg)}, d
}

// authedContext is one authenticated request context, for a real user.
func authedContext(t *testing.T, userID uuid.UUID) context.Context {
	t.Helper()
	return identity.ContextForTest(t, context.Background(), identity.User{ID: userID})
}

func TestCreateImportBatchHandler(t *testing.T) {
	h, d := testHandler(t)
	owner := testUser(t, d)
	org, entityID := testOrgAndEntity(t, d, owner)
	ctx := authedContext(t, owner)

	resp, err := h.CreateImportBatch(ctx, connect.NewRequest(&vektv1.CreateImportBatchRequest{
		OrgId: org.UUID().String(), EntityId: entityID.String(),
		SourceKind: vektv1.SourceKind_SOURCE_KIND_BANK, FileName: "x.csv",
		DeclaredBytes: 10, DeclaredType: "text/csv",
	}))
	if err != nil {
		t.Fatalf("CreateImportBatch: %v", err)
	}
	if resp.Msg.GetBatch().GetId() == "" {
		t.Error("no batch id in the response")
	}
	if resp.Msg.GetUploadUrl() == "" {
		t.Error("no upload url in the response")
	}
	batchID := resp.Msg.GetBatch().GetId()

	t.Run("unauthenticated", func(t *testing.T) {
		_, err := h.CreateImportBatch(context.Background(), connect.NewRequest(&vektv1.CreateImportBatchRequest{
			OrgId: org.UUID().String(), EntityId: entityID.String(),
			SourceKind: vektv1.SourceKind_SOURCE_KIND_BANK, FileName: "x.csv",
			DeclaredBytes: 10, DeclaredType: "text/csv",
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("invalid org_id", func(t *testing.T) {
		_, err := h.CreateImportBatch(ctx, connect.NewRequest(&vektv1.CreateImportBatchRequest{
			OrgId: "not-a-uuid", EntityId: entityID.String(),
			SourceKind: vektv1.SourceKind_SOURCE_KIND_BANK, FileName: "x.csv",
			DeclaredBytes: 10, DeclaredType: "text/csv",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("invalid entity_id", func(t *testing.T) {
		_, err := h.CreateImportBatch(ctx, connect.NewRequest(&vektv1.CreateImportBatchRequest{
			OrgId: org.UUID().String(), EntityId: "not-a-uuid",
			SourceKind: vektv1.SourceKind_SOURCE_KIND_BANK, FileName: "x.csv",
			DeclaredBytes: 10, DeclaredType: "text/csv",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("invalid import_profile_id", func(t *testing.T) {
		bad := "not-a-uuid"
		_, err := h.CreateImportBatch(ctx, connect.NewRequest(&vektv1.CreateImportBatchRequest{
			OrgId: org.UUID().String(), EntityId: entityID.String(),
			SourceKind: vektv1.SourceKind_SOURCE_KIND_BANK, FileName: "x.csv",
			DeclaredBytes: 10, DeclaredType: "text/csv", ImportProfileId: &bad,
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("service error translates to a code", func(t *testing.T) {
		_, err := h.CreateImportBatch(ctx, connect.NewRequest(&vektv1.CreateImportBatchRequest{
			OrgId: org.UUID().String(), EntityId: entityID.String(),
			SourceKind: vektv1.SourceKind_SOURCE_KIND_BANK, FileName: "",
			DeclaredBytes: 10, DeclaredType: "text/csv",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("ConfirmImportUpload, unauthenticated", func(t *testing.T) {
		_, err := h.ConfirmImportUpload(context.Background(), connect.NewRequest(&vektv1.ConfirmImportUploadRequest{
			OrgId: org.UUID().String(), BatchId: batchID,
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("ConfirmImportUpload, invalid batch_id", func(t *testing.T) {
		_, err := h.ConfirmImportUpload(ctx, connect.NewRequest(&vektv1.ConfirmImportUploadRequest{
			OrgId: org.UUID().String(), BatchId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("GetImportBatch", func(t *testing.T) {
		resp, err := h.GetImportBatch(ctx, connect.NewRequest(&vektv1.GetImportBatchRequest{
			OrgId: org.UUID().String(), BatchId: batchID,
		}))
		if err != nil {
			t.Fatalf("GetImportBatch: %v", err)
		}
		if resp.Msg.GetBatch().GetId() != batchID {
			t.Errorf("Id = %s, want %s", resp.Msg.GetBatch().GetId(), batchID)
		}
	})

	t.Run("GetImportBatch, not found", func(t *testing.T) {
		_, err := h.GetImportBatch(ctx, connect.NewRequest(&vektv1.GetImportBatchRequest{
			OrgId: org.UUID().String(), BatchId: uuid.NewString(),
		}))
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("GetImportBatch, invalid batch_id", func(t *testing.T) {
		_, err := h.GetImportBatch(ctx, connect.NewRequest(&vektv1.GetImportBatchRequest{
			OrgId: org.UUID().String(), BatchId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("ListImportBatches", func(t *testing.T) {
		resp, err := h.ListImportBatches(ctx, connect.NewRequest(&vektv1.ListImportBatchesRequest{
			OrgId: org.UUID().String(), EntityId: entityID.String(),
		}))
		if err != nil {
			t.Fatalf("ListImportBatches: %v", err)
		}
		if len(resp.Msg.GetBatches()) == 0 {
			t.Error("no batches listed")
		}
	})

	t.Run("ListImportBatches, unauthenticated", func(t *testing.T) {
		_, err := h.ListImportBatches(context.Background(), connect.NewRequest(&vektv1.ListImportBatchesRequest{
			OrgId: org.UUID().String(), EntityId: entityID.String(),
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("ListImportBatches, invalid entity_id", func(t *testing.T) {
		_, err := h.ListImportBatches(ctx, connect.NewRequest(&vektv1.ListImportBatchesRequest{
			OrgId: org.UUID().String(), EntityId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("GetValidationReport, not found", func(t *testing.T) {
		_, err := h.GetValidationReport(ctx, connect.NewRequest(&vektv1.GetValidationReportRequest{
			OrgId: org.UUID().String(), BatchId: batchID,
		}))
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("GetValidationReport, invalid batch_id", func(t *testing.T) {
		_, err := h.GetValidationReport(ctx, connect.NewRequest(&vektv1.GetValidationReportRequest{
			OrgId: org.UUID().String(), BatchId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("OverrideValidation, not found", func(t *testing.T) {
		_, err := h.OverrideValidation(ctx, connect.NewRequest(&vektv1.OverrideValidationRequest{
			OrgId: org.UUID().String(), BatchId: batchID, Reason: "a good reason here",
		}))
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("OverrideValidation, invalid batch_id", func(t *testing.T) {
		_, err := h.OverrideValidation(ctx, connect.NewRequest(&vektv1.OverrideValidationRequest{
			OrgId: org.UUID().String(), BatchId: "not-a-uuid", Reason: "a good reason here",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("GetDedupSummary", func(t *testing.T) {
		resp, err := h.GetDedupSummary(ctx, connect.NewRequest(&vektv1.GetDedupSummaryRequest{
			OrgId: org.UUID().String(), BatchId: batchID,
		}))
		if err != nil {
			t.Fatalf("GetDedupSummary: %v", err)
		}
		if resp.Msg.GetSummary() == nil {
			t.Error("no summary in the response")
		}
	})

	t.Run("GetDedupSummary, unauthenticated", func(t *testing.T) {
		_, err := h.GetDedupSummary(context.Background(), connect.NewRequest(&vektv1.GetDedupSummaryRequest{
			OrgId: org.UUID().String(), BatchId: batchID,
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("GetDedupSummary, invalid batch_id", func(t *testing.T) {
		_, err := h.GetDedupSummary(ctx, connect.NewRequest(&vektv1.GetDedupSummaryRequest{
			OrgId: org.UUID().String(), BatchId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("ListSkippedRows", func(t *testing.T) {
		resp, err := h.ListSkippedRows(ctx, connect.NewRequest(&vektv1.ListSkippedRowsRequest{
			OrgId: org.UUID().String(), BatchId: batchID,
		}))
		if err != nil {
			t.Fatalf("ListSkippedRows: %v", err)
		}
		if len(resp.Msg.GetRows()) != 0 {
			t.Errorf("got %d rows, want 0 for a batch nothing was skipped in", len(resp.Msg.GetRows()))
		}
	})

	t.Run("ListSkippedRows, unauthenticated", func(t *testing.T) {
		_, err := h.ListSkippedRows(context.Background(), connect.NewRequest(&vektv1.ListSkippedRowsRequest{
			OrgId: org.UUID().String(), BatchId: batchID,
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("ListSkippedRows, invalid batch_id", func(t *testing.T) {
		_, err := h.ListSkippedRows(ctx, connect.NewRequest(&vektv1.ListSkippedRowsRequest{
			OrgId: org.UUID().String(), BatchId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("ListInternalTransfers", func(t *testing.T) {
		resp, err := h.ListInternalTransfers(ctx, connect.NewRequest(&vektv1.ListInternalTransfersRequest{
			OrgId: org.UUID().String(), EntityId: entityID.String(),
		}))
		if err != nil {
			t.Fatalf("ListInternalTransfers: %v", err)
		}
		if len(resp.Msg.GetTransfers()) != 0 {
			t.Errorf("got %d transfers, want 0", len(resp.Msg.GetTransfers()))
		}
	})

	t.Run("ListInternalTransfers, unauthenticated", func(t *testing.T) {
		_, err := h.ListInternalTransfers(context.Background(), connect.NewRequest(&vektv1.ListInternalTransfersRequest{
			OrgId: org.UUID().String(), EntityId: entityID.String(),
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("ListInternalTransfers, invalid entity_id", func(t *testing.T) {
		_, err := h.ListInternalTransfers(ctx, connect.NewRequest(&vektv1.ListInternalTransfersRequest{
			OrgId: org.UUID().String(), EntityId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("DismissInternalTransfer, not found", func(t *testing.T) {
		_, err := h.DismissInternalTransfer(ctx, connect.NewRequest(&vektv1.DismissInternalTransferRequest{
			OrgId: org.UUID().String(), TransferId: uuid.NewString(),
		}))
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("DismissInternalTransfer, unauthenticated", func(t *testing.T) {
		_, err := h.DismissInternalTransfer(context.Background(), connect.NewRequest(&vektv1.DismissInternalTransferRequest{
			OrgId: org.UUID().String(), TransferId: uuid.NewString(),
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("DismissInternalTransfer, invalid transfer_id", func(t *testing.T) {
		_, err := h.DismissInternalTransfer(ctx, connect.NewRequest(&vektv1.DismissInternalTransferRequest{
			OrgId: org.UUID().String(), TransferId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})
}

func TestImportProfileHandlers(t *testing.T) {
	h, d := testHandler(t)
	owner := testUser(t, d)
	org, _ := testOrgAndEntity(t, d, owner)
	ctx := authedContext(t, owner)

	created, err := h.CreateImportProfile(ctx, connect.NewRequest(&vektv1.CreateImportProfileRequest{
		OrgId: org.UUID().String(), Name: "Test profile",
		SourceKind: vektv1.SourceKind_SOURCE_KIND_BANK,
		ColumnMap:  map[string]string{"Назначение": "description"},
	}))
	if err != nil {
		t.Fatalf("CreateImportProfile: %v", err)
	}
	profileID := created.Msg.GetProfile().GetId()
	if profileID == "" {
		t.Fatal("no profile id in the response")
	}

	t.Run("unauthenticated", func(t *testing.T) {
		_, err := h.CreateImportProfile(context.Background(), connect.NewRequest(&vektv1.CreateImportProfileRequest{
			OrgId: org.UUID().String(), Name: "Another", SourceKind: vektv1.SourceKind_SOURCE_KIND_BANK,
			ColumnMap: map[string]string{"a": "description"},
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("invalid org_id", func(t *testing.T) {
		_, err := h.CreateImportProfile(ctx, connect.NewRequest(&vektv1.CreateImportProfileRequest{
			OrgId: "not-a-uuid", Name: "Another", SourceKind: vektv1.SourceKind_SOURCE_KIND_BANK,
			ColumnMap: map[string]string{"a": "description"},
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("bad canonical field", func(t *testing.T) {
		_, err := h.CreateImportProfile(ctx, connect.NewRequest(&vektv1.CreateImportProfileRequest{
			OrgId: org.UUID().String(), Name: "Bad", SourceKind: vektv1.SourceKind_SOURCE_KIND_BANK,
			ColumnMap: map[string]string{"a": "descrption"},
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("GetImportProfile", func(t *testing.T) {
		resp, err := h.GetImportProfile(ctx, connect.NewRequest(&vektv1.GetImportProfileRequest{
			OrgId: org.UUID().String(), ProfileId: profileID,
		}))
		if err != nil {
			t.Fatalf("GetImportProfile: %v", err)
		}
		if resp.Msg.GetProfile().GetId() != profileID {
			t.Errorf("Id = %s, want %s", resp.Msg.GetProfile().GetId(), profileID)
		}
	})

	t.Run("GetImportProfile, unauthenticated", func(t *testing.T) {
		_, err := h.GetImportProfile(context.Background(), connect.NewRequest(&vektv1.GetImportProfileRequest{
			OrgId: org.UUID().String(), ProfileId: profileID,
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("GetImportProfile, not found", func(t *testing.T) {
		_, err := h.GetImportProfile(ctx, connect.NewRequest(&vektv1.GetImportProfileRequest{
			OrgId: org.UUID().String(), ProfileId: uuid.NewString(),
		}))
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("GetImportProfile, invalid profile_id", func(t *testing.T) {
		_, err := h.GetImportProfile(ctx, connect.NewRequest(&vektv1.GetImportProfileRequest{
			OrgId: org.UUID().String(), ProfileId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("ListImportProfiles", func(t *testing.T) {
		resp, err := h.ListImportProfiles(ctx, connect.NewRequest(&vektv1.ListImportProfilesRequest{
			OrgId: org.UUID().String(),
		}))
		if err != nil {
			t.Fatalf("ListImportProfiles: %v", err)
		}
		if len(resp.Msg.GetProfiles()) == 0 {
			t.Error("no profiles listed")
		}
	})

	t.Run("ListImportProfiles, unauthenticated", func(t *testing.T) {
		_, err := h.ListImportProfiles(context.Background(), connect.NewRequest(&vektv1.ListImportProfilesRequest{
			OrgId: org.UUID().String(),
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("ListImportProfiles, invalid org_id", func(t *testing.T) {
		_, err := h.ListImportProfiles(ctx, connect.NewRequest(&vektv1.ListImportProfilesRequest{
			OrgId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("UpdateImportProfile", func(t *testing.T) {
		resp, err := h.UpdateImportProfile(ctx, connect.NewRequest(&vektv1.UpdateImportProfileRequest{
			OrgId: org.UUID().String(), ProfileId: profileID, Name: "Renamed",
			ColumnMap: map[string]string{"Назначение": "description"},
		}))
		if err != nil {
			t.Fatalf("UpdateImportProfile: %v", err)
		}
		if resp.Msg.GetProfile().GetName() != "Renamed" {
			t.Errorf("Name = %s, want Renamed", resp.Msg.GetProfile().GetName())
		}
	})

	t.Run("UpdateImportProfile, unauthenticated", func(t *testing.T) {
		_, err := h.UpdateImportProfile(context.Background(), connect.NewRequest(&vektv1.UpdateImportProfileRequest{
			OrgId: org.UUID().String(), ProfileId: profileID, Name: "Renamed",
			ColumnMap: map[string]string{"a": "description"},
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("UpdateImportProfile, invalid org_id", func(t *testing.T) {
		_, err := h.UpdateImportProfile(ctx, connect.NewRequest(&vektv1.UpdateImportProfileRequest{
			OrgId: "not-a-uuid", ProfileId: profileID, Name: "Renamed",
			ColumnMap: map[string]string{"a": "description"},
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("UpdateImportProfile, invalid profile_id", func(t *testing.T) {
		_, err := h.UpdateImportProfile(ctx, connect.NewRequest(&vektv1.UpdateImportProfileRequest{
			OrgId: org.UUID().String(), ProfileId: "not-a-uuid", Name: "Renamed",
			ColumnMap: map[string]string{"a": "description"},
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("DeleteImportProfile, unauthenticated", func(t *testing.T) {
		_, err := h.DeleteImportProfile(context.Background(), connect.NewRequest(&vektv1.DeleteImportProfileRequest{
			OrgId: org.UUID().String(), ProfileId: profileID,
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("DeleteImportProfile, invalid org_id", func(t *testing.T) {
		_, err := h.DeleteImportProfile(ctx, connect.NewRequest(&vektv1.DeleteImportProfileRequest{
			OrgId: "not-a-uuid", ProfileId: profileID,
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("DeleteImportProfile, invalid profile_id", func(t *testing.T) {
		_, err := h.DeleteImportProfile(ctx, connect.NewRequest(&vektv1.DeleteImportProfileRequest{
			OrgId: org.UUID().String(), ProfileId: "not-a-uuid",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	// Last: DeleteImportProfile's own happy path, so later subtests above
	// that reference profileID still find it.
	t.Run("DeleteImportProfile", func(t *testing.T) {
		_, err := h.DeleteImportProfile(ctx, connect.NewRequest(&vektv1.DeleteImportProfileRequest{
			OrgId: org.UUID().String(), ProfileId: profileID,
		}))
		if err != nil {
			t.Fatalf("DeleteImportProfile: %v", err)
		}
	})
}
