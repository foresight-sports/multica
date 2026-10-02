package execenv

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPreparationBusySurvivesHelperBoundary(t *testing.T) {
	e := fmt.Errorf("claim: %w", ErrEnvRootBusy)
	if !errors.Is(rehydratePreparationError(e.Error(), preparationErrorKind(e)), ErrEnvRootBusy) {
		t.Fatal("busy claim would bypass bounded retry")
	}
}
func TestPreparationDiagnosticsBoundedTail(t *testing.T) {
	var b preparationDiagnostics
	_, _ = b.Write([]byte(strings.Repeat("old line\n", 10000) + "last diagnostic\n"))
	if b.Len() > 65536 || !strings.Contains(b.String(), "last diagnostic") {
		t.Fatal("missing bounded diagnostic tail")
	}
}
