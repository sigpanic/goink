package fixture

import (
	"reflect"
	"slices"
	"testing"
)

func TestSeedDeltasAndOrigins(t *testing.T) {
	base, err := SeedIDs("v1.0.0", "fresh")
	if err != nil {
		t.Fatal(err)
	}
	for _, delta := range SeedDeltas {
		t.Run(delta.Tag, func(t *testing.T) {
			fresh, err := SeedIDs(delta.Tag, "fresh")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := BuildSeed(fresh); err != nil {
				t.Fatal(err)
			}
			from, err := SeedIDs(delta.Tag, "from-v1.0.0")
			if err != nil || !reflect.DeepEqual(from, base) {
				t.Fatalf("from origin must use the original cases: got=%v error=%v", from, err)
			}
		})
	}
	old, _ := SeedIDs("v1.5.0", "fresh")
	current, _ := SeedIDs("v1.8.0", "fresh")
	if slices.Contains(old, "volumes") || !slices.Contains(current, "volumes") || slices.Contains(current, "settings-legacy") {
		t.Fatal("version deltas did not replace old layouts and add new scenarios")
	}
	if _, err := SeedIDs("v1.8.1", "fresh"); err == nil {
		t.Fatal("missing release delta must fail")
	}
}

func TestApplyDeltaDoesNotModifyPreviousSelection(t *testing.T) {
	ids := []string{"old", "keep"}
	got, err := ApplyDelta(ids, SeedDelta{Remove: []string{"old"}, Add: []string{"new"}})
	if err != nil || !reflect.DeepEqual(got, []string{"keep", "new"}) || !reflect.DeepEqual(ids, []string{"old", "keep"}) {
		t.Fatalf("delta result=%v previous=%v error=%v", got, ids, err)
	}
	for _, delta := range []SeedDelta{{Remove: []string{"missing"}}, {Add: []string{"keep"}}, {Add: []string{"new", "new"}}} {
		if _, err := ApplyDelta(ids, delta); err == nil {
			t.Fatalf("invalid delta accepted: %+v", delta)
		}
	}
}

func TestSeedInputsAreDetachedAndDigestBound(t *testing.T) {
	ids, _ := SeedIDs("v1.5.0", "fresh")
	hash, err := SeedHash(ids)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := BuildSeed(ids)
	data.Rows[0].Values["title"] = "accidental mutation"
	again, _ := SeedHash(ids)
	if hash != again {
		t.Fatal("building a seed leaked mutable case data")
	}
	f := Fixture{Tag: "v1.5.0", Origin: "fresh", CaseIDs: ids, InputSHA256: hash}
	if err := ValidateSeed(f); err != nil {
		t.Fatal(err)
	}
	f.InputSHA256 = "tampered"
	if err := ValidateSeed(f); err == nil {
		t.Fatal("changed input digest accepted")
	}
	f.InputSHA256 = hash
	f.CaseIDs = ids[:len(ids)-1]
	if err := ValidateSeed(f); err == nil {
		t.Fatal("changed case selection accepted")
	}
	if _, err := BuildSeed([]string{"settings-legacy", "settings-native"}); err == nil {
		t.Fatal("two scenarios owning the same table were accepted")
	}
	if _, err := BuildSeed([]string{"missing"}); err == nil {
		t.Fatal("unknown case accepted")
	}
}
