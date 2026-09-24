package service

import "errors"

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrSelfApproval      = errors.New("the submitter cannot approve the same safety clearance")
	ErrReviewerRequired  = errors.New("a reviewer or administrator must perform the second confirmation")
	ErrWindowVersion     = errors.New("风浪窗口版本缺失或已变化，请刷新后按当前窗口版本重新确认")
	ErrWindowUnsafe      = errors.New("关联风浪窗口当前不安全（受限、过期或尚未确认安全），禁止放行许可；请在窗口恢复安全后重新确认")
	ErrWindowMissing     = errors.New("关联风浪窗口不存在或已被删除，许可放行前请先核实窗口")
)
