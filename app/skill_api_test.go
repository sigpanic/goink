package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sigpanic/goink/internal/apperr"
)

func TestInstallRemoteSkillRequiresExpectedContent(t *testing.T) {
	a := &App{}
	result := a.InstallRemoteSkill(InstallRemoteSkillInput{
		Name:   "my-skill",
		Target: "user",
	})
	assert.Equal(t, apperr.CodeInvalid, result.ErrCode)
}
