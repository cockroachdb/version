// Copyright 2025 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package version

import (
	"cmp"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/cockroachdb/redact"
)

var _ redact.SafeFormatter = Version{}

type releasePhase int

const (
	alpha     = releasePhase(1)
	beta      = releasePhase(2)
	rc        = releasePhase(3)
	cloudonly = releasePhase(4)
	stable    = releasePhase(5)
	adhoc     = releasePhase(6)
)

// Version represents a CockroachDB (binary) version. Versions consist of three parts:
// a major version, written as "vX.Y" (which is typically the year and release number
// within the year), a patch version (the "Z" in "vX.Y.Z"), and sometimes one or more
// phases, sub-phases, and other suffixes. Note that CockroachDB versions are not
// semantic versions! You must use this package to parse and compare versions, in
// order to account for the variety of versions currently or historically in use.
type Version struct {
	// A version is composed of many (possible) fields. See [Parse] for details of
	// how a version string becomes these many fields. For comparison purposes, versions
	// are considered equal if they have equal values for all these fields; if not,
	// the fields are compared in the order listed here, and the earliest field with
	// a difference determines the relative ordering of two unequal versions.
	//
	// The reference order: year, ordinal, patch, phase, phaseOrdinal, phaseSubOrdinal, customOrdinal
	year, ordinal, patch                         int
	phase                                        releasePhase
	phaseOrdinal, phaseSubOrdinal, customOrdinal int
	adhocLabel                                   string
	// raw is the original, unprocessed string this Version was created with
	raw string
}

// Major returns the version's MajorVersion (the "vX.Y" part)
func (v Version) Major() MajorVersion {
	return MajorVersion{Year: v.year, Ordinal: v.ordinal}
}

// Patch returns the version's patch number.
func (v Version) Patch() int {
	return v.patch
}

// Format returns a string populated with parts of the version, using placeholders
// similar to the fmt package. The following placeholders are supported:
//
// - %X: year
// - %Y: ordinal
// - %Z: patch
// - %P: phase name (one of "alpha", "beta", "rc", "cloudonly")
// - %p: phase sort order (see the top of version.go)
// - %o: phase ordinal (eg, the 1 in "v24.1.0-rc.1")
// - %s: phase sub-ordinal (eg the 2 in "v24.1.0-rc.1-cloudonly.2")
// - %n: adhoc build ordinal (eg the 12 in "v24.1.0-12-gabcdef")
// - %%: literal "%"
func (v Version) Format(formatStr string) string {
	placeholderRe := regexp.MustCompile("%[^%XYZpPosn]")
	placeholders := placeholderRe.FindAllString(formatStr, -1)
	if len(placeholders) > 0 {
		panic(fmt.Sprintf("unknown placeholders in format string: %s", strings.Join(placeholders, ", ")))
	}

	phaseName := map[releasePhase]string{
		alpha:     "alpha",
		beta:      "beta",
		rc:        "rc",
		cloudonly: "cloudonly",
		adhoc:     "",
		stable:    "",
	}

	formatStr = strings.ReplaceAll(formatStr, "%X", strconv.Itoa(v.year))
	formatStr = strings.ReplaceAll(formatStr, "%Y", strconv.Itoa(v.ordinal))
	formatStr = strings.ReplaceAll(formatStr, "%Z", strconv.Itoa(v.patch))
	formatStr = strings.ReplaceAll(formatStr, "%p", strconv.Itoa(int(v.phase)))
	formatStr = strings.ReplaceAll(formatStr, "%P", phaseName[v.phase])
	formatStr = strings.ReplaceAll(formatStr, "%o", strconv.Itoa(v.phaseOrdinal))
	formatStr = strings.ReplaceAll(formatStr, "%s", strconv.Itoa(v.phaseSubOrdinal))
	formatStr = strings.ReplaceAll(formatStr, "%n", strconv.Itoa(v.customOrdinal))
	formatStr = strings.ReplaceAll(formatStr, "%%", "%")
	return formatStr
}

// Value implements [database/sql/driver.Valuer].
func (v Version) Value() (driver.Value, error) {
	return v.raw, nil
}

// Scan implements [database/sql.Scanner].
func (v *Version) Scan(value interface{}) error {
	if value == nil {
		return errors.New("non-nil Version string required")
	}
	if str, ok := value.(string); ok {
		if str == "" {
			// Parse() doesn't accept empty string, but we allow empty string as
			// equivalent to a null version
			*v = Version{}
		} else {
			parsed, err := Parse(str)
			if err != nil {
				return err
			}
			*v = parsed
		}
		return nil
	}
	return errors.Newf("cannot convert %T to Version", value)
}

// MarshalJSON implements [encoding/json.Marshaler].
func (v Version) MarshalJSON() ([]byte, error) {
	jsonData := map[string]string{
		"$raw": v.raw,
	}
	return json.Marshal(jsonData)
}

// UnmarshalJSON implements [encoding/json.Unmarshaler].
func (v *Version) UnmarshalJSON(data []byte) error {
	var rawMap map[string]string
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}
	if rawValue, ok := rawMap["$raw"]; ok {
		parsed, err := Parse(rawValue)
		if err != nil {
			return err
		}
		*v = parsed
		return nil
	}
	return errors.New("missing $raw key in Version JSON")
}

// IsPrerelease determines whether the version is a pre-release version.
func (v Version) IsPrerelease() bool {
	// cloudonly phase *is* stable, it's just not available to SH
	// customers, and has a special version suffix inside of CC
	return v.phase < cloudonly && !v.Empty()
}

// IsCustomOrAdhocBuild determines if the version is a adhoc build or adhoc build.
func (v Version) IsCustomOrAdhocBuild() bool {
	return v.IsCustomBuild() || v.IsAdhocBuild()
}

// IsCustomBuild determines if the version is a adhoc build.
func (v Version) IsCustomBuild() bool {
	return v.customOrdinal > 0
}

// IsAdhocBuild determines if the version is a adhoc build.
func (v Version) IsAdhocBuild() bool {
	return v.adhocLabel != ""
}

// IsCloudOnlyBuild determines if the version is a CockroachDB Cloud specific build.
func (v Version) IsCloudOnlyBuild() bool {
	return v.phase == cloudonly
}

// String returns the original version string passed to [Parse].
func (v Version) String() string {
	return redact.StringWithoutMarkers(v)
}

// SafeFormat implements [redact.SafePrinter].
func (v Version) SafeFormat(p redact.SafePrinter, _ rune) {
	p.Print(v.raw)
}

// Parse creates a version from a string.
func Parse(str string) (Version, error) {
	// these are roughly in "how often we expect to see them" order
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`^v(?P<year>[1-9][0-9]*)\.(?P<ordinal>[1-9][0-9]*)\.(?P<patch>(?:[1-9][0-9]*|0))(?:-fips)?$`),
		regexp.MustCompile(`^v(?P<year>[1-9][0-9]*)\.(?P<ordinal>[1-9][0-9]*)\.(?P<patch>(?:[1-9][0-9]*|0))-(?P<phase>alpha|beta|rc|cloudonly)\.(?P<phaseOrdinal>[0-9]+)(?:-fips)?$`),
		regexp.MustCompile(`^v(?P<year>[1-9][0-9]*)\.(?P<ordinal>[1-9][0-9]*)\.(?P<patch>(?:[1-9][0-9]*|0))-(?P<customOrdinal>(?:[1-9][0-9]*|0))-g[a-f0-9]+(?:-fips)?$`),
		regexp.MustCompile(`^v(?P<year>[1-9][0-9]*)\.(?P<ordinal>[1-9][0-9]*)\.(?P<patch>(?:[1-9][0-9]*|0))-(?P<phase>alpha|beta|rc|cloudonly).(?P<phaseOrdinal>[0-9]+)-(?P<customOrdinal>(?:[1-9][0-9]*|0))-g[a-f0-9]+(?:-fips)?$`),
		regexp.MustCompile(`^v(?P<year>[1-9][0-9]*)\.(?P<ordinal>[1-9][0-9]*)\.(?P<patch>(?:[1-9][0-9]*|0))-(?P<phase>alpha|beta|rc|cloudonly).(?P<phaseOrdinal>[0-9]+)-cloudonly(-rc|\.)(?P<phaseSubOrdinal>(?:[1-9][0-9]*|0))$`),
		regexp.MustCompile(`^v(?P<year>[1-9][0-9]*)\.(?P<ordinal>[1-9][0-9]*)\.(?P<patch>(?:[1-9][0-9]*|0))-(?P<phase>cloudonly)-rc(?P<phaseOrdinal>[0-9]+)$`),
		regexp.MustCompile(`^v(?P<year>[1-9][0-9]*)\.(?P<ordinal>[1-9][0-9]*)\.(?P<patch>(?:[1-9][0-9]*|0))-(?P<phase>cloudonly)(?P<phaseOrdinal>[0-9]+)?$`),

		// vX.Y.Z-<anything> will sort after the corresponding "plain" vX.Y.Z version
		regexp.MustCompile(`^v(?P<year>[1-9][0-9]*)\.(?P<ordinal>[1-9][0-9]*)\.(?P<patch>(?:[1-9][0-9]*|0))-(?P<adhocLabel>[-a-zA-Z0-9\.\+]+)$`),

		// sha256:<hash>:latest-vX.Y-build will sort just after vX.Y.0, but before vX.Y.1
		regexp.MustCompile(`^sha256:(?P<adhocLabel>[^:]+):latest-v(?P<year>[1-9][0-9]*)\.(?P<ordinal>[1-9][0-9]*)-build$`),
	}

	preReleasePhase := map[string]releasePhase{
		"alpha":     alpha,
		"beta":      beta,
		"rc":        rc,
		"cloudonly": cloudonly,
	}

	submatch := func(pat *regexp.Regexp, matches []string, group string) string {
		index := pat.SubexpIndex(group)
		if index == -1 {
			return ""
		}
		return matches[index]
	}

	v := Version{raw: str, phase: stable}

	for _, pat := range patterns {
		if pat.MatchString(str) {
			matches := pat.FindStringSubmatch(str)

			// all patterns have vX.Y
			v.year, _ = strconv.Atoi(submatch(pat, matches, "year"))
			v.ordinal, _ = strconv.Atoi(submatch(pat, matches, "ordinal"))

			// most have vX.Y.Z
			if patch := submatch(pat, matches, "patch"); patch != "" {
				v.patch, _ = strconv.Atoi(patch)
			}

			// handle -alpha.1, -rc.3, etc
			if phase := submatch(pat, matches, "phase"); phase != "" {
				if phaseName, ok := preReleasePhase[phase]; !ok {
					return Version{}, errors.Newf("unknown phase '%s", phaseName)
				} else {
					v.phase = phaseName
				}

				if ord := submatch(pat, matches, "phaseOrdinal"); ord != "" {
					v.phaseOrdinal, _ = strconv.Atoi(ord)
				}
				// -beta.1-cloudonly-rc1
				if subOrd := submatch(pat, matches, "phaseSubOrdinal"); subOrd != "" {
					v.phaseSubOrdinal, _ = strconv.Atoi(subOrd)
				}
			}

			// adhoc/adhoc builds, eg -10-g7890abcd
			if ord := submatch(pat, matches, "customOrdinal"); ord != "" {
				v.customOrdinal, _ = strconv.Atoi(ord)
			}

			// arbitrary/adhoc build tags; we have these old versions and need to parse them
			if adhocLabel := submatch(pat, matches, "adhocLabel"); adhocLabel != "" {
				v.phase = adhoc
				v.adhocLabel = adhocLabel
			}

			return v, nil
		}
	}

	err := errors.Errorf("invalid version string '%s'", str)
	return Version{}, err
}

// MustParse is like Parse but panics on any error. Recommended as an
// initializer for global values.
func MustParse(str string) Version {
	v, err := Parse(str)
	if err != nil {
		panic(err)
	}
	return v
}

// Compare returns -1, 0, or +1 indicating the relative ordering of versions.
//
// CockroachDB versions are not semantic versions. SemVer treats suffixes after
// the major.minor.patch quite generically; we have specific, known cases that
// have well-defined ordering requirements:
//
// There are 4 known named prerelease phases. In order, they are: alpha, beta,
// rc, cloudonly. Pre-release versions will look like "v24.1.0-cloudonly.1"
// or "v23.2.0-rc.1".
//
// Additionally, we have adhoc builds, which have suffixes like "-<n>-g<hex>",
// where <n> is an integer commit count past the branch point, and <hex> is
// the git SHA. These versions sort AFTER the corresponding "normal" version,
// eg "v24.1.0-1-g9cbe7c5281" is AFTER "v24.1.0".
//
// A version can have both a pre-release and adhoc build suffix, like
// "v24.1.0-rc.2-14-g<hex>". In these cases, the pre-release portion has precedence,
// so this example would sort after v24.1.0-rc.2, but before v24.1.0-rc.3.
func (v Version) Compare(w Version) int {
	if rslt := cmp.Compare(v.year, w.year); rslt != 0 {
		return rslt
	}
	if rslt := cmp.Compare(v.ordinal, w.ordinal); rslt != 0 {
		return rslt
	}
	if rslt := cmp.Compare(v.patch, w.patch); rslt != 0 {
		return rslt
	}
	if rslt := cmp.Compare(v.phase, w.phase); rslt != 0 {
		return rslt
	}
	if rslt := cmp.Compare(v.phaseOrdinal, w.phaseOrdinal); rslt != 0 {
		return rslt
	}
	if rslt := cmp.Compare(v.phaseSubOrdinal, w.phaseSubOrdinal); rslt != 0 {
		return rslt
	}
	if rslt := cmp.Compare(v.customOrdinal, w.customOrdinal); rslt != 0 {
		return rslt
	}
	if rslt := cmp.Compare(v.adhocLabel, w.adhocLabel); rslt != 0 {
		return rslt
	}
	return 0
}

func (v Version) Equals(w Version) bool {
	return v.Compare(w) == 0
}

func (v Version) LessThan(w Version) bool {
	return v.Compare(w) < 0
}

// Empty returns true if the version is the zero value.
func (v Version) Empty() bool {
	return v.Equals(Version{})
}

// Convenience wrapper for v.Major.Compare(w.Major())
func (v Version) CompareSeries(w Version) int {
	return v.Major().Compare(w.Major())
}

// AtLeast returns true if v >= w.
func (v Version) AtLeast(w Version) bool {
	return v.Compare(w) >= 0
}

// IncPatch returns a new version with the patch number incremented by 1.
// This method returns an error if the version is not a stable version.
func (v Version) IncPatch() (Version, error) {
	if v.phase != stable {
		return Version{}, fmt.Errorf("version %s is not a stable version", v.String())
	}
	nextVersion := Version{
		phase:   v.phase,
		year:    v.year,
		ordinal: v.ordinal,
		patch:   v.patch + 1,
	}
	nextVersion.raw = nextVersion.Format("v%X.%Y.%Z")
	return nextVersion, nil
}

// IncPreRelease returns a new version with the pre-release part incremented by 1.
// This method returns an error if the version is not a pre-release.
func (v Version) IncPreRelease() (Version, error) {
	if !(v.IsPrerelease()) {
		return Version{}, errors.New("version is not a prerelease")
	}
	if v.phaseSubOrdinal > 0 || v.customOrdinal > 0 {
		return Version{}, errors.New("only unmodified CRDB versions are supported")
	}
	nextVersion := Version{
		raw:          v.raw,
		phase:        v.phase,
		year:         v.year,
		ordinal:      v.ordinal,
		patch:        v.patch,
		phaseOrdinal: v.phaseOrdinal + 1,
	}
	nextVersion.raw = nextVersion.Format("v%X.%Y.%Z-%P.%o")
	return nextVersion, nil
}
