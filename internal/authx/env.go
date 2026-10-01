package authx

type UserInfo struct {
	Name     string
	UID, GID int
	Home     string
	Shell    string
}

// BridgeEnv là toàn bộ môi trường của metaos-bridge; môi trường của người gọi
// bị bỏ hết để không biến nào lọt qua ranh giới setuid.
func BridgeEnv(u UserInfo) []string {
	shell := u.Shell
	if shell == "" {
		shell = "/bin/sh"
	}
	return []string{
		"HOME=" + u.Home, "USER=" + u.Name, "LOGNAME=" + u.Name, "SHELL=" + shell,
		"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8",
	}
}
