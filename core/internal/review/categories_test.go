package review_test

import (
	"context"
	"testing"
)

// Task 7.7. ListCategories returns no computed line and no non-leaf node.
// Negative scenario: a picker that offers GM ("91", a computed line) or "04"
// (OPEX, a section) produces a classification the report cannot place --
// ClassifiableCategories already refuses both, and this is that refusal
// exercised through review.Service.Categories.
func TestCategoriesOffersNoComputedLineAndNoSection(t *testing.T) {
	f := newFixture(t)

	cats, err := f.svc.Categories(context.Background(), f.owner, f.orgID)
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if len(cats) == 0 {
		t.Fatal("Categories returned none; the fixture's organisation should have the shared taxonomy at least")
	}

	forbidden := map[string]string{
		"91": "GM, a computed line",
		"92": "NM, a computed line",
		"93": "CM, a computed line",
		"94": "IBT, a computed line",
		"95": "NI, a computed line",
		"04": "OPEX, a section (not a leaf)",
		"01": "NET SALES, a section (not a leaf)",
	}
	for _, c := range cats {
		if reason, ok := forbidden[c.Code]; ok {
			t.Errorf("Categories offered %q (%s)", c.Code, reason)
		}
		if c.Path == "" {
			t.Errorf("category %q has an empty path", c.Code)
		}
	}
}
