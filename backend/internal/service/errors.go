package service

import "errors"

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrSelfApproval      = errors.New("the submitter cannot approve the same safety clearance")
	ErrReviewerRequired  = errors.New("a reviewer or administrator must perform the second confirmation")
	ErrWindowVersion     = errors.New("the weather window version is missing or changed")
	ErrWindowUnsafe      = errors.New("当前风浪窗口不安全（非安全状态），许可不能放行，请窗口恢复安全后重新确认")
	ErrVersionChanged    = errors.New("风浪窗口版本已变化，旧提交不能直接放行，请基于最新窗口版本重新确认")
	ErrRebindRequired    = errors.New("许可已换绑新窗口版本，须由原提交人重新确认后才能复核放行")
	ErrWindowUnbound     = errors.New("许可未绑定风浪窗口，无法校验窗口安全状态")
)
