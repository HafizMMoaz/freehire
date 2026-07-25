package auth

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// WSSubprotocolMarker is the literal a browser client sends as its first
// requested WebSocket subprotocol, with the session JWT as the second. Browsers
// cannot set headers on `new WebSocket(url, protocols)`, so the subprotocol slot
// is the only place a cross-origin extension can put a credential — the same
// trick the Roy client already uses. The server echoes back the marker alone,
// never the token.
const WSSubprotocolMarker = "freehire-jwt"

// SubprotocolToken extracts the credential from a `Sec-WebSocket-Protocol`
// header of the form "freehire-jwt, <token>", returning "" for anything else so
// an unmarked or empty header simply fails to authenticate.
func SubprotocolToken(header string) string {
	parts := strings.Split(header, ",")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) != WSSubprotocolMarker {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// RequireAuthWS returns middleware that authenticates a WebSocket handshake from
// the session JWT and stores the user id in the same locals as RequireAuth, so
// the upgraded connection inherits it. The token comes from an
// `Authorization: Bearer` header (a server-side harness) or from the subprotocol
// (a browser, which has neither headers nor a cross-origin cookie). Rejects with
// 401 when neither yields a valid session.
func RequireAuthWS(iss *Issuer) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := bearerToken(c)
		if token == "" {
			token = SubprotocolToken(c.Get("Sec-WebSocket-Protocol"))
		}
		id, err := iss.Parse(token)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "not authenticated")
		}
		c.Locals(LocalsUserID, id)
		return c.Next()
	}
}
