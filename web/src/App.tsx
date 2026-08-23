import type { Transport } from "@connectrpc/connect";

import { HealthCard } from "./HealthCard";

export function App({ transport }: { transport: Transport }) {
  return (
    <main className="mx-auto flex min-h-dvh max-w-lg flex-col justify-center gap-6 p-8">
      <div>
        <h1 className="text-2xl font-semibold text-slate-900">Vekst</h1>
        <p className="text-sm text-slate-500">
          Walking skeleton — every value below crossed both service boundaries.
        </p>
      </div>
      <HealthCard transport={transport} />
    </main>
  );
}
