package main

/*
#cgo LDFLAGS: -lpam
#include <security/pam_appl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <grp.h>
#include <pwd.h>

// Trả mật khẩu cho mọi câu hỏi ẩn ký tự; câu hỏi khác nhận chuỗi rỗng.
// Bản strdup thuộc về module PAM (pam_unix xoá rồi free); nhánh lỗi tự xoá.
static int metaos_conv(int n, const struct pam_message **msg, struct pam_response **resp, void *appdata) {
	if (n <= 0 || n > PAM_MAX_NUM_MSG) return PAM_CONV_ERR;
	struct pam_response *r = calloc((size_t)n, sizeof(*r));
	if (r == NULL) return PAM_BUF_ERR;
	for (int i = 0; i < n; i++) {
		if (msg[i]->msg_style != PAM_PROMPT_ECHO_OFF) continue;
		if ((r[i].resp = strdup((const char *)appdata)) != NULL) continue;
		for (int j = 0; j < i; j++) if (r[j].resp != NULL) { explicit_bzero(r[j].resp, strlen(r[j].resp)); free(r[j].resp); }
		free(r);
		return PAM_BUF_ERR;
	}
	*resp = r;
	return PAM_SUCCESS;
}

static int metaos_pam_check(const char *user, const char *password) {
	struct pam_conv conv = { metaos_conv, (void *)password };
	pam_handle_t *h = NULL;
	int rc;
	if ((rc = pam_start("metaos", user, &conv, &h)) != PAM_SUCCESS) return rc;
	rc = pam_authenticate(h, PAM_DISALLOW_NULL_AUTHTOK);
	if (rc == PAM_SUCCESS) rc = pam_acct_mgmt(h, PAM_DISALLOW_NULL_AUTHTOK);
	pam_end(h, rc);
	return rc;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// pamExit: mã thoát theo D1. Kết quả PAM không có trong bảng — lỗi hệ thống,
// pam_start hỏng (PAM_BUF_ERR, PAM_SYSTEM_ERR, PAM_ABORT…), PAM_AUTHINFO_UNAVAIL
// (không tới được nguồn danh bạ = hạ tầng) — là exitInternal.
var pamExit = map[C.int]int{
	C.PAM_SUCCESS: exitOK, C.PAM_NEW_AUTHTOK_REQD: exitExpired, C.PAM_AUTH_ERR: exitAuth, C.PAM_USER_UNKNOWN: exitAuth,
	C.PAM_MAXTRIES: exitAuth, C.PAM_ACCT_EXPIRED: exitAuth, C.PAM_PERM_DENIED: exitAuth, C.PAM_CRED_INSUFFICIENT: exitAuth,
}

// pamCheck: pw phải khác rỗng (authx.ReadPassword đã bảo đảm). Bản sao C của
// mật khẩu bị xoá trước khi giải phóng. Trả mã thoát và mã PAM gốc.
func pamCheck(user string, pw []byte) (code, rc int) {
	cu := C.CString(user)
	defer C.free(unsafe.Pointer(cu))
	size := C.size_t(len(pw) + 1)
	cp := C.calloc(size, 1)
	defer func() { C.explicit_bzero(cp, size); C.free(cp) }()
	C.memcpy(cp, unsafe.Pointer(&pw[0]), C.size_t(len(pw)))
	r := C.metaos_pam_check(cu, (*C.char)(cp))
	if code, ok := pamExit[r]; ok {
		return code, int(r)
	}
	return exitInternal, int(r)
}

// flushStdio đẩy bộ đệm stdio của C (printf của module PAM/NSS) khi fd 1 còn
// trỏ /dev/null, để không byte nào lọt vào ống frame sau khi trả stdout.
func flushStdio() { C.fflush(nil) }

// initgroups đặt nhóm phụ của user và trả login shell; cả hai qua NSS
// (getpwnam/initgroups) nên user LDAP/SSSD cũng đúng.
func initgroups(user string, gid int) (shell string, err error) {
	cu := C.CString(user)
	defer C.free(unsafe.Pointer(cu))
	if p := C.getpwnam(cu); p != nil {
		shell = C.GoString(p.pw_shell)
	}
	if rc, e := C.initgroups(cu, C.gid_t(gid)); rc != 0 {
		return "", fmt.Errorf("initgroups %s: %v", user, e)
	}
	return shell, nil
}
