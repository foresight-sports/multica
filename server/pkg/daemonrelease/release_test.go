package daemonrelease

import "testing"

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{{"0.4.44-foresight.10", "0.4.44-foresight.9", true}, {"0.4.44-foresight.9", "0.4.44-foresight.10", false}, {"0.4.44-foresight.10", "0.4.44-foresight.10", false}, {"0.4.44", "0.4.44-foresight.9", false}, {"0.4.44-foresight.10", "dev", false}, {"0.5.0-foresight.1", "0.4.44-foresight.10", true}} {
		if got := Newer(tc.latest, tc.current); got != tc.want {
			t.Errorf("Newer(%q,%q)=%v", tc.latest, tc.current, got)
		}
	}
}
func TestAssetNameRejectsPaths(t *testing.T) {
	for _, s := range []string{"../windows", "linux/../../", "", "other"} {
		if AssetName(s, "amd64") != "" {
			t.Fatal(s)
		}
	}
}
