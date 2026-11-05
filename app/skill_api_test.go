package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sigpanic/goink/internal/skill/remote"
)

func TestInstallRemoteSkillRequiresExpectedContent(t *testing.T) {
	a := &App{}
	result := a.InstallRemoteSkill(InstallRemoteSkillInput{
		Name:   "my-skill",
		Target: "user",
	})
	assert.Equal(t, remote.CodeExpectedContentRequired, result.ErrCode)
}
