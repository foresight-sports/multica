package daemonrelease

import (
	"fmt"
	"regexp"
	"strconv"
)

const MaxBinarySize = 150 * 1024 * 1024

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)-foresight\.(\d+)$`)

type Asset struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Manifest struct {
	Version string           `json:"version"`
	Assets  map[string]Asset `json:"assets"`
}

func Parts(v string) ([4]int, error) {
	var p [4]int
	m := versionPattern.FindStringSubmatch(v)
	if m == nil {
		return p, fmt.Errorf("unrecognized instance release version")
	}
	for i := range p {
		n, e := strconv.Atoi(m[i+1])
		if e != nil {
			return p, e
		}
		p[i] = n
	}
	return p, nil
}
func Newer(latest, current string) bool {
	l, e := Parts(latest)
	if e != nil {
		return false
	}
	c, e := Parts(current)
	if e != nil {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
func AssetName(os, arch string) string {
	switch os {
	case "windows", "linux", "darwin":
	default:
		return ""
	}
	if arch != "amd64" && arch != "arm64" {
		return ""
	}
	s := "multica-" + os + "-" + arch
	if os == "windows" {
		s += ".exe"
	}
	return s
}
