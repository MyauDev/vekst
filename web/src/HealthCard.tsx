import { useQuery } from "@tanstack/react-query";
import { createClient, type Transport } from "@connectrpc/connect";

import { HealthService } from "./gen/vekst/v1/health_pb";

/**
 * The walking skeleton, and the acceptance test for change 0.1.
 *
 * The values below travelled browser -> Connect -> core -> gRPC -> classifier
 * and back, through types nobody wrote by hand.
 */
export function HealthCard({ transport }: { transport: Transport }) {
  const { data, error, isPending } = useQuery({
    queryKey: ["health"],
    queryFn: () => createClient(HealthService, transport).check({}),
  });

  if (isPending) return <p className="text-text-muted">Checking…</p>;
  if (error) return <p className="text-danger">core unreachable: {error.message}</p>;

  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 font-mono text-sm">
      <Row label="status" value={data.status === 1 ? "SERVING" : "NOT SERVING"} />
      <Row label="core" value={data.version} />
      <Row label="built" value={data.builtAt} />
      {/* Empty is a valid answer, not an error: a classifier outage is a
          retryable condition, not an outage of core. ARCHITECTURE.md 3.5. */}
      <Row
        label="classifier"
        value={data.classifierVersion || "unreachable"}
        muted={!data.classifierVersion}
      />
    </dl>
  );
}

function Row({ label, value, muted }: { label: string; value: string; muted?: boolean }) {
  return (
    <>
      <dt className="text-text-muted">{label}</dt>
      <dd className={muted ? "text-warn" : "text-text"}>{value}</dd>
    </>
  );
}
