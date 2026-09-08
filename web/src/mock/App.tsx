import { useCallback, useState } from "react";

import { REVIEW_GROUPS } from "./data";
import type { ReviewGroup } from "./data";
import type { Locale } from "./money";
import { ImportsScreen } from "./ImportsScreen";
import { ReportScreen } from "./ReportScreen";
import { ReviewScreen } from "./ReviewScreen";
import type { Resolution } from "./ReviewScreen";
import { TokensScreen } from "./TokensScreen";
import { Rail, TopBar, c } from "./ui";
import type { Screen } from "./ui";

export function App() {
  const [screen, setScreen] = useState<Screen>("reports");
  const [lang, setLang] = useState<Locale>("en");
  const [queue, setQueue] = useState<readonly ReviewGroup[]>(REVIEW_GROUPS);
  const [resolved, setResolved] = useState<readonly Resolution[]>([]);

  const onResolve = useCallback((group: ReviewGroup, decision: string) => {
    setQueue((current) => current.filter((g) => g.id !== group.id));
    setResolved((current) => [...current, { counterparty: group.counterparty, decision, rows: group.rows }]);
  }, []);

  const pending = queue.reduce((acc, group) => acc + group.rows, 0);

  return (
    <div style={{ minHeight: "100dvh", display: "flex", position: "relative", background: c.surface }}>
      <Rail screen={screen} onNavigate={setScreen} lang={lang} pending={pending} />
      <div style={{ flexGrow: 1, display: "flex", flexDirection: "column", minWidth: 0 }}>
        <TopBar lang={lang} onLang={setLang} />
        {screen === "reports" ? <ReportScreen lang={lang} /> : null}
        {screen === "imports" ? <ImportsScreen lang={lang} /> : null}
        {screen === "review" ? (
          <ReviewScreen lang={lang} queue={queue} resolved={resolved} onResolve={onResolve} />
        ) : null}
        {screen === "tokens" ? <TokensScreen lang={lang} /> : null}
      </div>
    </div>
  );
}
