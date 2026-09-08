/**
 * Upload, with the `source_kind` chosen **before the file is read**.
 *
 * `DESIGN.md` §8 and `WORKFLOW.md`: the tag decides the accounting basis and is
 * not optional. It cannot be recovered from the file either -- a CSV of
 * payments looks identical whether it came from a bank or a ledger -- and
 * guessing wrong makes every report built on it wrong in a way nothing
 * downstream can detect. So the choice gates the file input rather than sitting
 * beside it.
 */
import { useRef, useState } from "react";

import { uploadBatch } from "../data/imports";
import type { SourceKind } from "../data/types";
import { t } from "../i18n";
import type { Locale } from "../i18n";

export function Upload({ locale, onUploaded }: Readonly<{ locale: Locale; onUploaded: () => void }>) {
  const [sourceKind, setSourceKind] = useState<SourceKind | null>(null);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  async function onPick(file: File | undefined) {
    if (!file || !sourceKind) return;
    setBusy(true);
    try {
      await uploadBatch({ file, sourceKind });
      onUploaded();
      setSourceKind(null);
      if (fileRef.current) fileRef.current.value = "";
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="border-b border-border pb-5">
      <h2 className="text-2xs font-semibold uppercase tracking-widest text-text">
        {t("imports.chooseSource", locale)}
      </h2>

      <div className="mt-3 flex flex-wrap items-center gap-5">
        {(["bank", "ledger"] as const).map((k) => (
          <button
            key={k}
            type="button"
            onClick={() => setSourceKind(k)}
            aria-pressed={sourceKind === k}
            className={
              sourceKind === k
                ? "border-b-2 border-text pb-0.5 text-sm font-semibold text-text"
                : "border-b-2 border-transparent pb-0.5 text-sm text-text-muted"
            }
          >
            {t(`imports.source.${k}`, locale)}
          </button>
        ))}

        {/* The file input only exists once a source is chosen. Disabled would
            invite the click and then explain nothing. */}
        {sourceKind ? (
          <label className="cursor-pointer border-b-2 border-text pb-0.5 text-sm font-semibold uppercase tracking-widest text-text">
            {busy ? "…" : t("imports.upload", locale)}
            <input
              ref={fileRef}
              type="file"
              accept=".csv,.xlsx"
              className="sr-only"
              onChange={(e) => void onPick(e.target.files?.[0])}
            />
          </label>
        ) : null}
      </div>

      <p className="mt-3 max-w-prose text-xs text-text-muted">{t("imports.sourceNote", locale)}</p>
    </section>
  );
}
