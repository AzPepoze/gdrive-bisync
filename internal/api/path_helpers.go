package api

import "regexp"

func shouldIgnorePath(path string, ignoreRegexps []*regexp.Regexp) bool {
	for _, re := range ignoreRegexps {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}
