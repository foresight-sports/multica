package cli

import (
	"strings"
	"testing"
)

func TestInstanceUpdateRejectsChecksumBeforeInstall(t *testing.T) {
	if _, err := InstallInstanceBinary([]byte("untrusted"), strings.Repeat("0", 64)); err == nil {
		t.Fatal("invalid checksum accepted")
	}
}
