package utils

import (
	"regexp"
	"strings"
)

// companySuffixPattern matches trailing country/region and legal-entity suffixes
// that sources append inconsistently: the same employer appears as both
// "Thales" and "Thales Australia", or "Amazon" and "Amazon Australia and New
// Zealand". Company name feeds the listing fingerprint, so an unstripped suffix
// mints a separate document for what is really the same employer.
//
// "Group" is deliberately NOT stripped. It is part of the canonical name for
// employers like Macquarie Group and Coles Group, and removing it would rename
// them on the public job board rather than just deduplicate them.
var companySuffixPattern = regexp.MustCompile(
	`(?i)\s+(` +
		`australia\s+and\s+new\s+zealand|` +
		`australia\s+&\s+new\s+zealand|` +
		`australia\s+pty\s+ltd|` +
		`australia|` +
		`new\s+zealand|` +
		`anz|` +
		`au|` +
		`pty\s+ltd|` +
		`pty\s+limited|` +
		`limited|` +
		`ltd` +
		`)$`)

// StripCompanySuffix removes trailing country and legal-entity suffixes from a
// company name, repeatedly, so "Datacom Australia Pty Ltd" reduces to
// "Datacom". A name that is entirely a suffix is returned unchanged rather than
// reduced to the empty string.
func StripCompanySuffix(name string) string {
	trimmed := strings.TrimSpace(name)

	for {
		stripped := strings.TrimSpace(companySuffixPattern.ReplaceAllString(trimmed, ""))
		if stripped == trimmed || stripped == "" {
			return trimmed
		}
		trimmed = stripped
	}
}
