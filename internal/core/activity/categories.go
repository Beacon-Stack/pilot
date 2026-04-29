package activity

// Category groups related event types for filtering. The set is closed —
// every row in activity_log.category should be one of these constants.
// The frontend's Activity-page "Needs attention" rail filters on the
// failure variants; renaming or removing a constant means coordinating a
// migration AND a frontend update at the same time.
//
// Mirrored in:
//   - internal/db/migrations/00009_activity_categories.sql (back-fill)
//   - web/ui/src/api/activity.ts            (TypeScript union type)
//   - web/ui/src/pages/activity/categories.ts (display labels)
type Category string

const (
	// Grabs.
	CategoryGrabSucceeded Category = "grab_succeeded"
	CategoryGrabFailed    Category = "grab_failed"

	// Imports.
	CategoryImportSucceeded Category = "import_succeeded"
	CategoryImportFailed    Category = "import_failed"

	// Stalled grabs (Pilot-side classification of Haul stall reports).
	CategoryStalled Category = "stalled"

	// Library mutations.
	CategoryShow Category = "show"

	// Reserved — no emitter today, but kept so the migration is forward-only
	// when a task tracker / health emitter is wired up.
	CategoryTask   Category = "task"
	CategoryHealth Category = "health"
)

// allCategories is the closed set of new-style values.
var allCategories = map[Category]struct{}{
	CategoryGrabSucceeded:   {},
	CategoryGrabFailed:      {},
	CategoryImportSucceeded: {},
	CategoryImportFailed:    {},
	CategoryStalled:         {},
	CategoryShow:            {},
	CategoryTask:            {},
	CategoryHealth:          {},
}

// legacyCategories are the pre-00009_activity_categories values. We keep
// them ValidCategory-true so a) the API accepts them as filter inputs
// (back-compat for any external caller), and b) any rows that survived
// the back-fill (none expected, but defensive) round-trip without a 400.
var legacyCategories = map[Category]struct{}{
	"grab":   {},
	"import": {},
}

// ValidCategory reports whether c is one of the closed-set categories or
// a legacy value the API still accepts.
func ValidCategory(c string) bool {
	if _, ok := allCategories[Category(c)]; ok {
		return true
	}
	_, ok := legacyCategories[Category(c)]
	return ok
}

// AllCategories returns the canonical (post-migration) category values.
// Used by the API handler's docstring and (indirectly) the frontend
// command-palette suggestions.
func AllCategories() []Category {
	return []Category{
		CategoryGrabSucceeded,
		CategoryGrabFailed,
		CategoryImportSucceeded,
		CategoryImportFailed,
		CategoryStalled,
		CategoryShow,
		CategoryTask,
		CategoryHealth,
	}
}
