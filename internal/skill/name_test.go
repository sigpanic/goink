package skill

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateName(t *testing.T) {
	valid := []string{"demo", "my skill", "-draft", ".hidden", "技能"}
	for _, name := range valid {
		require.NoError(t, ValidateName(name), name)
	}

	invalid := []string{
		"", " ", " demo", "demo ", ".", "..",
		"a/b", `a\b`,
		"a.md", "a.MD",
		"a:b", "a*b", "a?b", `a"b`, "a<b", "a>b", "a|b",
	}
	for _, name := range invalid {
		require.ErrorIs(t, ValidateName(name), ErrInvalidName, name)
	}
}
