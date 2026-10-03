package domain

import "strings"

func MergeLoginPath(current, login string) string {
	seen := map[string]bool{}
	for _, dir := range strings.Split(current, ":") {
		seen[dir] = true
	}
	merged := current
	for _, dir := range strings.Split(login, ":") {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		if merged == "" {
			merged = dir
		} else {
			merged += ":" + dir
		}
	}
	return merged
}
