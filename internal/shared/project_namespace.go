package shared

import "regexp"

var deploymentProjectID = regexp.MustCompile(`^prj_[a-f0-9]{32}$`)

// ProjectNamespace uses an immutable generated ID, never a user-supplied name.
// It cannot select system namespaces or collide after project renaming.
func ProjectNamespace(id string) string {
	if !deploymentProjectID.MatchString(id) {
		return ""
	}
	return "hm-prj-" + id[4:]
}
