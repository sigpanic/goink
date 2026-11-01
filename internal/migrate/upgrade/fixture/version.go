package fixture

import (
	"regexp"
	"strconv"
)

var stableTag = regexp.MustCompile(`^v([1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func releaseVersion(tag string) ([3]uint64, bool) {
	var version [3]uint64
	parts := stableTag.FindStringSubmatch(tag)
	if parts == nil {
		return version, false
	}
	for i := range version {
		value, err := strconv.ParseUint(parts[i+1], 10, 64)
		if err != nil {
			return version, false
		}
		version[i] = value
	}
	return version, true
}

func SupportedTag(tag string) bool {
	_, ok := releaseVersion(tag)
	return ok
}

// BeforeTarget selects stable releases since v1.0.0. An empty or non-release
// target denotes a development build and includes every supported release.
func BeforeTarget(tag, target string) bool {
	source, ok := releaseVersion(tag)
	if !ok {
		return false
	}
	destination, bounded := releaseVersion(target)
	if !bounded {
		return true
	}
	for i := range source {
		if source[i] != destination[i] {
			return source[i] < destination[i]
		}
	}
	return false
}

func (m Manifest) Historical(target string) []Fixture {
	var samples []Fixture
	for _, sample := range m.Fixtures {
		if BeforeTarget(sample.Tag, target) {
			samples = append(samples, sample)
		}
	}
	return samples
}
