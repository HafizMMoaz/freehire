package handler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
)

func TestMapCVErrorListCapIsConflictWithSafeMessage(t *testing.T) {
	raw := fmt.Errorf("%w: %s: Staff Engineer at Contoso already has %d bullets (the maximum). "+
		"The edit was not applied and no existing bullets were deleted. "+
		"Set an existing bullet or remove one before inserting",
		cvedit.ErrListCap, cvedit.ListCapCode, cv.MaxBullets)

	mapped := mapCVError(raw)
	var fe *fiber.Error
	if !errors.As(mapped, &fe) {
		t.Fatalf("mapCVError = %T %v, want *fiber.Error", mapped, mapped)
	}
	if fe.Code != fiber.StatusConflict {
		t.Fatalf("status = %d, want %d", fe.Code, fiber.StatusConflict)
	}
	want := cvedit.UserListCapMessage(raw)
	if fe.Message != want {
		t.Fatalf("message = %q, want %q", fe.Message, want)
	}
	if want == "" || fe.Message == raw.Error() {
		t.Fatal("HTTP message must be the candidate-safe UserListCapMessage, not the raw ErrListCap")
	}
}
