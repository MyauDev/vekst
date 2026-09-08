/**
 * ReviewScreen. Its content arrives with a later section of
 * `add-web-experience`; the empty state below is not a placeholder for that --
 * it is the state a real customer meets before they have imported anything,
 * and it stays when the screen is filled in.
 */
import { t } from "../i18n";
import { useLocale } from "../ui/preferences";
import { EmptyState } from "../ui/feedback";

export function ReviewScreen() {
  const [locale] = useLocale();
  return (
    <EmptyState
      title={t("empty.review.title", locale)}
      detail={t("empty.review.detail", locale)}
    />
  );
}
