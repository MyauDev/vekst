/**
 * The two RPCs add-ingest-validation adds: reading a batch's validation
 * report, and overriding a completeness warning.
 *
 * Named distinctly from `./imports.ts`'s own `ValidationError` (a mock
 * fixture shape predating this backend) rather than reusing it -- the two
 * disagree on field names (`fileLine`/`detail` there, `line`/`raw` here,
 * because the proto is `line`/`field`/`code`/`raw`), and reconciling them is
 * the screen's job (`add-web-experience` §6), not this module's.
 */
import { createClient } from "@connectrpc/connect";
import { timestampDate } from "@bufbuild/protobuf/wkt";

import { transport } from "../transport";
import { ImportService, ValidationOutcome as ProtoValidationOutcome } from "../gen/vekst/v1/import_pb";
import type { ValidationReport as ProtoValidationReport } from "../gen/vekst/v1/import_pb";

const client = createClient(ImportService, transport);

export type ValidationOutcome = "valid" | "valid_with_warnings" | "rejected";

function outcomeFromProto(o: ProtoValidationOutcome): ValidationOutcome {
  switch (o) {
    case ProtoValidationOutcome.VALID:
      return "valid";
    case ProtoValidationOutcome.REJECTED:
      return "rejected";
    default:
      return "valid_with_warnings";
  }
}

export interface ValidationErrorDetail {
  line: number;
  field: string;
  /** A code, never a sentence. The client owns the words (`error.${code}`). */
  code: string;
  raw: string;
}

export interface BalanceMismatchDetail {
  opening: bigint;
  movements: bigint;
  closing: bigint;
  difference: bigint;
  currency: string;
}

export interface ValidationWarningDetail {
  code: string;
  balanceMismatch?: BalanceMismatchDetail;
}

export interface ValidationReportView {
  batchId: string;
  outcome: ValidationOutcome;
  rowCount: number;
  errorCount: number;
  warningCount: number;
  /** undefined means the file declared no balances to check -- distinct from `false`. */
  balanceCheckPassed?: boolean;
  errors: ValidationErrorDetail[];
  warnings: ValidationWarningDetail[];
  /** Empty when the batch has never been overridden. */
  overriddenByUserId: string;
  overrideReason: string;
  overriddenAt?: Date;
}

function reportFromProto(report: ProtoValidationReport): ValidationReportView {
  return {
    batchId: report.batchId,
    outcome: outcomeFromProto(report.outcome),
    rowCount: report.rowCount,
    errorCount: report.errorCount,
    warningCount: report.warningCount,
    balanceCheckPassed: report.balanceCheckPassed,
    errors: report.errors.map((e) => ({ line: e.line, field: e.field, code: e.code, raw: e.raw })),
    warnings: report.warnings.map((w) => ({ code: w.code, balanceMismatch: w.balanceMismatch })),
    overriddenByUserId: report.overriddenByUserId,
    overrideReason: report.overrideReason,
    overriddenAt: report.overriddenAt ? timestampDate(report.overriddenAt) : undefined,
  };
}

export async function getValidationReport(input: { orgId: string; batchId: string }): Promise<ValidationReportView> {
  const res = await client.getValidationReport({ orgId: input.orgId, batchId: input.batchId });
  if (!res.report) {
    throw new Error("getValidationReport: response carried no report");
  }
  return reportFromProto(res.report);
}

/**
 * A completeness warning may be overridden; a correctness error may not
 * (design D1) -- refused server-side, by a database constraint the handler
 * cannot bypass, not just by this client's own good behaviour.
 */
export async function overrideValidation(input: {
  orgId: string;
  batchId: string;
  reason: string;
}): Promise<ValidationReportView> {
  const res = await client.overrideValidation({ orgId: input.orgId, batchId: input.batchId, reason: input.reason });
  if (!res.report) {
    throw new Error("overrideValidation: response carried no report");
  }
  return reportFromProto(res.report);
}
