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
	int rc = pam_start("metaos", user, &conv, &h);
	if (rc != PAM_SUCCESS) return rc;
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

const pamSuccess, pamNewAuthtokReqd = int(C.PAM_SUCCESS), int(C.PAM_NEW_AUTHTOK_REQD)

// pamCheck: pw phải khác rỗng (authx.ReadPassword đã bảo đảm). Bản sao C của
// mật khẩu bị xoá trước khi giải phóng.
func pamCheck(user string, pw []byte) int {
	cu := C.CString(user)
	defer C.free(unsafe.Pointer(cu))
	size := C.size_t(len(pw) + 1)
	cp := C.calloc(size, 1)
	defer func() { C.explicit_bzero(cp, size); C.free(cp) }()
	C.memcpy(cp, unsafe.Pointer(&pw[0]), C.size_t(len(pw)))
	return int(C.metaos_pam_check(cu, (*C.char)(cp)))
}

// flushStdio đẩy bộ đệm stdio của C (printf của module PAM/NSS) khi fd 1 còn
// trỏ /dev/null, để không byte nào lọt vào ống frame sau khi trả stdout.
func flushStdio() { C.fflush(nil) }

// loginShell lấy shell qua NSS (getpwnam) nên user LDAP/SSSD cũng đúng.
func loginShell(user string) string {
	cu := C.CString(user)
	defer C.free(unsafe.Pointer(cu))
	if p := C.getpwnam(cu); p != nil {
		return C.GoString(p.pw_shell)
	}
	return ""
}

func initgroups(user string, gid int) error {
	cu := C.CString(user)
	defer C.free(unsafe.Pointer(cu))
	if rc, err := C.initgroups(cu, C.gid_t(gid)); rc != 0 {
		return fmt.Errorf("initgroups %s: %v", user, err)
	}
	return nil
}
