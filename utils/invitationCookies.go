package utils

import (
	"github.com/gofiber/fiber/v2"
	"time"
)

func InvitationContinuationCookieName() string {
	if isProductionCookieEnvironment() {
		return "__Host-invitation_continuation"
	}
	return "invitation_continuation"
}
func SetInvitationContinuationCookie(c *fiber.Ctx, handle string, expires time.Time) {
	setAuthCookieWithPath(c, InvitationContinuationCookieName(), handle, expires.Unix(), "/")
}
func ClearInvitationContinuationCookie(c *fiber.Ctx) {
	clearAuthCookieWithPath(c, InvitationContinuationCookieName(), "/")
}
