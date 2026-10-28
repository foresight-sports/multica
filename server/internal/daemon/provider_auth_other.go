//go:build !windows

package daemon

import (
	"context"
	"fmt"
)

func runProviderAuth(context.Context, string, string) error {
	return fmt.Errorf("provider setup requires Windows")
}
func applyProviderAuthEnvironment(string, map[string]string) {}
func providerAuthCapabilityVersion() int                     { return 0 }
