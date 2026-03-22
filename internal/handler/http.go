package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"test-backend/internal/apperrors"
	"test-backend/internal/auth"
	"test-backend/internal/model"
)

type Service interface {
	DummyLogin(ctx context.Context, role model.Role) (uuid.UUID, model.Role, error)
	Register(ctx context.Context, email, password string, role model.Role) (model.User, error)
	Login(ctx context.Context, email, password string) (uuid.UUID, model.Role, error)
	CreateRoom(ctx context.Context, name string, description *string, capacity *int) (model.Room, error)
	ListRooms(ctx context.Context) ([]model.Room, error)
	CreateSchedule(ctx context.Context, roomID uuid.UUID, days []int, startTime, endTime string) (model.Schedule, error)
	ListAvailableSlots(ctx context.Context, roomID uuid.UUID, date time.Time) ([]model.Slot, error)
	CreateBooking(ctx context.Context, slotID, userID uuid.UUID, createConferenceLink bool) (model.Booking, error)
	ListBookings(ctx context.Context, page, pageSize int) ([]model.Booking, int, error)
	ListMyBookings(ctx context.Context, userID uuid.UUID) ([]model.Booking, error)
	CancelBooking(ctx context.Context, bookingID, userID uuid.UUID) (model.Booking, error)
}

type APIHandler struct {
	service Service
	secret  string
	log     *logrus.Entry
}

func New(svc Service, jwtSecret string, log *logrus.Entry) *APIHandler {
	return &APIHandler{service: svc, secret: jwtSecret, log: log}
}

func (h *APIHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	api := HandlerWithOptions(h, StdHTTPServerOptions{
		Middlewares: []MiddlewareFunc{JWTMiddleware([]byte(h.secret), h.log)},
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadRequest, INVALIDREQUEST, err.Error())
		},
	})
	mux.Handle("/_info", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mux.Handle("/", api)
	return mux
}

func (h *APIHandler) PostDummyLogin(w http.ResponseWriter, r *http.Request) {
	var req PostDummyLoginJSONBody
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		h.log.Warn("invalid json in dummy login")
		writeError(w, http.StatusBadRequest, INVALIDREQUEST, "invalid json")
		return
	}
	userID, role, err := h.service.DummyLogin(r.Context(), model.Role(req.Role))
	if err != nil {
		writeError(w, http.StatusBadRequest, INVALIDREQUEST, "invalid role")
		return
	}
	token, err := auth.SignToken([]byte(h.secret), userID, role)
	if err != nil {
		h.log.WithFields(logrus.Fields{"user_id": userID, "error": err.Error()}).Error("failed to sign token")
		writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Token{Token: token})
	h.log.WithFields(logrus.Fields{"user_id": userID, "role": role}).Info("dummy login successful")
}

func (h *APIHandler) PostRegister(w http.ResponseWriter, r *http.Request) {
	var req PostRegisterJSONBody
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		h.log.Warn("invalid json in register request")
		writeError(w, http.StatusBadRequest, INVALIDREQUEST, "invalid json")
		return
	}
	user, err := h.service.Register(r.Context(), string(req.Email), req.Password, model.Role(req.Role))
	if err != nil {
		if errors.Is(err, apperrors.Conflict) {
			writeError(w, http.StatusBadRequest, INVALIDREQUEST, "email already exists")
			return
		}
		writeInternal(w, err)
		return
	}
	resp := struct {
		User User `json:"user"`
	}{User: toUser(user)}
	writeJSON(w, http.StatusCreated, resp)
	h.log.WithFields(logrus.Fields{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    user.Role,
	}).Info("user registered successfully")
}

func (h *APIHandler) PostLogin(w http.ResponseWriter, r *http.Request) {
	var req PostLoginJSONBody
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		h.log.Warn("invalid json in login request")
		writeError(w, http.StatusBadRequest, INVALIDREQUEST, "invalid json")
		return
	}
	userID, role, err := h.service.Login(r.Context(), string(req.Email), req.Password)
	if err != nil {
		if errors.Is(err, apperrors.NotFound) {
			writeError(w, http.StatusUnauthorized, FORBIDDEN, "invalid credentials")
			return
		}
		writeInternal(w, err)
		return
	}
	token, err := auth.SignToken([]byte(h.secret), userID, role)
	if err != nil {
		h.log.WithFields(logrus.Fields{"user_id": userID, "error": err.Error()}).Error("failed to sign token")
		writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Token{Token: token})
	h.log.WithFields(logrus.Fields{"user_id": userID, "role": role}).Info("login successful")
}

func (h *APIHandler) GetRoomsList(w http.ResponseWriter, r *http.Request) {
	p, ok := mustPrincipal(w, r)
	if !ok {
		return
	}
	rooms, err := h.service.ListRooms(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	resp := struct {
		Rooms []Room `json:"rooms"`
	}{Rooms: make([]Room, 0, len(rooms))}
	for _, room := range rooms {
		resp.Rooms = append(resp.Rooms, toRoom(room))
	}
	writeJSON(w, http.StatusOK, resp)
	h.log.WithFields(logrus.Fields{"user_id": p.UserID, "rooms_count": len(rooms)}).Info("rooms listed successfully")
}

func (h *APIHandler) PostRoomsCreate(w http.ResponseWriter, r *http.Request) {
	p, ok := mustPrincipal(w, r)
	if !ok {
		return
	}
	if p.Role != model.RoleAdmin {
		h.log.WithFields(logrus.Fields{"user_id": p.UserID, "role": p.Role}).Warn("create room: insufficient permissions")
		writeError(w, http.StatusForbidden, FORBIDDEN, "admin role required")
		return
	}
	var req PostRoomsCreateJSONBody
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		h.log.WithFields(logrus.Fields{"user_id": p.UserID}).Warn("invalid json in create room request")
		writeError(w, http.StatusBadRequest, INVALIDREQUEST, "invalid json")
		return
	}
	room, err := h.service.CreateRoom(r.Context(), req.Name, req.Description, req.Capacity)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Room Room `json:"room"`
	}{Room: toRoom(room)})
	h.log.WithFields(logrus.Fields{"room_id": room.ID, "room_name": room.Name, "user_id": p.UserID}).Info("room created successfully")
}

func (h *APIHandler) PostRoomsRoomIdScheduleCreate(w http.ResponseWriter, r *http.Request, roomId RoomIdPath) {
	p, ok := mustPrincipal(w, r)
	if !ok {
		return
	}
	if p.Role != model.RoleAdmin {
		h.log.WithFields(logrus.Fields{"user_id": p.UserID, "role": p.Role}).Warn("create schedule: insufficient permissions")
		writeError(w, http.StatusForbidden, FORBIDDEN, "admin role required")
		return
	}
	var req Schedule
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		h.log.WithFields(logrus.Fields{"user_id": p.UserID}).Warn("invalid json in create schedule request")
		writeError(w, http.StatusBadRequest, INVALIDREQUEST, "invalid json")
		return
	}
	schedule, err := h.service.CreateSchedule(r.Context(), uuid.UUID(roomId), req.DaysOfWeek, req.StartTime, req.EndTime)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.NotFound):
			writeError(w, http.StatusNotFound, ROOMNOTFOUND, "room not found")
		case errors.Is(err, apperrors.Conflict):
			writeError(w, http.StatusConflict, SCHEDULEEXISTS, "schedule already exists")
		case errors.Is(err, apperrors.BadRequest):
			writeError(w, http.StatusBadRequest, INVALIDREQUEST, "invalid schedule")
		default:
			writeInternal(w, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Schedule Schedule `json:"schedule"`
	}{Schedule: toSchedule(schedule)})
	h.log.WithFields(logrus.Fields{"schedule_id": schedule.ID, "room_id": roomId, "user_id": p.UserID}).Info("schedule created successfully")
}

func (h *APIHandler) GetRoomsRoomIdSlotsList(w http.ResponseWriter, r *http.Request, roomId RoomIdPath, params GetRoomsRoomIdSlotsListParams) {
	p, ok := mustPrincipal(w, r)
	if !ok {
		return
	}
	slots, err := h.service.ListAvailableSlots(r.Context(), uuid.UUID(roomId), time.Time(params.Date.Time))
	if err != nil {
		if errors.Is(err, apperrors.NotFound) {
			writeError(w, http.StatusNotFound, ROOMNOTFOUND, "room not found")
			return
		}
		writeInternal(w, err)
		return
	}
	resp := struct {
		Slots []Slot `json:"slots"`
	}{Slots: make([]Slot, 0, len(slots))}
	for _, slot := range slots {
		resp.Slots = append(resp.Slots, toSlot(slot))
	}
	writeJSON(w, http.StatusOK, resp)
	h.log.WithFields(logrus.Fields{"room_id": roomId, "date": params.Date.Time, "slots_count": len(slots), "user_id": p.UserID}).Info("available slots listed")
}

func (h *APIHandler) PostBookingsCreate(w http.ResponseWriter, r *http.Request) {
	p, ok := mustPrincipal(w, r)
	if !ok {
		return
	}
	if p.Role != model.RoleUser {
		h.log.WithFields(logrus.Fields{"user_id": p.UserID, "role": p.Role}).Warn("create booking: insufficient permissions")
		writeError(w, http.StatusForbidden, FORBIDDEN, "user role required")
		return
	}
	var req PostBookingsCreateJSONBody
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		h.log.WithFields(logrus.Fields{"user_id": p.UserID}).Warn("invalid json in create booking request")
		writeError(w, http.StatusBadRequest, INVALIDREQUEST, "invalid json")
		return
	}
	createLink := req.CreateConferenceLink != nil && *req.CreateConferenceLink
	booking, err := h.service.CreateBooking(r.Context(), uuid.UUID(req.SlotId), p.UserID, createLink)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.NotFound):
			writeError(w, http.StatusNotFound, SLOTNOTFOUND, "slot not found")
		case errors.Is(err, apperrors.Conflict):
			writeError(w, http.StatusConflict, SLOTALREADYBOOKED, "slot already booked")
		case errors.Is(err, apperrors.BadRequest):
			writeError(w, http.StatusBadRequest, INVALIDREQUEST, "slot in past")
		default:
			writeInternal(w, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Booking Booking `json:"booking"`
	}{Booking: toBooking(booking)})
	h.log.WithFields(logrus.Fields{"booking_id": booking.ID, "slot_id": booking.SlotID, "user_id": p.UserID}).Info("booking created successfully")
}

func (h *APIHandler) GetBookingsList(w http.ResponseWriter, r *http.Request, params GetBookingsListParams) {
	p, ok := mustPrincipal(w, r)
	if !ok {
		return
	}
	if p.Role != model.RoleAdmin {
		h.log.WithFields(logrus.Fields{"user_id": p.UserID, "role": p.Role}).Warn("list bookings: insufficient permissions")
		writeError(w, http.StatusForbidden, FORBIDDEN, "admin role required")
		return
	}
	page, pageSize := 1, 20
	if params.Page != nil {
		page = *params.Page
	}
	if params.PageSize != nil {
		pageSize = *params.PageSize
	}
	bookings, total, err := h.service.ListBookings(r.Context(), page, pageSize)
	if err != nil {
		if errors.Is(err, apperrors.BadRequest) {
			writeError(w, http.StatusBadRequest, INVALIDREQUEST, "invalid pagination")
			return
		}
		writeInternal(w, err)
		return
	}
	resp := struct {
		Bookings   []Booking  `json:"bookings"`
		Pagination Pagination `json:"pagination"`
	}{Bookings: make([]Booking, 0, len(bookings)), Pagination: Pagination{Page: page, PageSize: pageSize, Total: total}}
	for _, b := range bookings {
		resp.Bookings = append(resp.Bookings, toBooking(b))
	}
	writeJSON(w, http.StatusOK, resp)
	h.log.WithFields(logrus.Fields{"page": page, "page_size": pageSize, "total": total, "returned": len(bookings), "user_id": p.UserID}).Info("bookings listed successfully")
}

func (h *APIHandler) GetBookingsMy(w http.ResponseWriter, r *http.Request) {
	p, ok := mustPrincipal(w, r)
	if !ok {
		return
	}
	if p.Role != model.RoleUser {
		h.log.WithFields(logrus.Fields{"user_id": p.UserID, "role": p.Role}).Warn("list my bookings: insufficient permissions")
		writeError(w, http.StatusForbidden, FORBIDDEN, "user role required")
		return
	}
	bookings, err := h.service.ListMyBookings(r.Context(), p.UserID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	resp := struct {
		Bookings []Booking `json:"bookings"`
	}{Bookings: make([]Booking, 0, len(bookings))}
	for _, b := range bookings {
		resp.Bookings = append(resp.Bookings, toBooking(b))
	}
	writeJSON(w, http.StatusOK, resp)
	h.log.WithFields(logrus.Fields{"user_id": p.UserID, "bookings_count": len(bookings)}).Info("my bookings listed successfully")
}

func (h *APIHandler) PostBookingsBookingIdCancel(w http.ResponseWriter, r *http.Request, bookingId BookingIdPath) {
	p, ok := mustPrincipal(w, r)
	if !ok {
		return
	}
	if p.Role != model.RoleUser {
		h.log.WithFields(logrus.Fields{"booking_id": bookingId, "user_id": p.UserID, "role": p.Role}).Warn("cancel booking: insufficient permissions")
		writeError(w, http.StatusForbidden, FORBIDDEN, "user role required")
		return
	}
	booking, err := h.service.CancelBooking(r.Context(), uuid.UUID(bookingId), p.UserID)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.NotFound):
			writeError(w, http.StatusNotFound, BOOKINGNOTFOUND, "booking not found")
		case errors.Is(err, apperrors.Forbidden):
			writeError(w, http.StatusForbidden, FORBIDDEN, "cannot cancel another user's booking")
		default:
			writeInternal(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Booking Booking `json:"booking"`
	}{Booking: toBooking(booking)})
	h.log.WithFields(logrus.Fields{"booking_id": booking.ID, "user_id": p.UserID, "status": booking.Status}).Info("booking cancelled successfully")
}
