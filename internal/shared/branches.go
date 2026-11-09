package shared

import "regexp"

var branchPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9/_-]{0,79}$`)

func ValidBranch(value string) bool { return branchPattern.MatchString(value) }
