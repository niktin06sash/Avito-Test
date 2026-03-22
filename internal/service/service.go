package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"

	"test-backend/internal/apperrors"
	"test-backend/internal/model"
)

const slotDuration = 30 * time.Minute

var (
	fixedAdminID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fixedUserID  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

type Repository interface {
	CreateUser(ctx context.Context, user model.User) (model.User, error)
	EnsureUser(ctx context.Context, user model.User) error
	GetUserByEmail(ctx context.Context, email string) (model.User, error)
	CreateRoom(ctx context.Context, room model.Room) (model.Room, error)
	ListRooms(ctx context.Context) ([]model.Room, error)
	RoomExists(ctx context.Context, roomID uuid.UUID) (bool, error)
	CreateSchedule(ctx context.Context, schedule model.Schedule) (model.Schedule, error)
	SaveSlots(ctx context.Context, slots []model.Slot) error
	ListAvailableSlotsByRoomAndDate(ctx context.Context, roomID uuid.UUID, date time.Time) ([]model.Slot, error)
	GetSlotByID(ctx context.Context, slotID uuid.UUID) (model.Slot, error)
	CreateBooking(ctx context.Context, b model.Booking) (model.Booking, error)
	ListBookings(ctx context.Context, page, pageSize int) ([]model.Booking, int, error)
	ListMyFutureBookings(ctx context.Context, userID uuid.UUID, now time.Time) ([]model.Booking, error)
	GetBooking(ctx context.Context, bookingID uuid.UUID) (model.Booking, error)
	CancelBooking(ctx context.Context, bookingID uuid.UUID) error
}

type Service struct {
	repo Repository
	log  *logrus.Entry
}

func New(repo Repository, log *logrus.Entry) *Service {
	return &Service{repo: repo, log: log}
}

func (s *Service) EnsureSystemUsers(ctx context.Context) error {
	now := time.Now().UTC()
	if err := s.repo.EnsureUser(ctx, model.User{
		ID:           fixedAdminID,
		Email:        "admin@dummy.local",
		PasswordHash: "dummy",
		Role:         model.RoleAdmin,
		CreatedAt:    now,
	}); err != nil {
		s.log.WithField("error", err.Error()).Error("failed to ensure admin user")
		return err
	}
	if err := s.repo.EnsureUser(ctx, model.User{
		ID:           fixedUserID,
		Email:        "user@dummy.local",
		PasswordHash: "dummy",
		Role:         model.RoleUser,
		CreatedAt:    now,
	}); err != nil {
		s.log.WithField("error", err.Error()).Error("failed to ensure user user")
		return err
	}
	s.log.Info("system users ensured")
	return nil
}

func (s *Service) DummyLogin(ctx context.Context, role model.Role) (uuid.UUID, model.Role, error) {
	var id uuid.UUID
	switch role {
	case model.RoleAdmin:
		id = fixedAdminID
	case model.RoleUser:
		id = fixedUserID
	default:
		s.log.WithField("role", role).Warn("invalid role for dummy login")
		return uuid.Nil, "", apperrors.BadRequest
	}
	s.log.WithFields(logrus.Fields{"user_id": id, "role": role}).Info("dummy login")
	return id, role, nil
}

func (s *Service) Register(ctx context.Context, email, password string, role model.Role) (model.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		s.log.WithField("email", email).Error("failed to generate password hash")
		return model.User{}, err
	}
	user := model.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}
	createdUser, err := s.repo.CreateUser(ctx, user)
	if err != nil {
		s.log.WithFields(logrus.Fields{"email": email, "error": err.Error()}).Error("failed to create user in repository")
		return model.User{}, err
	}
	s.log.WithFields(logrus.Fields{"user_id": createdUser.ID, "email": email}).Info("user registered successfully")
	return createdUser, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (uuid.UUID, model.Role, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		s.log.WithField("email", email).Debug("user not found for login")
		return uuid.Nil, "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		s.log.WithField("email", email).Warn("invalid password for login")
		return uuid.Nil, "", apperrors.Forbidden
	}
	s.log.WithFields(logrus.Fields{"user_id": user.ID, "email": email, "role": user.Role}).Info("login successful")
	return user.ID, user.Role, nil
}

func (s *Service) CreateRoom(ctx context.Context, name string, description *string, capacity *int) (model.Room, error) {
	room, err := s.repo.CreateRoom(ctx, model.Room{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		Capacity:    capacity,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		s.log.WithFields(logrus.Fields{"room_name": name, "error": err.Error()}).Error("failed to create room")
		return model.Room{}, err
	}
	s.log.WithFields(logrus.Fields{"room_id": room.ID, "room_name": room.Name}).Info("room created successfully")
	return room, nil
}

func (s *Service) ListRooms(ctx context.Context) ([]model.Room, error) {
	rooms, err := s.repo.ListRooms(ctx)
	if err != nil {
		s.log.WithField("error", err.Error()).Error("failed to list rooms")
		return nil, err
	}
	s.log.WithField("rooms_count", len(rooms)).Info("rooms listed")
	return rooms, nil
}

func (s *Service) CreateSchedule(ctx context.Context, roomID uuid.UUID, days []int, startTime, endTime string) (model.Schedule, error) {
	ok, err := s.repo.RoomExists(ctx, roomID)
	if err != nil {
		s.log.WithFields(logrus.Fields{"room_id": roomID, "error": err.Error()}).Error("failed to check room existence")
		return model.Schedule{}, err
	}
	if !ok {
		s.log.WithField("room_id", roomID).Warn("room not found for schedule")
		return model.Schedule{}, apperrors.NotFound
	}

	startClock, endClock, err := parseClockRange(startTime, endTime)
	if err != nil {
		s.log.WithFields(logrus.Fields{"start_time": startTime, "end_time": endTime, "error": err.Error()}).Warn("invalid time range")
		return model.Schedule{}, apperrors.BadRequest
	}
	for _, d := range days {
		if d < 1 || d > 7 {
			s.log.WithField("days", days).Warn("invalid days in schedule")
			return model.Schedule{}, apperrors.BadRequest
		}
	}

	schedule, err := s.repo.CreateSchedule(ctx, model.Schedule{
		ID:         uuid.New(),
		RoomID:     roomID,
		DaysOfWeek: days,
		StartTime:  startTime,
		EndTime:    endTime,
	})
	if err != nil {
		s.log.WithFields(logrus.Fields{"room_id": roomID, "error": err.Error()}).Error("failed to create schedule in repository")
		return model.Schedule{}, err
	}
	var slots []model.Slot
	now := time.Now().UTC()
	baseDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for i := 0; i < 7; i++ {
		day := baseDate.AddDate(0, 0, i)
		wd := int(day.Weekday())
		if wd == 0 {
			wd = 7
		}
		if !containsDay(days, wd) {
			continue
		}
		cur := time.Date(day.Year(), day.Month(), day.Day(), startClock.Hour(), startClock.Minute(), 0, 0, time.UTC)
		limit := time.Date(day.Year(), day.Month(), day.Day(), endClock.Hour(), endClock.Minute(), 0, 0, time.UTC)
		for cur.Before(limit) {
			slotEnd := cur.Add(slotDuration)
			if slotEnd.After(limit) {
				break
			}
			slots = append(slots, model.Slot{
				ID:     uuid.New(),
				RoomID: roomID,
				Start:  cur,
				End:    slotEnd,
			})
			cur = slotEnd
		}
	}
	if err = s.repo.SaveSlots(ctx, slots); err != nil {
		s.log.WithFields(logrus.Fields{"room_id": roomID, "slots_count": len(slots), "error": err.Error()}).Error("failed to save slots")
		return model.Schedule{}, err
	}
	s.log.WithFields(logrus.Fields{"schedule_id": schedule.ID, "room_id": roomID, "slots_created": len(slots)}).Info("schedule created successfully")
	return schedule, nil
}

func (s *Service) ListAvailableSlots(ctx context.Context, roomID uuid.UUID, date time.Time) ([]model.Slot, error) {
	ok, err := s.repo.RoomExists(ctx, roomID)
	if err != nil {
		s.log.WithFields(logrus.Fields{"room_id": roomID, "error": err.Error()}).Error("failed to check room existence")
		return nil, err
	}
	if !ok {
		s.log.WithField("room_id", roomID).Warn("room not found for slots listing")
		return nil, apperrors.NotFound
	}
	slots, err := s.repo.ListAvailableSlotsByRoomAndDate(ctx, roomID, date)
	if err != nil {
		s.log.WithFields(logrus.Fields{"room_id": roomID, "date": date, "error": err.Error()}).Error("failed to list available slots")
		return nil, err
	}
	s.log.WithFields(logrus.Fields{"room_id": roomID, "date": date, "slots_count": len(slots)}).Info("available slots listed")
	return slots, nil
}

func (s *Service) CreateBooking(ctx context.Context, slotID, userID uuid.UUID, createConferenceLink bool) (model.Booking, error) {
	slot, err := s.repo.GetSlotByID(ctx, slotID)
	if err != nil {
		s.log.WithFields(logrus.Fields{"slot_id": slotID, "error": err.Error()}).Error("failed to get slot")
		return model.Booking{}, err
	}
	if slot.Start.Before(time.Now().UTC()) {
		s.log.WithFields(logrus.Fields{"slot_id": slotID, "slot_start": slot.Start}).Warn("slot is in the past")
		return model.Booking{}, apperrors.BadRequest
	}
	var link *string
	if createConferenceLink {
		v := fmt.Sprintf("https://meet.example.local/%s", uuid.NewString())
		link = &v
	}
	booking, err := s.repo.CreateBooking(ctx, model.Booking{
		ID:             uuid.New(),
		SlotID:         slotID,
		UserID:         userID,
		Status:         model.BookingActive,
		ConferenceLink: link,
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		s.log.WithFields(logrus.Fields{"slot_id": slotID, "user_id": userID, "error": err.Error()}).Error("failed to create booking in repository")
		return model.Booking{}, err
	}
	s.log.WithFields(logrus.Fields{"booking_id": booking.ID, "slot_id": slotID, "user_id": userID}).Info("booking created successfully")
	return booking, nil
}

func (s *Service) ListBookings(ctx context.Context, page, pageSize int) ([]model.Booking, int, error) {
	if page < 1 || pageSize < 1 || pageSize > 100 {
		s.log.WithFields(logrus.Fields{"page": page, "page_size": pageSize}).Warn("invalid pagination parameters")
		return nil, 0, apperrors.BadRequest
	}
	bookings, total, err := s.repo.ListBookings(ctx, page, pageSize)
	if err != nil {
		s.log.WithFields(logrus.Fields{"page": page, "page_size": pageSize, "error": err.Error()}).Error("failed to list bookings")
		return nil, 0, err
	}
	s.log.WithFields(logrus.Fields{"page": page, "page_size": pageSize, "total": total, "returned": len(bookings)}).Info("bookings listed")
	return bookings, total, nil
}

func (s *Service) ListMyBookings(ctx context.Context, userID uuid.UUID) ([]model.Booking, error) {
	bookings, err := s.repo.ListMyFutureBookings(ctx, userID, time.Now().UTC())
	if err != nil {
		s.log.WithFields(logrus.Fields{"user_id": userID, "error": err.Error()}).Error("failed to list my bookings")
		return nil, err
	}
	s.log.WithFields(logrus.Fields{"user_id": userID, "bookings_count": len(bookings)}).Info("my bookings listed")
	return bookings, nil
}

func (s *Service) CancelBooking(ctx context.Context, bookingID, userID uuid.UUID) (model.Booking, error) {
	booking, err := s.repo.GetBooking(ctx, bookingID)
	if err != nil {
		s.log.WithFields(logrus.Fields{"booking_id": bookingID, "error": err.Error()}).Error("failed to get booking")
		return model.Booking{}, err
	}
	if booking.UserID != userID {
		s.log.WithFields(logrus.Fields{"booking_id": bookingID, "user_id": userID, "booking_owner": booking.UserID}).Warn("user attempting to cancel someone else's booking")
		return model.Booking{}, apperrors.Forbidden
	}
	if booking.Status == model.BookingCancelled {
		s.log.WithFields(logrus.Fields{"booking_id": bookingID, "user_id": userID}).Info("booking already cancelled")
		return booking, nil
	}
	err = s.repo.CancelBooking(ctx, bookingID)
	if err != nil {
		s.log.WithFields(logrus.Fields{"booking_id": bookingID, "user_id": userID, "error": err.Error()}).Error("failed to cancel booking")
		return model.Booking{}, err
	}
	booking.Status = model.BookingCancelled
	s.log.WithFields(logrus.Fields{"booking_id": bookingID, "user_id": userID, "status": booking.Status}).Info("booking cancelled successfully")
	return booking, nil
}
