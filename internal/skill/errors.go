package skill

import "github.com/sigpanic/goink/internal/apperr"

const (
	// CodeInvalidName 与 remote.CodeInvalidName 使用同一契约值。
	CodeInvalidName    apperr.Code = "skill.invalid_name"
	CodeInvalidScope   apperr.Code = "skill.invalid_scope"
	CodeNotFound       apperr.Code = "skill.not_found"
	CodeExists         apperr.Code = "skill.exists"
	CodeInvalidContent apperr.Code = "skill.invalid_content"
	CodeMovePartial    apperr.Code = "skill.move_partial"
)
