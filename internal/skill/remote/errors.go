package remote

import "github.com/sigpanic/goink/internal/apperr"

const (
	CodeInvalidTarget           apperr.Code = "skill.invalid_target"
	CodeNovelRequired           apperr.Code = "skill.novel_required"
	CodeInvalidName             apperr.Code = "skill.invalid_name"
	CodeInvalidPath             apperr.Code = "skill.invalid_path"
	CodeExpectedContentRequired apperr.Code = "skill.expected_content_required"
	CodeInstallConflict         apperr.Code = "skill.install_conflict"
)
