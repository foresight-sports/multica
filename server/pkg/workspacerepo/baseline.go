package workspacerepo

import "regexp"

type Baseline struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	Revision   int64  `json:"revision"`
	Snapshot   string `json:"snapshot,omitempty"`
}

var commitSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)

func ValidCommit(s string) bool { return commitSHA.MatchString(s) }
