// Package decision holds the rules for whether a release is worth
// grabbing for an episode, given its quality profile and the file the
// episode already has. Automatic search and RSS sync share these rules so
// they can't drift apart.
package decision

import (
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// Rejection reasons, stable strings suitable for logs and API responses.
const (
	ReasonBelowProfile     = "below_quality_profile"
	ReasonNotAnUpgrade     = "not_an_upgrade"
	ReasonUpgradesDisabled = "upgrades_disabled"
	ReasonUpgradeLimit     = "upgrade_until_reached"
	ReasonCurrentUnknown   = "current_quality_unknown"
)

// MeetsProfileMinimum reports whether q is acceptable for profile p at all.
//
// The rule gates on resolution only: a release whose resolution is below
// the cutoff's resolution is rejected. A strict match on the exact
// source+codec combination would reject almost every real release (see
// plans/sonarr-parity.md B1, quality groups, which will replace this).
// A nil profile, or one with no meaningful cutoff, accepts everything.
func MeetsProfileMinimum(q plugin.Quality, p *quality.Profile) bool {
	if p == nil {
		return true
	}
	cutoffRes := p.Cutoff.Score() / 100
	if cutoffRes == 0 {
		return true
	}
	return q.Score()/100 >= cutoffRes
}

// Evaluate decides whether to grab a release of quality candidate for an
// episode whose current file has quality current (nil when the episode
// has no file). It returns ok=true, or ok=false with a reason.
//
// Rules, in order:
//  1. The candidate must meet the profile minimum.
//  2. No current file: grab.
//  3. Current quality unknown (e.g. imported before quality was tracked):
//     don't upgrade automatically; a manual search still can.
//  4. Same quality (resolution + source; codec doesn't matter, as in
//     Sonarr), higher revision (PROPER/REPACK): grab. A proper fixes
//     a broken release of the quality you already chose, so it is taken
//     even when the cutoff is met or upgrades are off, as Sonarr's default
//     "prefer and upgrade" does.
//  5. Not strictly better than the current file: reject.
//  6. Current file below the cutoff: grab.
//  7. Cutoff met: grab only if the profile allows upgrades and the current
//     file is below its upgrade-until ceiling.
func Evaluate(candidate plugin.Quality, current *plugin.Quality, p *quality.Profile) (bool, string) {
	if !MeetsProfileMinimum(candidate, p) {
		return false, ReasonBelowProfile
	}
	if current == nil {
		return true, ""
	}
	if current.Score() == 0 {
		return false, ReasonCurrentUnknown
	}
	if sameQuality(candidate, *current) && candidate.EffectiveRevision() > current.EffectiveRevision() {
		return true, ""
	}
	if !candidate.BetterThan(*current) {
		return false, ReasonNotAnUpgrade
	}
	if p == nil || !current.AtLeast(p.Cutoff) {
		return true, ""
	}
	if !p.UpgradeAllowed {
		return false, ReasonUpgradesDisabled
	}
	if p.UpgradeUntil != nil && current.AtLeast(*p.UpgradeUntil) {
		return false, ReasonUpgradeLimit
	}
	return true, ""
}

// sameQuality reports whether a and b are the same resolution and source.
// Score is resolution*100 + source*10 + codec, so dropping the last digit
// ignores the codec: an x265 PROPER still counts as a PROPER of an x264
// file of the same resolution and source.
func sameQuality(a, b plugin.Quality) bool {
	return a.Score()/10 == b.Score()/10
}
