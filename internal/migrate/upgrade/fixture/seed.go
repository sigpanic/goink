package fixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture/seeddata"
)

type SeedDelta struct {
	Tag    string
	Remove []string
	Add    []string
}

// Each supported release is explicit, including releases with no input changes.
var SeedDeltas = []SeedDelta{
	{Tag: "v1.0.0", Add: []string{"novels", "chapters-numbered", "characters", "references-v100", "preferences", "settings-legacy", "sessions-legacy", "messages", "writing-v100", "llm-v1", "locations"}},
	{Tag: "v1.1.0", Add: []string{"styles"}},
	{Tag: "v1.1.1"},
	{Tag: "v1.2.0", Remove: []string{"references-v100", "writing-v100"}, Add: []string{"references-v120", "writing-v120"}},
	{Tag: "v1.3.0", Remove: []string{"settings-legacy"}, Add: []string{"setting-items", "settings-legacy-relay"}},
	{Tag: "v1.3.1"},
	{Tag: "v1.4.0"},
	{Tag: "v1.4.1"},
	{Tag: "v1.4.2"},
	{Tag: "v1.4.3"},
	{Tag: "v1.5.0"},
	{Tag: "v1.6.0", Remove: []string{"chapters-numbered", "references-v120", "writing-v120"}, Add: []string{"chapters-id", "references-id", "writing-id", "volumes"}},
	{Tag: "v1.6.1", Remove: []string{"writing-id"}, Add: []string{"writing-daily", "groups", "activity"}},
	{Tag: "v1.7.0"},
	{Tag: "v1.8.0", Remove: []string{"settings-legacy-relay", "sessions-legacy", "llm-v1"}, Add: []string{"settings-native", "sessions-native", "llm-v2"}},
}

func ApplyDelta(ids []string, delta SeedDelta) ([]string, error) {
	result := slices.Clone(ids)
	for _, id := range delta.Remove {
		i := slices.Index(result, id)
		if i < 0 {
			return nil, fmt.Errorf("%s: remove missing seed case %s", delta.Tag, id)
		}
		result = slices.Delete(result, i, i+1)
	}
	for _, id := range delta.Add {
		if slices.Contains(result, id) {
			return nil, fmt.Errorf("%s: duplicate seed case %s", delta.Tag, id)
		}
		result = append(result, id)
	}
	return result, nil
}

func SeedIDs(tag, origin string) ([]string, error) {
	if origin != "fresh" && origin != "from-v1.0.0" {
		return nil, fmt.Errorf("unsupported seed origin %q", origin)
	}
	var ids []string
	found := false
	for _, delta := range SeedDeltas {
		var err error
		ids, err = ApplyDelta(ids, delta)
		if err != nil {
			return nil, err
		}
		if delta.Tag == tag {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("release %s requires an explicit seed delta", tag)
	}
	if origin == "from-v1.0.0" {
		return slices.Clone(SeedDeltas[0].Add), nil
	}
	return ids, nil
}

func ResolveCases(ids []string) ([]seeddata.SeedCase, error) {
	registry := seedCases()
	seen := map[string]bool{}
	cases := make([]seeddata.SeedCase, 0, len(ids))
	for _, id := range ids {
		c, ok := registry[id]
		if !ok || seen[id] || c.AssertAfter == nil {
			return nil, fmt.Errorf("unknown, duplicate, or unchecked seed case %q", id)
		}
		seen[id] = true
		cases = append(cases, c)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("empty seed selection")
	}
	return cases, nil
}

func BuildSeed(ids []string) (seeddata.SeedData, error) {
	cases, err := ResolveCases(ids)
	if err != nil {
		return seeddata.SeedData{}, err
	}
	data := seeddata.SeedData{Files: map[string]string{}}
	owners := map[string]string{}
	for _, c := range cases {
		for _, row := range c.Before.Rows {
			// A table belongs to one scenario, allowing complete row-set assertions.
			if owner := owners[row.Table]; owner != "" && owner != c.ID {
				return seeddata.SeedData{}, fmt.Errorf("seed cases %s and %s both own table %s", owner, c.ID, row.Table)
			}
			owners[row.Table] = c.ID
			data.Rows = append(data.Rows, row)
		}
		for path, content := range c.Before.Files {
			if _, exists := data.Files[path]; exists {
				return seeddata.SeedData{}, fmt.Errorf("duplicate seed file %s", path)
			}
			data.Files[path] = content
		}
		if len(c.Before.LLMConfig) != 0 {
			if len(data.LLMConfig) != 0 {
				return seeddata.SeedData{}, fmt.Errorf("multiple seed LLM configurations")
			}
			data.LLMConfig = c.Before.LLMConfig
		}
	}
	// Round-tripping detaches returned maps and slices from case declarations.
	encoded, err := json.Marshal(data)
	if err != nil {
		return seeddata.SeedData{}, err
	}
	var detached seeddata.SeedData
	if err := json.Unmarshal(encoded, &detached); err != nil {
		return seeddata.SeedData{}, err
	}
	return detached, nil
}

func SeedHash(ids []string) (string, error) {
	data, err := BuildSeed(ids)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(struct {
		IDs  []string          `json:"case_ids"`
		Data seeddata.SeedData `json:"before"`
	}{ids, data})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func ValidateSeed(f Fixture) error {
	ids, err := SeedIDs(f.Tag, f.Origin)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(ids, f.CaseIDs) {
		return fmt.Errorf("fixture %s seed selection changed; rebuild its input", f.Archive)
	}
	hash, err := SeedHash(f.CaseIDs)
	if err != nil {
		return err
	}
	if hash != f.InputSHA256 {
		return fmt.Errorf("fixture %s seed input changed; rebuild its input", f.Archive)
	}
	return nil
}
