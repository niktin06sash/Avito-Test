package handler

import (
	"encoding/json"
	"net/http"

	"test-backend/internal/model"

	"github.com/oapi-codegen/runtime/types"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code ErrorResponseErrorCode, message string) {
	resp := ErrorResponse{}
	resp.Error.Code = code
	resp.Error.Message = message
	writeJSON(w, status, resp)
}

func writeInternal(w http.ResponseWriter, _ error) {
	resp := InternalErrorResponse{}
	resp.Error.Code = string(INTERNALERROR)
	resp.Error.Message = "internal server error"
	writeJSON(w, http.StatusInternalServerError, resp)
}

func mustPrincipal(w http.ResponseWriter, r *http.Request) (principal, bool) {
	p, ok := r.Context().Value(principalKey).(principal)
	if !ok {
		writeError(w, http.StatusUnauthorized, UNAUTHORIZED, "unauthorized")
		return principal{}, false
	}
	return p, true
}

func toUser(u model.User) User {
	id := u.ID
	email := u.Email
	createdAt := u.CreatedAt
	return User{
		Id:        id,
		Email:     types.Email(email),
		Role:      UserRole(u.Role),
		CreatedAt: &createdAt,
	}
}

func toRoom(r model.Room) Room {
	id := r.ID
	createdAt := r.CreatedAt
	return Room{Id: id, Name: r.Name, Description: r.Description, Capacity: r.Capacity, CreatedAt: &createdAt}
}

func toSchedule(s model.Schedule) Schedule {
	id := s.ID
	return Schedule{
		Id:         &id,
		RoomId:     s.RoomID,
		DaysOfWeek: s.DaysOfWeek,
		StartTime:  s.StartTime,
		EndTime:    s.EndTime,
	}
}

func toSlot(s model.Slot) Slot {
	return Slot{Id: s.ID, RoomId: s.RoomID, Start: s.Start, End: s.End}
}

func toBooking(b model.Booking) Booking {
	id := b.ID
	slotID := b.SlotID
	userID := b.UserID
	createdAt := b.CreatedAt
	return Booking{
		Id:             id,
		SlotId:         slotID,
		UserId:         userID,
		Status:         BookingStatus(b.Status),
		ConferenceLink: b.ConferenceLink,
		CreatedAt:      &createdAt,
	}
}
