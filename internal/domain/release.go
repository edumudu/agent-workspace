package domain

import (
	"strconv"
	"strings"
)

func NewerRelease(current, latest string) bool {
	cur, ok := parseSemver(current)
	if !ok {
		return false
	}
	lat, ok := parseSemver(latest)
	if !ok {
		return false
	}
	return compareSemver(lat, cur) > 0
}

func HighestRelease(tags []string) string {
	var best string
	var bestVer semver
	found := false
	for _, tag := range tags {
		v, ok := parseSemver(tag)
		if ok && (!found || compareSemver(v, bestVer) > 0) {
			best, bestVer, found = tag, v, true
		}
	}
	if !found && len(tags) > 0 {
		return tags[0]
	}
	return best
}

type semver struct {
	core [3]int
	pre  []string
}

func parseSemver(s string) (semver, bool) {
	s = strings.TrimPrefix(s, "v")
	s, build, hasBuild := strings.Cut(s, "+")
	if hasBuild && build == "" {
		return semver{}, false
	}
	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var v semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		v.core[i] = n
	}
	if hasPre {
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if id == "" || strings.Contains(id, "-") {
				return semver{}, false
			}
		}
	}
	return v, true
}

func compareSemver(a, b semver) int {
	for i := range a.core {
		if a.core[i] != b.core[i] {
			return cmpInt(a.core[i], b.core[i])
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := comparePreID(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(a.pre), len(b.pre))
}

func comparePreID(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return cmpInt(an, bn)
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
